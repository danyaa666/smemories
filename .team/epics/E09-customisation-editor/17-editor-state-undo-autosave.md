# E09_T-158 — Editor state: document store, undo/redo, autosave and conflict handling (web)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-156 (the server version never goes back: the conflict rule below relies on it), T-154 (merged). **Read first:** `docs/adr/0004-layout-editor.md` (Q4 undo, "Changes this implies" 5), `.team/epics/E09-customisation-editor/12-layout-document-model-and-api.md` (the document and the endpoints), `web/src/book/layout/runtime.tsx` and `geometry.ts` (types, `place`/`unplace`), `docs/api-contract.md` (`book-layout/*`), `web/src/queryClient.ts`.

#### Why
Every editor feature (gestures, text, palette, history) edits one layout document and must undo, redo and save it the same way. This task builds that engine with no editor UI, so it can be proved by tests before screens exist.

#### Requirements (SHALL)
1. **Module** `web/src/book/editor/` with a framework-free core (`document.ts`: pure immutable operations; `history.ts`: the stack) and one React hook `useLayoutEditor(yearbookId)` (loads `book-layout/get`, exposes state and actions). No new dependency in `web/`.
2. **Pure operations** on the layout document (v1): `setItemBox`, `addItem` (text, asset, image), `removeItem`, `reorderItem` (draw order = array order), `setOverride` (move/resize/hide a template element by its layout id), `setBg`, `setText`. They return a new document sharing unchanged parts by reference. They enforce the client-side mirror of the server limits (40 items per page kind, 150 in total, 500-code-point text cap, box bounds from `place`/`unplace`); a refused operation returns the same document and a typed reason. The server stays the authority.
3. **History (undo/redo)**: a snapshot stack of document references, depth 100, one step per user action. A gesture is `begin()`, many `preview(change)` (render only, no step, no save) and one `commit()` (one step). `undo()`, `redo()` and a new commit after undo drop the redo tail. Undo/redo never touches the server directly; they mark the document dirty like any edit.
4. **Autosave**: after the last commit wait 1.5 s (debounce), then save with `base_version` = the last version the client knows; at most one request in flight (a commit during a save queues one more save of the newest document). On 200 store the new version. `flush()` saves now. While dirty or saving, warn on `beforeunload` and on in-app leave (reuse the pattern of the customise leave guard, T-148). A page hide (`visibilitychange` to hidden) triggers `flush()`.
5. **Conflict (409)**: the response carries the stored document and version. The editor enters state `conflict` and stops autosaving; it never overwrites silently. Two explicit actions: `useTheirs()` (replace the local document with the stored one, clear history) and `keepMine()` (re-save the local document with the stored version as `base_version`). Both are reachable from the hook for T-159 to put on screen.
6. **Other save failures**: 429 waits `Retry-After` then retries; network or 5xx errors retry with backoff (1 s, 2 s, 4 s, 8 s, then every 30 s) and keep the document dirty; 400/413 (invalid document) go to state `invalid` with the problem path from the response and stop autosaving until the next commit; 404/403 go to state `gone` (reuse `makeQueryClient`'s no-retry rule, no retry loop).
7. **State exposed**: `doc`, `version`, `dirty`, `saveState` (`idle`, `saving`, `saved`, `offline`, `conflict`, `invalid`, `gone`), `canUndo`, `canRedo`, `selection` (ids; set/clear actions; selection is not part of history). A small `SaveStatus` component renders the state with i18n keys in `vi` and `en` (parity lint must pass).
8. **Reset** (`book-layout/reset`) is an action that clears history and replaces the document; it asks for no confirmation itself (T-159 shows the dialog).
9. Tests (vitest, fake timers, mocked fetch): every pure operation including refusals; undo/redo/redo-tail; gesture preview/commit gives one step; depth 100; debounce, one in flight and the queued second save; 200, 409 (both choices), 429, offline retry, 400, 404; unsaved-leave guard; the ABA case (stale base version gets 409 and the editor does not overwrite).
10. Nothing under `.team/`; no server change; no new UI route.

#### Acceptance criteria
- [ ] AC1 — The tests of requirement 9 pass; web lint, prettier, tsc, vitest, `lint:i18n`, `check:api` and build are green.
- [ ] AC2 — A scripted session (test) of 30 edits, 10 undos, 5 redos and a conflict ends with the expected document, version and `saveState` and no lost update.
- [ ] AC3 — The hook never sends two saves at once and never saves while in `conflict` or `invalid`.
- [ ] AC4 — A document returned by the operations passes the server validation (a round-trip test against the `book-layout/save` contract fixtures).

#### Test plan
Dev: unit tests and one hook test against a mock server. QA: reviews the state machine against requirement 4-6, runs the suite, and on a live stack drives the hook from a small test page or Playwright script: edit, undo, offline, reconnect, conflict from a second tab (both choices).
