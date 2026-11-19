# Accessibility and i18n of the layout editor (T-182, E09 stage 2c)

The editor was built panel by panel. This is the audit of the whole: what a keyboard user can do, what a screen reader is told, how it looks at 200 % zoom, on a touch tablet and in Vietnamese,
what was fixed in T-182 and what is left (the findings table), and what only a person with the real tools can prove (the owner checklist at the end).

## The keyboard map

Every gesture has a keyboard way (ADR 0004). Items on the page are reached with Tab (the focused item is the selected one); the shortcuts below work on a focused item. `?` shows the same list in the editor
(`gestures/keys.ts` is the one table, `gestures/HelpSheet.tsx` draws it, in English and Vietnamese).

| Keys | What it does | The same in the panel |
| --- | --- | --- |
| Tab, Shift+Tab | next or previous control; the items of the page are one stop each, in drawing order | Layers list on a phone |
| Arrows | move the item 1 design px (10 with Shift) | Left, Top fields; the move arrows on a phone |
| Alt+Shift+Arrows (Mac: Option+Shift) | resize by 10: Left/Right the width, Up/Down the height | Wider, Narrower, Taller, Shorter; Width, Height fields |
| `,` and `.` (Shift: 15) | rotate left, right by 1 degree | Rotate left/right 1 and 15; Rotation field |
| Delete, Backspace | delete the selected item | Delete |
| Enter on a text item | edit the text; Esc ends it and returns to the item | the text field of the panel |
| Esc | cancels a drag, ends text editing, closes the open panel or preview, or clears the selection | Close, Back to current |
| Ctrl+Z, Ctrl+Shift+Z or Ctrl+Y (Mac: Cmd) | undo, redo | Undo, Redo |
| Ctrl+D | duplicate | Duplicate |
| Page Up, Page Down | previous, next page of the book | Previous page, Next page, the page strip |
| `?` | show or hide the shortcut list (Esc closes it and returns to where it was opened) | Keyboard shortcuts |
| Stacking order | none | Bring to front, Bring forward, Send backward, Send to back |

Inside lists that behave like one control (palette tiles, template cards, history rows, the page strip) the arrows move inside the list and Tab leaves it; Enter or Space picks.
Panels (Templates, History) take the focus when they open and give it back to their toolbar button when they close; dialogs start on Cancel, keep Tab inside, close with Esc and return the focus to what opened them.

## The announcements (the polite live region of the editor)

One visually hidden `role="status" aria-live="polite"` region (`EditorBody`, `say`) carries every message; the save status and the alerts have their own regions. Messages are in `en.json` and `vi.json` under `editor.*`.

| When | Message key | Rate |
| --- | --- | --- |
| an item is selected, the selection is cleared | `selected`, `cleared` | once per change |
| arrow, resize or rotate keys | `said.leadMove`, `said.movedTo`, `said.resized`, `said.rotated` | **first key at once, then at most one message per 500 ms with the latest place** (T-182); 20 presses in a row are 2 messages (`useGestures.test.tsx`) |
| end of a pointer drag, resize, rotate; Esc during it | `said.movedTo`, `said.resized`, `said.rotated`, `said.cancelled` | once, at the end; never while dragging |
| a panel nudge button (rotate, wider, ...) | `nudged.*` | once per press |
| delete, duplicate, reorder, image replaced | `deleted`, `duplicated`, `reordered`, `image.replaced` | once |
| text box added | `textAdded` | once |
| page changed | `pageOf` (+ `notesShared`) | once |
| palette: item added, background set or removed, upload finished, image deleted | `palette.added`, `palette.bg*`, `upload.finished`, `deleteImage.done` | once |
| History: preview, back to current, restored (+ dropped count) | `history.previewingShort`, `history.backDone`, `history.restored` | once |
| Templates: preview, back, applied (+ dropped), saved, deleted, gone | `templates.previewingShort`, `templates.backDone`, `templates.applied`, `templates.save.*`, `templates.deleted`, `templates.err.gone` | once |
| reset all layout, invalid items removed | `resetDone`, `invalidRemoved` | once |
| Done could not save | `doneFailed` (also an `alert`) | once |

