// The state machine of the layout editor (epic E09 stage 2, T-158): the document, its history and the autosave with optimistic concurrency.
// Framework-free so it is tested with fake timers and a mocked fetch; `useLayoutEditor` wraps it for React. Rules (spec 17, requirements 3-6, 8):
//  - `base_version` comes only from book-layout/get and from our own save answers, never from book-data/get (it reports 0 for a null layout).
//  - one save in flight; edits during a save queue exactly one more save of the newest committed document.
//  - a 409 puts the editor in `conflict`: no autosave, nothing overwritten until the user picks useTheirs() or keepMine().
import { ApiError } from "../../api/client";
import {
  getBookLayout,
  resetBookLayout,
  saveBookLayout,
  type LayoutConflict,
  type LayoutDoc,
  type LayoutProblem,
} from "../../api/bookLayout";
import { emptyDoc, type Op, type Refusal } from "./document";
import { History } from "./history";

export type SaveState = "idle" | "saving" | "saved" | "offline" | "conflict" | "invalid" | "gone";
export type Snapshot = {
  loaded: "loading" | "ready" | "failed";
  doc: LayoutDoc;
  /** The last version the client knows (0 before the first save). */
  version: number;
  dirty: boolean;
  saveState: SaveState;
  canUndo: boolean;
  canRedo: boolean;
  /** Ids of the selected items or elements; not part of the history. */
  selection: readonly string[];
  /** What the server refused (state `invalid`); empty otherwise. */
  problems: readonly LayoutProblem[];
};

export const DEBOUNCE_MS = 1500;
const BACKOFF_MS = [1000, 2000, 4000, 8000];
const BACKOFF_LAST_MS = 30_000;
const DEFAULT_RETRY_AFTER_S = 5;

export class EditorStore {
  private history = new History<LayoutDoc>(emptyDoc());
  private version = 0;
  private rev = 0; // bumped by every commit, undo and redo
  private savedRev = 0; // the rev the server has
  private epoch = 0; // bumped by reset: answers of older saves are ignored
  private inFlight = false;
  private queued = false;
  private attempt = 0;
  private debounce: ReturnType<typeof setTimeout> | undefined;
  private retry: ReturnType<typeof setTimeout> | undefined;
  private notBefore = 0; // end of the current 429 / backoff wait (ms); the debounce never saves before it
  private stopped = false;
  private loaded: Snapshot["loaded"] = "loading";
  private saveState: SaveState = "idle";
  private selection: readonly string[] = [];
  private problems: readonly LayoutProblem[] = [];
  private conflict: LayoutConflict | null = null;
  private listeners = new Set<() => void>();
  private snap: Snapshot;

  constructor(private yearbookId: string) {
    this.snap = this.build();
  }

