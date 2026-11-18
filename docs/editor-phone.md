# The editor on a phone (T-181, E09, owner decision D-42)

Below **768 px** (320 to 767 px wide, portrait and landscape) "Customise / Free layout" opens the **phone editor**. A phone gets **view and light edit**: look at every page, change what is already there, add a text or a sticker, undo, apply a template, restore a version. It does **not** get drag handles: a handle is about 9 screen px at 375 px, too small to hit, and a finger that drags would fight the screen's scroll. Drag, resize and rotate (and the keyboard shortcuts) stay at 768 px and wider, unchanged.

## What is the same

One component (`web/src/book/editor/EditorBody.tsx`), one store (`useLayoutEditor`: document, undo/redo, autosave, 409 handling), the same page renderer, History, Templates, palette, property panel and the same limits (clamps, safe margin, 500 code point text cap, composition handling of T-161). No new server route and no change to the layout document. `EditorShell.tsx` is the desktop wrapper (it gives `EditorBody` the desktop stage); the route (`pages/Editor.tsx`) gives it the phone stage below 768 px and loads the desktop stage lazily above it.

Crossing 768 px (a window resize, a tablet turning) swaps the chrome and the stage only. The selection, the open sheet, the open History or Templates dialog and the typed text live in the same component and store, so none is lost (vitest `EditorPhone.test.tsx`, "crossing 768 px"; Playwright test 3).

## What a phone has

- **Top bar** (sticky): Undo, Redo, the autosave status, Done; History, Templates, Print or save as PDF. "Back to customise" is the link above the title; `?page=cover|profile|notes|back` deep links work.
- **The page** (`PhoneStage.tsx`): the real book renderer, fit to the screen width, read-only. No pointer handlers, no handles, no snapping, no guides; nothing from `gestures/` is imported, so the desktop stage chunk (`Stage-*.js`, the gesture code) is never fetched on a phone. A tap on an item selects it (outline 2 px plus a white ring, visible on any colour); a tap beside the items clears the selection. `touch-action` is `manipulation` everywhere on the page, so a vertical swipe always scrolls the screen.
- **Bottom** (fixed, on the **visual viewport**: `useVisualViewport` sets `--kb`, so the on-screen keyboard never covers it; safe-area insets respected): the **pager** (previous, "Page 3 of 12: Notes page 1" which opens a page list, next) and three buttons that open the **sheet**:
  - **Edit**: the property panel of the selection (or of the page when nothing is selected): four move arrows with a small (5 design px) and a big (40) step, size (wider, narrower, taller, shorter), rotate (plus or minus 1 degree and 15), the number fields, bring forward and send back, duplicate, delete (one confirm), hide and show for template elements, and for a text its **textarea** (font 16 px so iOS Safari does not zoom), font, size, colour, alignment, bold. Text is edited **only here** on a phone; there is no in-page field.
  - **Layers**: every item of the page, topmost first, as 44 px rows; picks an item without hitting a small target.
  - **Add**: Add text and the palette (stickers, shapes, frames, own images, background), placed at the page centre as on the desktop.
- **History and Templates** are full-height modal dialogs (focus goes in, Tab stays inside, Escape or Close closes, focus returns to the button). While a version or a template is previewed the dialog is hidden (kept mounted) and a bar under the page holds Restore or Apply and Back, so the page being previewed stays in view.
- Every button is at least 44 px high, every button has an accessible name, live-region announcements are the desktop's, all strings are in `en.json` and `vi.json` (`editor.phone.*`).

## What a phone cannot do

Drag, resize, rotate by handle, snapping and guides, keyboard shortcuts, zoom controls, the page strip, "Reset all layout" and the shortcuts help. Use a tablet or a computer for those.

## Known limit

The phone's hardware Back button follows the editor's leave guard (T-148: a "leave without saving?" question while something is unsaved, otherwise it leaves the editor); it does not close the History or Templates dialog. Close, and Escape on a keyboard, do.

## Manual checks for the owner (real devices; Playwright is Chromium only)

Run them on a real phone with the app open on the book's editor (a development server on the local network, or the deployed site once there is one). Mark each line.

iOS Safari (iPhone):
- [ ] The editor opens (no "needs a wide screen" message). The page fits the width, no sideways scroll.
- [ ] A vertical swipe over the page scrolls the screen, also when an item is selected.
- [ ] Tap the cover title: outline visible, sheet opens. Tap Layers, pick another item.
- [ ] In a text's textarea the page does **not** zoom when the field gets the focus; the keyboard opens and the sheet and its textarea stay **above** the keyboard.
- [ ] Type Vietnamese with Telex (for example "Chucs ban vui ve" gives "Chúc bạn vui vẻ") and with VNI: no letter is doubled or lost, the page text updates when the word is done, the character count is right.
- [ ] Turn the phone to landscape and back: the selection, the open sheet and the text are still there.
- [ ] Add a sticker, Undo, next page with the pager, Templates (apply one), History (restore one), Done, then reload: every change is there.
- [ ] The bottom bar is not under the home indicator (safe area).

Android Chrome with Gboard:
- [ ] The same list, with Gboard in Telex and in VNI (the Vietnamese keyboard) and with the Vietnamese handwriting input if you use it.
- [ ] The Back button (the system one) asks "leave?" only while something is unsaved and never leaves a dialog half open.
- [ ] Text at 200% (Settings, Display, Font size and Display size): the sheet scrolls, no button is cut off.
