# E09_T-153 — Layout document model and API (backend)

**Epic:** E09-customisation-editor (stage 2) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-150 (ADR 0004, merged), T-151 (asset registry, merged), T-152 (design images: the `design` purpose; merge it first). **Read first:** [ADR 0004](../../../docs/adr/0004-layout-editor.md) (units, "Changes this implies" 1), E09 PRD "Stage 2 plan", T-141 spec `01-design-model-and-api.md` and the code in `internal/bookdesign/` and `migrations/20261030120000_book_design_tab.sql` (mirror their structure, error codes, tests and docs), `docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`, the skills in `.agents/skills/`, `internal/assets/`.

#### Why
The editor (stage 2) stores what a student places on the pages of a book: moved or hidden template elements, added text boxes, stickers, shapes and images, and a page background. This task defines that document, validates it on the server, stores it with optimistic concurrency, and hands it to the print route. It changes no web code.

#### The document (v1), stored per yearbook as JSON, at most 128 KiB
```json
{"v":1,"pages":{
  "cover":{"bg":{"asset":"shape.x"},
           "overrides":{"cover.photo":{"x":120,"w":500,"h":500,"cy":400,"r":0,"hidden":false}},
           "items":[{"id":"a1b2c3","t":"text","x":100,"w":400,"h":120,"cy":850,"r":-4,"text":"Lớp 12A1","font":"display","size":28,"color":"accent","align":"center","bold":true},
                    {"id":"d4e5f6","t":"asset","asset":"sticker.star","tint":"accent","x":800,"w":150,"h":150,"cy":100,"r":15},
                    {"id":"g7h8i9","t":"image","media":"<ulid>","fit":"cover","x":0,"w":300,"h":200,"cy":500,"r":0}]},
  "profile":{...},"notes":{...},"back":{...}}}
```
- **Boxes (ADR 0004 Q1 "centre")**: `x`, `w`, `h` integers in thousandths of the page width, `cy` the vertical centre in thousandths of the page height, `r` degrees about the centre (integer, -180..180). Items are drawn in array order (later on top), so there is no `z`.
- **Page kinds** come from the template manifest: for Memory Book `cover`, `profile`, `notes` (repeats on every notes page) and `back`. Each has `bg` (optional: `{"asset":id}`, `{"media":id}` or `{"color":token}`), `overrides` (editable template elements by id: `x,w,h,cy,r,hidden`, all optional) and `items`.
- **Item types**: `text` (`text` at most 500 runes counted as code points, `font` one of the manifest's `text_fonts`, `size` pt within the manifest's range, `color` a token from the manifest's `colors`, `align` left|center|right, `bold`), `asset` (`asset` an id of the registry in `internal/assets`; `tint` a colour token, only if the asset is `tintable`), `image` (`media` the id of one of the owner's own `design` images of this yearbook; `fit` cover|contain; `radius` 0..500). Item `id` is a client-made string of 6..26 chars `[A-Za-z0-9]`, unique within the document.
- **Limits**: at most 40 items per page kind and 150 in the document; `w` and `h` from 10 to 2000; a box may leave the page by at most half of its size (so `x` is within `[-w/2, 1000-w/2]`, same rule for `cy` with `h/2` against the page height: use the paper-independent form `cy` within `[-h/2·k, 1000+h/2·k]` with the A5 height ratio and say which in the code comment); unknown keys, unknown ids and wrong types are errors; text is plain (never HTML), control characters other than newline are rejected.

#### Manifest (`design.layout`, `internal/templates/embed/memory-book.json`)
A new optional block, validated when the manifest loads: `page_kinds` (`id`, `repeats`, `editable` element ids), `text_fonts` (`display`, `body`), `colors` (theme token names), `text_size` (`min`, `max` pt), `safe_margin_mm`. For Memory Book start with `cover`: `cover.title, cover.photo, cover.belongs, cover.sticker`; `profile`: `profile.heading, profile.photo`; `notes`: `notes.heading`; `back`: `back.heading, back.subtitle, back.pact`. **These ids are proposals**: the web runtime task (T-154) corrects them to what the template can really move and edits this block in its own PR; the validation here reads whatever the manifest says. Navy Classic gets its block later (separate task).

