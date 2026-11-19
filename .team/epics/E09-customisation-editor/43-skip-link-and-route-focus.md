# E09_T-187 — Skip link and focus on route change (app-wide)

**Epic:** E09-customisation-editor · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-182 (merged). Source: findings **F-08** and **F-09** in [docs/editor-accessibility.md](../../../docs/editor-accessibility.md).

#### Description
The T-182 audit found two app-wide keyboard and screen-reader gaps: 14 Tab stops (brand, sidebar, account, language, footer) come before
the content on every screen, and a route change leaves focus where the removed link was, so screen readers do not hear the new page.

#### Requirements (SHALL)
1. **F-08:** a "Skip to content" link SHALL be the first Tab stop of `App` and `YearbookLayout`, visible on focus, targeting `<main>`
   (en and vi strings in the locale files).
2. **F-09:** after every client-side navigation, focus SHALL move to the new page's `<h1>` (`tabindex="-1"`) and the page title SHALL be
   announced once. Not on the first load, not on in-page state changes (dialogs, tabs).
3. No visual change except the skip link on focus; no change to the editor's own focus handling from T-182.

#### Test plan
- vitest: skip link is first and moves focus to `<main>`; navigation focuses the `<h1>`; first load does not.
- e2e: extend `tools/e2e/editor-a11y.spec.mjs` (or a new `app-a11y.spec.mjs`) with a keyboard-only route change on two screens, en and vi.
- Update the F-08 and F-09 rows in `docs/editor-accessibility.md` to "fixed in T-187".
