// A mock book-layout server for the editor tests: the real optimistic-concurrency rule (save needs base_version == version) and a script of
// canned answers (an error, a network failure, a held response) that is consumed one per save before the rule applies.
import { vi } from "vitest";
import type { LayoutDoc } from "./document";

type Step = () => Response | Promise<Response>;

export type MockMedia = { id: string; width: number; height: number };
/** A history snapshot (T-163): `layout` is the document it holds. */
export type MockVersion = {
  id: string;
  version: number;
  kind: "auto" | "reset" | "restore";
  created_at: number;
  layout: LayoutDoc;
};

export function mockServer(init: {
  version: number;
  layout: LayoutDoc | null;
  /** The book's design images (media/get-list with purpose=design). */
  media?: MockMedia[];
  /** The book's history, newest first (version-list / version-get / restore). */
  versions?: MockVersion[];
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
    versions: [...(init.versions ?? [])],
    /** Calls of the three history routes, in order: "list", "get:<id>", "restore:<id>@<base_version>". */
    history: [] as string[],
    restores: [] as { id: string; base_version: number }[],
    /** Answers for the next restore calls (consumed one per call); `dropped` is what the next successful restore reports. */
    restoreScript: [] as Step[],
    dropped: 0,
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
      if (url === "/api/book-layout/version-list") {
        s.history.push("list");
        return body({
          next_id: "",
          items: s.versions.slice(0, 30).map((v) => ({
            id: v.id,
            version: v.version,
            kind: v.kind,
            created_at: v.created_at,
            item_count: Object.values(v.layout.pages).reduce(
              (n, p) => n + (p.items?.length ?? 0),
              0,
            ),
            bytes: JSON.stringify(v.layout).length,
          })),
        });
      }
      if (url === "/api/book-layout/version-get") {
        const id = JSON.parse(String(req?.body)).id as string;
        s.history.push(`get:${id}`);
        const v = s.versions.find((x) => x.id === id);
        return v ? body(v) : fail(404, "ERROR_NOT_FOUND");
      }
      if (url === "/api/book-layout/restore") {
        const b = JSON.parse(String(req?.body)) as { id: string; base_version: number };
        s.history.push(`restore:${b.id}@${b.base_version}`);
        s.restores.push(b);
        const step = s.restoreScript.shift();
        if (step) return step();
        const v = s.versions.find((x) => x.id === b.id);
        if (!v) return fail(404, "ERROR_NOT_FOUND");
        if (b.base_version !== s.version)
          return fail(409, "ERROR_VERSION_CONFLICT", { version: s.version, layout: s.layout });
        if (s.layout)
          s.versions.unshift({
            id: `R${s.version}`.padEnd(26, "0"),
            version: s.version,
            kind: "restore",
            created_at: Date.now(),
            layout: s.layout,
          });
        s.version++;
        s.layout = v.layout;
        return body({ version: s.version, updated_at: 1, layout: v.layout, dropped: s.dropped });
      }
      throw new Error(`unexpected ${url}`);
    }),
  );
  return { s, fail, body };
}