#### Requirements (SHALL)
1. **Table** `book_layout_tab` (new migration, goose, up and down): `yearbook_id` (unique), `template_id`, `layout_json` (JSON text up to 128 KiB), `version` (BIGINT, starts at 1, +1 per save), `updated_at` (BIGINT ms); conventions of `docs/db-conventions.md` (`_tab`, BIGINT ms timestamps, no foreign keys, ULID public ids as in `book_design_tab`).
2. **Package** `internal/booklayout` (handler, service, store, document validation), wired in `cmd/smemories-api`, mirroring `internal/bookdesign`. Validation is a pure function `Validate(doc, manifest, registry, ownMediaIDs)` returning all problems (path and code, at most 20 reported), unit-tested table-driven including every limit above at the boundary and one over, Vietnamese and emoji text (rune counting), `<script>` text kept as text, duplicate item ids, an unknown asset id, a non-tintable asset with a tint, a media id that is not the owner's `design` image, and an override for an element the manifest does not list.
3. **Endpoints** (v2 contract, owner only; a non-owner, a class admin and an anonymous caller get 404, like `book-design`):
   - `GET /api/book-layout/get?yearbook_id=` returns `data: {layout, version, template_id, updated_at}`, `layout: null` and `version: 0` when none.
   - `POST /api/book-layout/save` body `{yearbook_id, base_version, layout}`: `base_version` must equal the stored version (0 for the first save); the update is one conditional statement (`WHERE version = ?`), so two tabs cannot both win. 200 returns `{version, updated_at}`. On a mismatch: 409 `ERROR_VERSION_CONFLICT` with `data: {version, layout}` of the stored one. Invalid document: 400 `ERROR_INVALID_LAYOUT` with the problem list. Over 128 KiB: 413 `ERROR_TOO_LARGE` (cap the request body before decoding).
   - `POST /api/book-layout/reset` body `{yearbook_id}` deletes the row (idempotent).
   - Rate limit saves per user (default 60 per minute, burst 20, configurable as `SMEM_RATE_LAYOUT_SAVE_PER_MINUTE`, documented in `docs/production-config.md`, `.env.example`, and the limiter table in `docs/redis.md`); `429 ERROR_RATE_LIMITED` with `Retry-After`.
4. **Template change** (existing yearbook update path that resets the stage 1 design) also deletes the layout; answers, photos and design images survive. Test it.
5. **Print route**: `book-data/get` carries `layout` (validated; parts that no longer validate, for example an asset removed from the registry or a deleted media id, are dropped on read instead of failing the page) and `layout_version`. The response stays within the existing size expectations; add a test with a 128 KiB layout.
6. **Deletion and export**: deleting a yearbook or an account deletes the layout; the data export includes it (tests, docs).
7. **Observability**: counters `smem_layout_saves_total{result}` (ok, conflict, invalid, too_large, rate_limited), no document content in logs.
8. Docs: `docs/api-contract.md` (endpoints, errors), `docs/templates.md` (the `design.layout` block and the document), `docs/class-privacy-review.md` (a layout is the owner's design data, readable by a class admin only through the class book, see the class task), `api/openapi.yaml` and the generated web schema (`check:api` clean).
9. **Narrow the class admin's read of design images (carry-over from T-152, 2026-10-31).** T-152 lets a class admin open every `design` image of a linked member's book, including images no layout uses, because the layout table did not exist. Now that it does: `media.Store.readable` lets a class admin read a `design` image only if the member's stored layout references it (`bg.media` or an `image` item), the owner always can; extend the class matrix (referenced: admin 200; unreferenced: admin 404; other callers 404) and update `docs/class-privacy-review.md` (section 1 and the endpoint list). Note for the editor tasks: the `display` variant of a design image is 3000 px (about 256 dpi on A4), the `print` variant (1800 px) is too small for a full-bleed background, so the layout asks for `display`. Also (QA, 2026-10-31): `media.Service.DesignImages` is the only check that a media id is a `design` image and not a profile photo, so the layout save path MUST call it for every `image` item and `bg` media id before storing the document, and refuse the whole save with a precise problem path otherwise (test: a profile-photo id and another user's design image id are both rejected).
10. Nothing under `.team/`; no web code except the regenerated API schema file.

#### Acceptance criteria
- [ ] AC1 — A valid document saves, reads back identical and bumps the version; the same `base_version` used twice gives 200 then 409 with the stored document (test with two concurrent saves: exactly one wins).
- [ ] AC2 — Every limit and every invalid case in requirement 2 is rejected with a precise problem path; a 128 KiB document passes and 128 KiB + 1 byte is 413 before decoding.
- [ ] AC3 — Non-owner, class admin and anonymous get 404 on all three endpoints (class matrix script extended).
- [ ] AC4 — Template change, yearbook deletion, account deletion and export behave as in requirements 4 and 6.
- [ ] AC5 — `book-data/get` returns the layout and drops parts that no longer validate; migration up and down tested with existing rows.
- [ ] AC6 — A class admin reads a member's design image only while the layout references it (matrix script covers both cases).
- [ ] AC7 — `go test ./...`, `make test-integration` for the touched packages, lint, `check:api`, `scripts/check-class-matrix.sh` pass.

#### Test plan
QA hits the endpoints with valid and hostile documents (oversize, deep nesting, unicode edge cases, wrong types), races two saves, walks the class matrix, checks template change and deletion, and runs the migration both ways. The Go skills' checklists apply (`be-api-design`, `be-rldb`, `be-golang`).
