// The design-image uploads of the editor (T-162), framework-free so the state machine is unit-tested with a fake `upload`.
// A file is checked on the client (type, 10 MiB) before anything is sent; the server stays the judge. At most MAX_IN_FLIGHT uploads run at once
// (the server allows 2 per user and answers ERROR_RATE_LIMITED to a third, so the rest wait in the queue). The same file chosen twice while it is
// still queued or uploading (a double click) is one upload. Each job ends in `done`, `failed` (with a reason the screen words) or is removed (cancel).
import { ApiError } from "../../../api/client";
import { DEFAULT_MAX_BYTES, type Media } from "../../../api/yearbooks";

export const MAX_IN_FLIGHT = 2;
export const MAX_DESIGN_IMAGES = 30;
export const ACCEPTED = ["image/jpeg", "image/png", "image/webp"];

/** Why an upload failed; the screen words it with `editor.upload.error.<code>`. */
export type UploadFailure = { code: string; seconds?: number; count?: number };
export type Job = {
  key: number;
  name: string;
  state: "queued" | "uploading" | "done" | "failed";
  progress: number; // 0..1
  failure?: UploadFailure;
};

/** The reason for an error of `media/create`. The 30-image cap and the 500 MiB quota have their own codes (T-164), so no list is needed. */
export function failureOf(e: unknown): UploadFailure {
  if (!(e instanceof ApiError)) return { code: "generic" };
  if (e.status === 0) return { code: "offline" };
  const seconds = e.retryAfter;
  switch (e.code) {
    case "payload_too_large":
      return { code: "tooLarge" };
    case "unsupported_media_type":
      return { code: "unsupported" };
    case "invalid_image":
      return { code: "corrupt" };
    case "design_image_limit": {
      const limit = (e.data as { limit?: unknown } | undefined)?.limit;
      return { code: "cap", count: typeof limit === "number" ? limit : MAX_DESIGN_IMAGES };
    }
    case "quota_exceeded":
      return { code: "quota" };
    case "rate_limited":
      return { code: "rateLimited", seconds: seconds ?? 5 };
    case "busy":
      return { code: "busy", seconds: seconds ?? 5 };
    case "email_not_verified":
      return { code: "notVerified" };
    case "request_timeout":
      return { code: "timeout" };
    default:
      return { code: "generic" };
  }
}

type Deps = {
  yearbookId: string;
  upload: (
    yearbookId: string,
    file: File,
    onProgress: (f: number) => void,
    opts: { purpose: "design"; signal: AbortSignal },
  ) => Promise<Media>;
};
/** What the screen tells the queue: what to do with a finished upload. */
type Hooks = { onDone: (m: Media) => void };

export class Uploads {
  private jobs: Job[] = [];
  private files = new Map<number, File>();
  private aborts = new Map<number, AbortController>();
  private next = 1;
  private listeners = new Set<() => void>();
  private snap: readonly Job[] = [];
  private h: Hooks = { onDone: () => {} };

  constructor(private d: Deps) {}

  /** Sets the hooks (the screen does it in an effect, so they always see the newest state). */
  bind(h: Hooks) {
    this.h = h;
  }

  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => void this.listeners.delete(fn);
  };
  getSnapshot = () => this.snap;
  private emit() {
    this.snap = [...this.jobs];
    this.listeners.forEach((f) => f());
  }
  private patch(key: number, p: Partial<Job>) {
    this.jobs = this.jobs.map((j) => (j.key === key ? { ...j, ...p } : j));
    this.emit();
  }

  /** The same file picked twice: name, size and modification time (the browser gives no other identity). */
  private same(a: File, b: File) {
    return a.name === b.name && a.size === b.size && a.lastModified === b.lastModified;
  }

  add(files: readonly File[]) {
    for (const file of files) {
      const active = [...this.files].some(
        ([k, f]) => this.same(f, file) && this.jobs.find((j) => j.key === k)?.state !== "failed",
      );
      if (active) continue;
      const key = this.next++;
      const bad: UploadFailure | undefined = !ACCEPTED.includes(file.type)
        ? { code: "unsupported" }
        : file.size > DEFAULT_MAX_BYTES
          ? { code: "tooLarge" }
          : undefined;
      this.jobs.push({
        key,
        name: file.name,
        state: bad ? "failed" : "queued",
        progress: 0,
        failure: bad,
      });
      if (!bad) this.files.set(key, file);
    }
    this.emit();
    this.pump();
  }

  private pump() {
    for (const j of this.jobs) {
      if (this.aborts.size >= MAX_IN_FLIGHT) return;
      if (j.state === "queued") void this.run(j.key);
    }
  }

  private async run(key: number) {
    const file = this.files.get(key);
    if (!file) return;
    const ctl = new AbortController();
    this.aborts.set(key, ctl);
    this.patch(key, { state: "uploading" });
    try {
      const m = await this.d.upload(
        this.d.yearbookId,
        file,
        (f) => this.patch(key, { progress: f }),
        { purpose: "design", signal: ctl.signal },
      );
      this.files.delete(key);
      this.patch(key, { state: "done", progress: 1 });
      this.h.onDone(m);
    } catch (e) {
      this.files.delete(key);
      if (ctl.signal.aborted || (e instanceof DOMException && e.name === "AbortError")) {
        this.jobs = this.jobs.filter((j) => j.key !== key); // cancelled: it leaves the list
        this.emit();
      } else this.patch(key, { state: "failed", failure: failureOf(e) });
    } finally {
      this.aborts.delete(key);
      this.pump();
    }
  }

  /** Cancels a queued or running upload; it leaves the list. */
  cancel(key: number) {
    const ctl = this.aborts.get(key);
    if (ctl) return ctl.abort();
    this.files.delete(key);
    this.jobs = this.jobs.filter((j) => j.key !== key);
    this.emit();
  }

  /** Removes a finished or failed job from the list. */
  dismiss(key: number) {
    this.jobs = this.jobs.filter((j) => j.key !== key);
    this.emit();
  }
}