The save status (`SaveStatus`) shows "Saving..." and "Saved" at every autosave but does **not** announce them (T-182; a screen reader would repeat them after every pause in the editing). It announces, politely, what the
student has to act on: offline, failing (retrying), conflict, invalid, gone. Errors and refusals are `role="alert"` (assertive): the refusal notice, "Done could not save", "Print could not save", a failed restore, apply or save-as-template, a conflict or invalid banner.
Nothing is announced on a pointer drag until it ends, so a drag cannot flood the region.

## What was tested, and how

| What | Where | Result |
| --- | --- | --- |
| The whole editor by keyboard only (no mouse), focus ring on every stop, order of the stops, no trap, focus returns to the opener | `tools/e2e/editor-keyboard.spec.mjs` (run 3 times in a row) | passes |
| Accessibility tree of 9 states (idle, selected, text editing, palette, Templates, template preview, History, help, confirm dialog, error) in English and Vietnamese: no unnamed control, landmarks, dialog `aria-modal`, selected state, status and alert | `tools/e2e/editor-a11y.spec.mjs` (`ariaSnapshot()`; attached to the report, not committed) | passes |
| Colour contrast of every text and bordered control of the chrome in 10 states | `editor-a11y.spec.mjs` (canvas-resolved colours, 4.5:1 text, 3:1 large text and controls) | passes; disabled controls and the page strip's thumbnails are listed as F-11 and F-12 |
| Touch targets at 768 and 1024 px with a touch screen (`pointer: coarse`), 8 states | `editor-a11y.spec.mjs` | passes after F-06 |
| 200 % browser zoom (a 640 px page: the phone editor) and 200 % text size at 1280 px | `editor-a11y.spec.mjs` | passes |
| `prefers-reduced-motion` | `editor-a11y.spec.mjs` (computed animation and transition) | passes after F-07 |
| Vietnamese at 768, 1024 and 1440 px, 9 states: no clipped, overlapping or sideways-scrolling control; screenshots attached | `editor-a11y.spec.mjs` | passes |
| Language switch in the middle of an edit keeps the selection, the document and the text | `editor-a11y.spec.mjs` | passes |
| Announcement bound for 20 key steps | `web/src/book/editor/gestures/useGestures.test.tsx` | 2 messages |
| No hard-coded user-facing string in `web/src/book/editor/` and `pages/Editor*.tsx` | `web/src/book/editor/hardcoded.test.ts` (TypeScript AST: JSX text and `aria-label`, `title`, `placeholder`, `alt`) | passes |
| Counted messages for 0, 1, 2, 5, 21 in English (one/other) and Vietnamese (one form) | `web/src/book/editor/counts.test.ts` | passes |
| Keys of the two locales agree | `npm run lint:i18n` | passes |

Vietnamese labels are up to 1.9 times as long as English for the shortest ones ("Moved left" / "Đã chuyển sang trái"), 1.4 to 1.6 for most buttons; the toolbar wraps onto a second row instead of clipping at 768 px.
Dates in History and Templates use the page language (`Intl`, `toLocaleString`). Plurals: English has "1 item" / "2 items" (`_one`, `_other`); Vietnamese has one form (`_other` serves every count).
The phone shell (T-181) is covered by the zoom test, the contrast and tree checks use the desktop shell; the phone's own aria roles are tested in `EditorPhone.test.tsx` and `editor-phone.spec.mjs`.

## Findings

Severity: **high** = keyboard trap, unnamed control, lost focus, unreadable contrast, hard-coded string (fixed in T-182); **medium** = hurts a real user, has a workaround; **low** = polish.
Size: XS under an hour, S half a day, M a day or more.

