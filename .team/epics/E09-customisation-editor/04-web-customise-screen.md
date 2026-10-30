# E09_T-144 — Web: the customise screen

**Epic:** E09-customisation-editor · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-143. **Read first:** PRD "Stage 1 plan" (owner: desktop and tablet first, phone usable but basic), the yearbook pages and navigation in `web/src/`, the T-129 assemble screen (keyboard reorder pattern, live region, error banner), `TemplatePicker.tsx`, `BookPages`, `book.css` (sheets are boxes in design px scaled to the paper), `GET /api/book-design/get`, `update`, `reset` (T-141), the i18n parity check.

#### Requirements (SHALL)
1. **Route** `/yearbooks/:id/customise` (owner only; others get the not-found screen) with a link "Customise the design" on the yearbook page and next to the template picker. The screen loads `book-design/get` and the book data once.
2. **Layout.** Desktop/tablet: controls on the left, live preview on the right. Phone: one column, controls first, preview below (the preview may scroll); no drag on small screens, move up/down buttons everywhere; targets >= 44 px.
3. **Controls.** Theme swatches (name + colour chips, selected state in text not colour only), font sets (name + a sample line in that font), sections list (show/hide switch per hideable section, move up/down per movable one, fixed ones shown as locked), text fields for each manifest text with rune counters (T-135 `cutRunes`) and the template default as placeholder, image choice per manifest image. Labels in en and vi; names of themes/fonts/sections come from the template's i18n, not the server.
4. **Live preview.** Reuse the real book render (`BookPages` pieces) at screen scale (CSS zoom/transform of the design box), showing a page strip: the page(s) the focused control affects (cover for title/pattern, profile, one notes page, back), not the whole book; no printing from this screen. Photos in the preview use the same eager/queued loading rules (never more than 16 in flight).
5. **Save.** Explicit "Save" and "Reset to template" (confirm names what is lost); unsaved-changes guard on leaving; a 400 `ERROR_INVALID_DESIGN` shows the server message on the field; success announces via a live region. The screen applies unsaved choices to the preview immediately (the preview is driven by local state, not by the server).
6. **Template change.** If the owner changes the template, the screen says the design was reset (T-141) and loads the new template's choices.
7. Both locales, parity, accessibility basics (labels, focus order, live region, keyboard-only use), no new dependency, vitest for each control and state (including the exact `update` payload).

#### Acceptance criteria
- [ ] AC1 — Choosing a theme, a font set, hiding a section, reordering, editing a text and an image updates the preview and saves exactly the validated document.
- [ ] AC2 — Reload shows the saved design; Reset returns to the template look; leaving with unsaved changes asks first.
- [ ] AC3 — Keyboard-only: every control and the reordering work; focus stays on the moved item; live region announces.
- [ ] AC4 — A non-owner sees NotFound and no design request is made; phone width has no horizontal scroll.
- [ ] AC5 — Lint, typecheck, build, vitest, i18n parity, check:api; CI green (jobs that run).

#### Test plan
QA walks the screen at 1280, 768 and 375 px in both locales, saves, reloads, prints the book to PDF and compares with the preview.
