# E09_T-188 — Editor screen-reader polish: numbers, live regions, roles, handles

**Epic:** E09-customisation-editor · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** feature
**Depends on:** T-182 (merged). Source: findings **F-10, F-13, F-14, F-15, F-16, F-17** in [docs/editor-accessibility.md](../../../docs/editor-accessibility.md);
each row there has the proposed fix. F-11 and F-12 stay accepted (design pass), not in this task.

#### Requirements (SHALL)
1. **F-10:** numbers in announcements and in the Left/Top/Width/Height fields use `Intl.NumberFormat(language)` (decimal comma in vi);
   typing either separator in vi still parses.
2. **F-13:** the "Save as template" name counter is not announced on every key: silent until the limit is near or passed.
3. **F-14:** the palette search count is announced once after typing pauses (about 600 ms), not per key.
4. **F-15:** template cards and history rows use a correct pattern: the Apply and Delete buttons are not inside the `radiogroup`,
   or the group becomes a list of buttons; arrow keys and Enter behave as the chosen pattern says.
5. **F-16:** resize and rotate handles are `aria-hidden` (keyboard and panel remain the alternative); the e2e gesture specs still find them.
6. **F-17:** the "A5 (148 × 210 mm)" pattern comes from the locale files (`editor.printHint`).

#### Test plan
- vitest per item (formatter and parser in vi, counter silence, debounce with fake timers, roles, aria-hidden).
- e2e: editor-a11y and editor-gestures specs green; aria snapshot updated deliberately, en and vi.
- Mark each row in `docs/editor-accessibility.md` "fixed in T-188".