| Id | Severity | Finding | Status |
| --- | --- | --- | --- |
| F-01 | high | A button the keyboard is on became disabled by its own action (Undo at the first step, Redo at the last, "Send to back" at the bottom): the browser dropped the focus and a screen reader lost its place | **fixed**: `keepFocus.ts` moves the focus to the nearest enabled button of the group; `editor-keyboard.spec.mjs` steps 5 and 6 |
| F-02 | high | The keyboard help closed (Esc or Close) without returning the focus to what opened it; the focus fell to the page | **fixed**: `HelpSheet` gives it back |
| F-03 | high | The shortcut list showed English words in the Vietnamese editor ("Alt (Mac: Option) while dragging", "Shift while resizing", "Shift while rotating", ", and .") | **fixed**: `editor.helpKeys.*` in both languages; `hardcoded.test.ts` guards the JSX |
| F-04 | medium | Every arrow press announced the new place: 20 presses were 20 messages | **fixed**: at most one message per 500 ms (AC3) |
| F-05 | medium | "Saving..." and "Saved" were announced at every autosave | **fixed**: shown, not announced; problems are announced |
| F-06 | medium | On a touch tablet (768 to 1024 px) the toolbar, the stage bar and the panel buttons and fields were 36 px high, the "Print or save as PDF" link 24 px, the swatches 32 px, the help's Close 21 px | **fixed**: 44 px under `pointer: coarse` (the dialog buttons of Templates were done in T-176; the Templates/History dialogs were measured too) |
| F-07 | low | No `prefers-reduced-motion` rule: the loading shimmer and the button colour transition always ran | **fixed** in `ui.css` (the shimmer carries no information) |
| F-08 | medium | There is no "skip to content" link: 14 Tab stops (brand, sidebar, account, language, footer) come before the editor's toolbar on every screen of the app | fixed in T-187: a "Skip to content" link is the first stop of `App` and `YearbookLayout` and moves focus to `<main>` |
| F-09 | medium | A route change does not move the focus: after "Edit page by page" the next Tab starts where the removed link was (mid-page). Screen readers do not hear the new page | fixed in T-187: after each client-side navigation (not the first load) focus moves to the new page's `<h1>` (tabindex -1), which the reader announces |
| F-10 | low | Numbers in announcements and in the Left/Top/Width/Height fields use a decimal point ("12.4") also in Vietnamese, which writes a decimal comma | fixed in T-188: the Left/Top/Width/Height fields are text inputs (`role="spinbutton"`, arrows step) that write and read the language's separator; announcements use the i18next number format |
| F-11 | low | Disabled buttons are drawn at 50 % opacity: text 3.06:1, border 1.7:1. Disabled controls are exempt in WCAG 1.4.3 and 1.4.11 | accepted; proposal: a grey that keeps 4.5:1 instead of opacity, size XS, when a design pass happens |
| F-12 | low | The unselected thumbnails of the page strip have no border of their own (non-text contrast 1:1); the picture of the page is the only boundary. The current one has a 3 px outline | accepted; proposal: a 1 px border in `--color-border`, size XS |
| F-13 | low | The name counter of the "Save as template" dialog ("5 of 60 characters") is a polite live region and speaks on every key | fixed in T-188: `aria-live` is `off` until 5 characters from the limit |
| F-14 | low | The palette search count ("12 found") is a polite live region and speaks on every key | fixed in T-188: the count text changes 600 ms after typing pauses |
| F-15 | low | The template cards and history rows are `role="radio"` buttons inside a `radiogroup` that also holds Apply and Delete buttons (not radios); arrows move the focus but not the checked state (Enter previews) | fixed in T-188: cards and rows are buttons (`aria-pressed`) in a `group`, Apply and Delete are no longer inside a radiogroup |
| F-16 | low | The resize and rotate handles are `button`s in the accessibility tree (`tabindex=-1`, 9 per selected item) that only work with a pointer; a screen reader's virtual cursor finds them | fixed in T-188: the handles are `aria-hidden`; e2e finds them by `getByLabel` |
| F-17 | low | The decimal separator and the "×" in "A5 (148 × 210 mm)" are fixed in the code, not in the locale files | fixed in T-188: `editor.printHint` in the locale files holds the whole pattern |

Not covered: the phone shell was checked for zoom, targets (T-181 CSS) and tree by the existing T-181 tests, not re-audited here; real screen readers, real keyboards in Vietnamese Telex/VNI and real touch (below).

## Owner manual checklist (what automation cannot prove)

Use a Memory Book in Vietnamese with a cover title, one photo and one text. In every list "expect" is what the screen reader should say (wording differs a little between readers); a step that fails is a finding.

### VoiceOver on macOS (Safari)

