# E09_T-159 — Editor route and shell: page strip, zoom, selection overlay, property panel (web)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-158. **Read first:** `docs/adr/0004-layout-editor.md` (risk 8: selection chrome outside the page clip; Q2 phone note), `web/src/pages/Customise.tsx` and `web/src/book/CustomisePreview.tsx` (how the book is drawn and scaled), `web/src/book/layout/*`, `web/src/book/env.tsx` (`Sheet` and its layer slot), `docs/design-system` or the existing CSS tokens.

#### Why
Students need a screen to see their pages, pick an element and change it. This task builds the screen and everything that is not a drag gesture or text typing (T-160, T-161), so those tasks plug into a working editor.

#### Requirements (SHALL)
1. **Route** `/yearbooks/:id/editor`, owner only (same 404/403 handling as `/yearbooks/:id/customise`; no retry loop). A button "Free layout" on the customise screen opens it; a back link returns. Layout: page strip on the left, stage in the middle, property panel on the right. Below 768 px wide show a clear read-only message with a link back to customise (phone mode is a later task).
2. **Page strip**: one thumbnail per page in book order (cover, profile, notes pages, back) drawn by the real book components at small scale; click selects the page; the current page is marked; arrow keys move between pages.
3. **Stage**: the selected page drawn by the real book renderer with the layout runtime (no copy of the templates). Zoom: fit-to-window (default) and 50-200 % with buttons and Ctrl/Cmd + wheel; the page keeps its paper aspect for A5, A4 and Letter (paper chosen like the customise screen does). The safe margin from the manifest (`design.layout.safe_margin_mm`) is drawn as a dashed overlay that can be toggled.
4. **Selection overlay**: an element layer positioned over the page and **outside** the page's `overflow: hidden` clip. It draws the selection outline and eight resize handles and a rotate handle (shapes only: handles are visible and focusable, but the drag behaviour is T-160). Click an item or an editable template element (by its layout id) to select, click empty stage to clear, `Tab` cycles through items in draw order. Overlapping items: click picks the topmost.
5. **Property panel** for the selection: x, y, width, height (shown as % of the page, step 0.1) and rotation (degrees) with numeric inputs that call `setItemBox`/`setOverride` through the T-158 store (one history step per committed input); z-order buttons (to front, forward, backward, to back); "hide/show" for a template element; "delete" for an item; "reset this element" for a template element (drops its override). For text items a read-only preview of the text (editing is T-161). Nothing selected: page properties (background colour or image comes in the palette task; here only a "clear background" button if one is set).
6. **Toolbar**: undo, redo (buttons and Ctrl/Cmd+Z, Ctrl/Cmd+Shift+Z, Ctrl+Y), the `SaveStatus` from T-158, "Reset all layout" with a confirmation dialog (calls the T-158 reset), "Preview as PDF" (reuse the existing preview/export action of the customise screen).
7. **Conflict banner**: in state `conflict` show a banner with the two actions of T-158 (use the saved version / keep my version) and block further edits until chosen; in `invalid` show the problem; in `gone` show a "this book no longer exists" screen.
8. **Accessibility basics**: every control has an accessible name (i18n `vi` and `en`), the stage has a role and label, selection changes are announced through a polite live region, focus is visible and never trapped. Keyboard move/resize/rotate shortcuts belong to T-160.
9. Tests: component tests (selection, Tab order, panel inputs update the store and history, undo/redo shortcuts, conflict banner, narrow-width message, i18n parity), and one Playwright journey `tools/e2e/editor-shell.spec.mjs` (open editor, select the cover title, change its width in the panel, undo, reload and see the saved layout) that runs in `scripts/e2e.sh`.
10. Nothing under `.team/`; no server change.

#### Acceptance criteria
- [ ] AC1 — The journey of requirement 9 passes in `scripts/e2e.sh` together with the existing journeys.
- [ ] AC2 — Handles of an element flush to the page edge are visible and reachable (overlay outside the clip), on A5, A4 and Letter.
- [ ] AC3 — The stage renders pixel-identically to the book preview for the same document (reuse the print-check parity method on the stage at zoom 100 %; layout adds under 0.5 % on top of the template baseline).
- [ ] AC4 — Lint, prettier, tsc, vitest, `lint:i18n`, `check:api`, build green; keyboard-only run through select, panel edit, undo works.
- [ ] AC5 — Below 768 px the editor does not mount and the message is shown.

#### Test plan
Dev: component tests, journey, print-check run. QA: runs everything on the PR head (including `make e2e` and `make print-check`), checks A5/A4/Letter and Vietnamese labels, the conflict banner from a second tab, and keyboard-only operation.
