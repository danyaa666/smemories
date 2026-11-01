# Text boxes in the editor

The free-layout editor (epic E09, T-161, ADR [0004](adr/0004-layout-editor.md) Q3) lets the student add text boxes to a page, type in them on the page and style them in the property panel. Code: `web/src/book/editor/` (`TextField.tsx`, `PageTextField.tsx`, `TextControls.tsx`, `textInput.ts`), the field is drawn by `web/src/book/layout/runtime.tsx` (`TextItem`).

## How it works

- **The printed text stays what prints.** A text box is a `div` that renders the text as React text with `white-space: pre-wrap` and is fitted by `FitText` (`fitElement`: 0.5 px steps down to 50 % of the box's own size, then clipped). In edit mode a transparent `<textarea>` lies over it inside the same item, so the rotation, the page scale and the font set come from the same ancestors. It copies the printed text's style and takes the size the fit rule chose; the printed text is hidden (`visibility`) but keeps being fitted. The edit view equals the printed view: the e2e parity test compares the two, see below.
- **500 characters are 500 code points**, not UTF-16 units and not graphemes: `Array.from(s).length` after `\r\n` to `\n`, NFC and trim, the same rule as the server (`utf8.RuneCountInString` of the cleaned text). A flag emoji is two, a family emoji joined with U+200D is five, decomposed Vietnamese counts as its composed form. `maxlength` is not used. The function is `runeCount` in `editor/document.ts`; `capText` in `editor/textInput.ts` cuts at a code point.
- **Composition rule.** From `compositionstart` to `compositionend` the field never rewrites its value and sends nothing to the document, and the cap is not applied (a word started at 499 runes may finish: 502 for a moment). At `compositionend` the value is cut to the cap. Outside a composition a value over the cap, or with a character the server refuses, is fixed at once and the caret is put back where the user was typing.
- **Paste rule.** The paste handler takes `text/plain` only (an HTML-only clipboard inserts nothing), turns `\r\n` and `\r` into `\n`, a tab into a space, drops the characters the server refuses (control and format characters except the joiner, line and paragraph separators, U+FFFD, lone surrogates) and inserts at the selection, then the cap applies. A drop that carries HTML or files is turned into its plain text.
- **History.** Typing is one undo step per burst (a pause of 1 s, blur, Escape, closing edit mode). Inside the field Ctrl/Cmd+Z is the browser's own undo and never reaches the editor's. Font, size (the slider is one step per drag), colour, alignment and bold are one step each. Closing edit mode with only white space removes the box (one more step; undo brings it back).
- **Open and close.** Double-click, Enter, or a click or tap on a text that is already selected opens the field; "Add text" (toolbar and empty-selection panel) adds a box in the manifest's default style (`body` font, the middle size 52 of 8..96, the first colour, left, not bold, centred, 60 % of the page wide), opens it and selects its text. Escape closes and keeps the text and gives the focus back to the box; so does a click outside. Refused at 40 items on a page or 150 in the book, with a message.
- **Clip warning.** If the text does not fit even at 50 % the editor says "Text is cut" beside the stage and in the panel (the fit report of `fitReport.ts`/`FitText`); it goes away when the text is shorter or the box larger (`FitText` fits again when its box changes size). The field keeps `overflow: hidden` (a scrolling textarea differs from print) and a visible caret; the panel field shows the whole text.
- **Touch.** On a coarse pointer the real font size of the field is never below 16 css px (iOS Safari zooms the page otherwise); a smaller text is drawn with the field made larger and scaled down by a transform.
- **Fallback (ADR Q3).** The panel always has a normal multi-line field with the same cap and paste rules (`TextControls`). It is also the only way to edit a text that is behind another item, and what the phone mode will use.
- The text size is in design px of the 816 px page (what the book draws), not in points; the label says "Size".

## Tests

- Unit and component (vitest): `textInput.test.ts` (rune counter, cap, paste normaliser), `TextField.test.tsx` (composition events: value not rewritten, cut at `compositionend`, caret after a cut, paste, drop, history), `EditorText.test.tsx` (the shell: add, 41st, edit in place, panel controls, clip warning), `document.test.ts` (`removeBlankText`).
- Browser (`tools/e2e/editor-text.spec.mjs`, in `make e2e`, see [e2e.md](e2e.md)): Vietnamese through CDP `Input.imeSetComposition`, 499 runes, paste, undo, the 41st box, the clip warning, style controls, the 16 px rule, and the parity of the edit view with the printed view on A5, A4 and Letter.

## Manual checks (AC3): what is proven and what is not

Automated tests drive the browser's composition path with CDP in Chromium. They do not replace a real keyboard. Result column: pass, fail or not run, with browser and OS; "not run" is **unproven**, never "pass".

| | Check | Result |
| --- | --- | --- |
| a | macOS Safari or Chrome with the Telex input source: type "Tiếng Việt có dấu" in a new text box, undo, redo | not run (no keyboard); unproven |
| b | iOS Safari (iPhone or simulator): focus does not zoom the page, Telex/VNI builds the letters, the box stays visible | not run; unproven |
| c | Firefox desktop: typing and paste | not run; unproven |
| d | Android Gboard Telex | not run (no device); unproven |

A failure of (a) or (b) blocks the merge: the editor then keeps only the panel field (requirement 9 of the task).