  // --- subscription (useSyncExternalStore) ---
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => void this.listeners.delete(fn);
  };
  getSnapshot = () => this.snap;
  private build(): Snapshot {
    return {
      loaded: this.loaded,
      doc: this.history.present,
      version: this.version,
      dirty: this.rev !== this.savedRev,
      saveState: this.saveState,
      canUndo: this.history.canUndo,
      canRedo: this.history.canRedo,
      selection: this.selection,
      problems: this.problems,
    };
  }
  private emit() {
    this.snap = this.build();
    this.listeners.forEach((f) => f());
  }

  // --- lifecycle ---
  /** Loads the layout on the first call; later calls (StrictMode, a remount) only re-arm the store. */
  start() {
    this.stopped = false;
    if (this.loaded === "loading" && !this.loading) void this.load();
  }
  private loading: Promise<void> | undefined;

  /** Leaving the screen: no timers are left behind and unsaved edits get one last save. */
  stop() {
    if (this.dirtyAndSavable()) void this.save();
    this.stopped = true;
    this.clearTimers();
  }

  load(): Promise<void> {
    this.loaded = "loading";
    this.emit();
    this.loading = (async () => {
      try {
        const r = await getBookLayout(this.yearbookId);
        this.history.clear(r.layout ?? emptyDoc());
        this.version = r.version;
        this.rev = this.savedRev = 0;
        this.loaded = "ready";
      } catch (e) {
        this.loaded = "failed";
        if (denied(e)) this.saveState = "gone";
      }
      this.loading = undefined;
      this.emit();
    })();
    return this.loading;
  }

  // --- editing ---
  /** One user action as one history step. Returns why it was refused, or undefined. */
  apply(op: Op): Refusal | undefined {
    if (this.loaded !== "ready") return undefined;
    const r = op(this.history.present);
    if (r.refused) return r.refused;
    this.history.push(r.doc);
    this.changed();
    return undefined;
  }
  /** Starts a gesture (pointer down). */
  begin() {
    this.history.begin();
  }
  /** Shows the result of `op` without a step and without a save. */
  preview(op: Op): Refusal | undefined {
    if (this.loaded !== "ready") return undefined;
    const r = op(this.history.present);
    if (r.refused) return r.refused;
    this.history.preview(r.doc);
    this.emit();
    return undefined;
  }
  /** Ends the gesture as one history step. */
  commit() {
    if (this.history.commit()) this.changed();
    else this.emit();
  }
  /** Throws the gesture away. */
  cancel() {
    this.history.cancel();
    this.emit();
  }
  undo() {
    if (this.history.undo()) this.changed();
  }
  redo() {
    if (this.history.redo()) this.changed();
  }
  select = (ids: readonly string[]) => {
    this.selection = ids;
    this.emit();
  };

  /** A new committed document: dirty, and the autosave countdown restarts. A new commit lifts `invalid`. */
  private changed() {
    this.rev++;
    if (this.saveState === "invalid") {
      this.saveState = "idle";
      this.problems = [];
    }
    this.schedule();
    this.emit();
  }

  // --- saving ---
  private dirtyAndSavable() {
    return (
      this.loaded === "ready" &&
      this.rev !== this.savedRev &&
      this.saveState !== "conflict" &&
      this.saveState !== "gone" &&
      this.saveState !== "invalid"
    );
  }
  private clearTimers() {
    clearTimeout(this.debounce);
    clearTimeout(this.retry);
    this.debounce = this.retry = undefined;
  }
  private schedule() {
    clearTimeout(this.debounce);
    this.debounce = undefined;
    if (this.saveState === "conflict" || this.saveState === "gone") return;
    // a commit during a 429 or backoff wait must not shorten it (flush on hide / online / stop still saves at once)
    const delay = Math.max(DEBOUNCE_MS, this.notBefore - Date.now());
    this.debounce = setTimeout(() => void this.save(), delay);
  }
  /** Saves now (page hide, leaving, the user asked). */
  flush = () => void this.save();

  private async save(): Promise<void> {
    this.clearTimers();
    if (!this.dirtyAndSavable()) return;
    if (this.inFlight) {
      this.queued = true;
      return;
    }
    this.inFlight = true;
    this.queued = false;
    const { epoch, rev } = this;
    const doc = this.history.committed;
    this.saveState = "saving";
    this.emit();
    try {
      const r = await saveBookLayout(this.yearbookId, this.version, doc);
      if (epoch !== this.epoch) return;
      this.version = r.version;
      this.savedRev = rev;
      this.attempt = 0;
      this.saveState = this.rev === rev ? "saved" : "idle";
    } catch (e) {
      if (epoch !== this.epoch) return;
      this.failed(e);
    } finally {
      this.inFlight = false;
      const again = this.queued;
      this.queued = false;
      this.emit();
      if (again) void this.save(); // edits during the save: one more save of the newest document
    }
  }

  private failed(e: unknown) {
    const status = e instanceof ApiError ? e.status : 0;
    if (e instanceof ApiError && status === 409) {
      this.conflict = (e.data as LayoutConflict | undefined) ?? null;
      this.saveState = "conflict";
      this.queued = false;
    } else if (status === 429) {
      this.wait((e as ApiError).retryAfter ?? DEFAULT_RETRY_AFTER_S, "saving");
    } else if (status === 401 || status === 403 || status === 404) {
      this.saveState = "gone";
      this.queued = false;
    } else if (status >= 400 && status < 500) {
      // 400 (invalid document, with the problem list) and 413: nothing more is sent until the next commit
      this.problems = (e as ApiError).data
        ? (((e as ApiError).data as { problems?: LayoutProblem[] }).problems ?? [])
        : [];
      this.saveState = "invalid";
      this.queued = false;
    } else {
      // network error or 5xx: stay dirty, retry with backoff (1 s, 2 s, 4 s, 8 s, then every 30 s)
      this.wait((BACKOFF_MS[this.attempt] ?? BACKOFF_LAST_MS) / 1000, "offline");
      this.attempt++;
    }
  }
  private wait(seconds: number, state: SaveState) {
    this.saveState = state;
    this.notBefore = Date.now() + seconds * 1000;
    if (this.stopped) return;
    this.retry = setTimeout(() => void this.save(), seconds * 1000);
  }

  // --- conflict ---
  /** The stored version and document the 409 carried (read again if the answer had none). */
  private async stored(): Promise<LayoutConflict | undefined> {
    if (this.conflict) return this.conflict;
    try {
      return await getBookLayout(this.yearbookId);
    } catch (e) {
      // the refetch failed: stay in `conflict` (the choice can be made again); no access any more -> `gone`
      if (denied(e)) this.saveState = "gone";
      this.emit();
      return undefined;
    }
  }
  /** Conflict choice 1: drop my edits, take the stored document, forget the history. */
  async useTheirs(): Promise<void> {
    if (this.saveState !== "conflict") return;
    const s = await this.stored();
    if (!s) return;
    this.history.clear(s.layout ?? emptyDoc());
    this.version = s.version;
    this.rev++;
    this.savedRev = this.rev;
    this.conflict = null;
    this.selection = [];
    this.saveState = "idle";
    this.emit();
  }
  /** Conflict choice 2: save my document over the stored one (on purpose), by naming the stored version. */
  async keepMine(): Promise<void> {
    if (this.saveState !== "conflict") return;
    const s = await this.stored();
    if (!s) return;
    this.version = s.version;
    this.rev++; // dirty even if my document equals the one I had last saved
    this.conflict = null;
    this.saveState = "idle";
    this.emit();
    await this.save();
  }

  // --- reset ---
  /** Deletes the stored layout and starts from the template's own pages. Asks for no confirmation (the screen does). */
  async reset(): Promise<void> {
    this.clearTimers();
    this.queued = false;
    await resetBookLayout(this.yearbookId);
    this.epoch++; // a save that was in flight is answered for a book that has moved on
    const r = await getBookLayout(this.yearbookId); // the version a reset moves on to is only known from get
    this.history.clear(r.layout ?? emptyDoc());
    this.version = r.version;
    this.rev++;
    this.savedRev = this.rev;
    this.conflict = null;
    this.problems = [];
    this.selection = [];
    this.saveState = "idle";
    this.emit();
  }
}

const denied = (e: unknown) =>
  e instanceof ApiError && (e.status === 401 || e.status === 403 || e.status === 404);