1. Turn VoiceOver on (Cmd+F5), open the editor from "Edit page by page". Expect: the page title, then the editor toolbar when you move into it ("Editor toolbar, toolbar").
2. VO+Right through the toolbar. Expect each button by name ("Undo, dimmed, button", "Add text", "History, collapsed", "Templates, collapsed") and the save status silent after a change.
3. Move to the page strip. Expect "Pages, navigation", then "Page 1 of 3: Cover, current page, button".
4. Move to the stage. Expect "Page being edited: Cover, region" and each item by name ("Cover title, toggle button, not pressed").
5. Interact with "Cover title" (VO+Space) and press the right arrow three times. Expect "Selected: Cover title" once, then at most one "Moved right ... percent from the left" every half second, not one per press.
6. Press `.` (rotate). Expect "Rotated to 1 degrees." Press Cmd+Z. Expect the item back.
7. Press Enter on the text item, type Vietnamese ("Chúc mừng"), press Esc. Expect "Text of the box (edit here)" while typing and the focus back on the item after Esc.
8. Open Templates (VO+Space on the button). Expect the panel "Templates for the cover" to be read and the focus inside it; open a card, "Previewing a template. Your page is unchanged."; choose Apply: "Apply this template?, dialog", the focus on "Cancel".
9. Esc closes the dialog and the focus is on the Apply button; Esc again leaves the preview.
10. Open History, preview a row, Restore, confirm. Expect "Version restored." Press Esc: the focus is on "History".

Repeat steps 2 to 4 on **iOS Safari** (VoiceOver, swipe right) on a tablet: every control should be reachable and at least a thumb wide; on a phone the editor is the phone shell: top bar, page, bottom pager, "Edit" sheet.

### NVDA on Windows (Firefox, then Chrome)

1. NVDA on, open the editor. Expect the page title and, with D (next landmark), "Editor toolbar", "Pages navigation", "Page being edited region", "Properties complementary".
2. Tab through the toolbar. Expect each button's name and state ("Add to page, collapsed"). Tab on into the stage: each item "Cover title, toggle button, not pressed", and "pressed" for the selected one.
3. With the focus on an item press Right three times. Expect one "Moved right 1." at once and a final position, not three.
4. Press Alt+Shift+Right. Expect "Size ... percent of the page width ..." and that the keyboard layout/language does **not** switch (if it does, note the key combination: it is the Windows language hotkey).
5. Open Templates, Tab to a card, Enter: "Previewing a template." Tab to "Apply to the cover", Enter: the dialog is read ("Apply this template?"); Tab stays inside it; Esc closes it.
6. Save this page as a template: the name field is read ("Template name, edit") and the character counter is **not** read on every key (finding F-13 if it is).
7. Open History, arrows move between rows, each row is read as "<date, time>, <kind>, <n items>".
8. Switch the app to Tiếng Việt in the sidebar. Expect the editor texts in Vietnamese with the same item selected and the text kept; Vietnamese is read by a Vietnamese voice if installed.
9. Make a change, switch the network off (browser dev tools). Expect "Offline: your changes are kept here..." once, not repeatedly.
10. Press Done. Expect the manage page, announced as a new page (finding F-09 if the reader stays silent).

### TalkBack on Android (Chrome)

1. Open the editor on a phone (below 768 px): it is the phone shell. Expect the top bar buttons by name ("Undo", "Redo", "History", "Templates", "Done") and "Saving..." not spoken after every change.
2. Swipe right through the bar, the page and the bottom pager. Expect "Page 1 of 3: Cover", "Previous page, dimmed", "Next page".
3. Double tap an item on the page. Expect "Selected: Cover title" and the "Edit" sheet to open with its title.
4. In the sheet, double tap the "Wider" button three times. Expect each press to be spoken once ("Wider").
5. Open "Layers": each row is "Cover title" with the selected state; double tap one to select it.
6. Open History: a full-screen dialog; swipe through the rows; double tap a row ("Previewing an earlier version."); the preview bar says "Restore this version" and "Back to current".
7. Close with the dialog's Close button; the focus returns to "History".
8. Open "Add" and add a sticker: expect "Added: ..." and the sheet to switch to "Edit".
9. Rotate the phone to landscape: the editor keeps the selection and the sheet.
10. Press Done: the manage page opens; expect the page heading to be read.
