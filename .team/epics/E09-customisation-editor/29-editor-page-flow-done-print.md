# E09_T-170 — Editor flow: page by page, Save and done, Print, back to the book manage page (web)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P1 · **Type:** feature
**Depends on:** none (the editor T-159..T-165 is merged). **Read first:** `web/src/book/editor/EditorShell.tsx` (header buttons, `Preview as PDF` opens `/yearbooks/:id/book`), `PageStrip.tsx`, `store.ts` (`settle()`, save state), `web/src/App.tsx` (routes `/yearbooks/:id` = book manage page `YearbookEdit`, `/yearbooks/:id/book` = print route, `/yearbooks/:id/editor`), the T-148 leave guard on the customise screen (`08-customise-leave-guard-and-not-found.md`), `docs/e2e.md`, the book's section order and hidden sections from the stage 1 design (`design.sections`).

#### Why
Owner request 2026-11-04: edit a page or the whole book page by page, save, print, then go back to the book manage page. Today the editor has a page strip, autosave and "Preview as PDF", but no way to step through the book, no "done", and no clear return to the manage page.

#### Requirements (SHALL)
1. **Page by page.** "Previous page" and "Next page" buttons (and the shortcuts you choose, listed in the help sheet and not clashing with T-160 keys) step through the book's page kinds in the book's real order (`design.sections` order; a hidden section is skipped), with a position text "Page 2 of 4: Profile" announced in the live region. The page strip highlights and scrolls to the active page. Selection is cleared on a page change, undo history is kept. The notes page says "applies to every notes page" (it is one shared layout, ADR 0004).
2. **Save and done.** A primary "Done" button: runs `store.settle()` (flush in-flight and queued saves); on success navigates to `/yearbooks/:id` (the book manage page), which shows the current state (refetch, no stale cache). If the save failed (conflict, invalid layout, offline, server error) the editor stays, the existing save-status error is shown, nothing is lost. While dirty, leaving by browser Back, a link or closing the tab asks first (the T-148 guard pattern; `beforeunload` for the tab).
3. **Print.** A "Print or save as PDF" button replaces "Preview as PDF": `settle()` first, then opens the print route in a new tab as the current button does (keep the popup-blocked fallback and the failure message). It states the page size and says the book prints all pages. Returning is easy: the editor tab stays open.
4. **Entry points.** On the book manage page, a "Design the pages" section: "Edit page by page" (opens the editor on the cover) and one link per page kind ("Edit cover", "Edit profile page", "Edit notes pages", "Edit back page", hidden sections omitted) that open the editor at that page via a deep link (`?page=<kind>`; unknown values fall back to the cover). The editor's first page is taken from the link.
5. **Reading order.** Stage 1 customise screen (`/customise`) and the editor link to each other where it helps (a student can still reach colours, fonts and section order). No change to the stage 1 screen's behaviour.
6. **Accessibility and i18n.** Every new control has a label, keyboard path and focus handling (focus returns sensibly after navigation); all strings in vi and en (`lint:i18n` green); works at 768 px and above (the editor does not mount below that today; keep that).
7. **Tests.** Unit/component: page order with hidden sections, position text, Done success/failure paths with a fake store and router, the dirty leave guard, deep link parsing; Playwright `tools/e2e/editor-flow.spec.mjs` on the real stack: manage page -> "Edit page by page" -> next through every page -> Done -> manage page shows the changes; "Edit cover" deep link opens the cover; Done with a forced 409 stays in the editor; Print opens the print route after the save settled; leaving with unsaved changes asks. Existing editor specs stay green.
8. **Docs.** `docs/` section "Editing flow: page by page, save, print" (what each button does, the guard, the deep link).
9. No new dependency; nothing under `.team/`; no API change.

#### Acceptance criteria
- [ ] AC1 — A student starts from the book manage page, edits the whole book page by page, presses Done and lands on the manage page showing their work; hidden sections are skipped.
- [ ] AC2 — Done never navigates before the save settled and never loses a failed save; the leave guard asks when dirty.
- [ ] AC3 — Print waits for the save and opens the print route; the editor tab is intact afterwards.
- [ ] AC4 — Deep links open the right page; unknown values open the cover.
- [ ] AC5 — Lint, prettier, tsc, vitest, `lint:i18n`, `check:api`, build and the e2e suite (incl. `editor-flow.spec.mjs`) green.

#### Test plan
Dev: requirement 7 on a private compose stack (non-default ports, `scripts/e2e.sh` directly). QA: runs everything on the PR head, walks the whole flow by keyboard only, tries a book with a hidden section, an offline Done, two tabs (409), the browser Back button, and prints.
