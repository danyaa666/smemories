// A mock book-layout server for the editor tests: the real optimistic-concurrency rule (save needs base_version == version) and a script of
// canned answers (an error, a network failure, a held response) that is consumed one per save before the rule applies.
import { vi } from "vitest";
import type { LayoutDoc } from "./document";

type Step = () => Response | Promise<Response>;

export type MockMedia = { id: string; width: number; height: number };

export function mockServer(init: {
  version: number;
  layout: LayoutDoc | null;
  /** The book's design images (media/get-list with purpose=design). */
  media?: MockMedia[];
}) {
  const s = {
    version: init.version,
    layout: init.layout,
    saves: [] as { yearbook_id: string; base_version: number; layout: LayoutDoc }[],
    gets: 0,
    resets: 0,
    media: [...(init.media ?? [])],
    deleted: [] as string[],
    /** The order of the calls that matter to the delete-in-use rule: "save" and "delete:<id>". */
    calls: [] as string[],
    script: [] as Step[],
    /** A second client saved: moves the stored version on. */
    otherTab(layout: LayoutDoc | null) {
      s.version++;
      s.layout = layout;
    },
  };
  const body = (data: object, status = 200) => Response.json({ status: "OK", data }, { status });
  const fail = (status: number, code: string, data?: object, headers?: Record<string, string>) =>
    Response.json(
      { status: code, error_message: code, request_id: "r1", data },
      { status, headers },
    );

  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, req?: RequestInit) => {
      if (url.startsWith("/api/book-layout/get")) {
        s.gets++;
        return body({
          layout: s.layout,
          version: s.version,
          template_id: "memory-book",
          updated_at: null,
        });
      }
      if (url.startsWith("/api/media/get-list")) {
        const items = s.media.map((m) => ({
          ...m,
          yearbook_id: "Y",
          content_type: "image/jpeg",
          bytes: 1,
          created_at: 1,
          purpose: "design",
        }));
        return body({ items, next_id: "" });
      }
      if (url === "/api/media/delete") {
        const id = JSON.parse(String(req?.body)).id as string;
        s.calls.push(`delete:${id}`);
        s.deleted.push(id);
        s.media = s.media.filter((m) => m.id !== id);
        return body({});
      }
      if (url === "/api/book-layout/reset") {
        s.resets++;
        s.version++;
        s.layout = null;
        return body({});
      }
      if (url === "/api/book-layout/save") {
        const b = JSON.parse(String(req?.body));
        s.saves.push(b);
        s.calls.push("save");
        const step = s.script.shift();
        if (step) return step();
        if (b.base_version !== s.version)
          return fail(409, "ERROR_VERSION_CONFLICT", { version: s.version, layout: s.layout });
        s.version++;
        s.layout = b.layout;
        return body({ version: s.version, updated_at: 1 });
      }
      throw new Error(`unexpected ${url}`);
    }),
  );
  return { s, fail, body };
}
