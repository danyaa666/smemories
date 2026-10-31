# ADR 0004: Layout editor on the book renderer (units, gestures, text, undo, print parity)

Status: **Proposed** (spike T-150, 2026-10-31; the leader accepts it). Decision context: owner decisions D-42 and D-43 (free-layout editor before go-live, DOM editor on our own book renderer, no canvas library, no paid SDK),
[ADR 0003](0003-html-print-export.md) (the book is HTML printed by the browser), E09 PRD "Stage 2 plan". The prototype and every number below are in [docs/spikes/editor](../spikes/editor/README.md)
(`npm run measure` reproduces them; each number names its result file).

## Verdict in one paragraph

The DOM editor on the real book components works and prints identically. Recommendations: **store a box in page-width units with the vertical place as a fraction of the page height** (rule "centre", clamped inside the page when drawn);
**write the drag, resize, rotate and snap code ourselves** (about 90 lines with keyboard and touch; `react-moveable` costs +82 KB gzip, ignores the book's stage scale, has no keyboard and has had no release since 2023-12);
edit text in a **`<textarea>` laid exactly over the printed text** with the book's own fit function (pixel-identical to print in all 8 cases; `contenteditable` is not, and is the riskier technique);
undo with a **snapshot stack of references** (one step per gesture, 100 deep: 0.6 MB, 4 microseconds a push); parity with print is **0.04 to 0.14 % of pixels** (glyph edges only) with the item placed within 1 px at 192 dpi.
**Not proven here** (the spike was run by an agent without a person at a keyboard): real macOS Vietnamese Telex typing, and Safari and Firefox editing. Section "Risks left".

## Q1. Units and page sizes (`units.json`, `units-grid.png`)

The book draws every page in a box 816 design px wide whose height follows the paper (`designHeight`: A5 1158, A4 1154, Letter 1056 design px; Letter is 9 % shorter). Four candidates, the same cover (real Memory Book) with one rotated 150-px sticker and one text box near the bottom, drawn on all three sizes:

| Candidate | What is stored | Result on Letter (A5 = reference) |
| --- | --- | --- |
| `pct` | x, y, w, h as fractions of page width and height | sticker stretched **+9.7 %** wider than tall (text box too); position right |
| `mm` | millimetres on A5, uniform scale by width, top-left | **a sticker 50 px above the A5 bottom edge sits 11.6 mm below the Letter page: gone; a bottom text box hangs 19.6 mm below it** (27 mm for a sticker flush to the A5 bottom) |
| `centre` | x, w, h in page-width units (shape kept); the box centre as a fraction of page height | shape exact; a box that touched the A5 bottom edge hangs out **1.7 mm** (0.1 mm on A4); the clamp moves it back inside; two boxes 20 px apart on A5 are 10 px apart |
| `anchor` | width units; y from the top edge for the upper half, from the bottom edge for the lower half | nothing leaves the page, but two boxes either side of the middle that were 20 px apart **overlap by 82 px** |

**Decision: `centre`, plus a clamp when drawing.** Stored per box: `x`, `w`, `h` in thousandths of the page width (so a sticker is the same shape and the same physical share of the width on every paper), `cy` the vertical centre in thousandths of the page height, `r` rotation in degrees about the centre.
Drawn as `left = x·W`, `top = cy·H − h/2`, then moved back inside the page if it would leave it (an item can only be pushed up or down by the missing 0.04 of its height on the shortest page; an item *near* the bottom edge on Letter moves up with the page, keeps its distance in proportion, and is never cut).
It is stateless (no migration when the owner changes the paper size later; A5 to Letter and back returns the same drawing) and is design px on any page, so template overrides (which start from the template's own design-px position) use the same form.
Known price: stacked items get closer by up to 8.8 % of the distance between their centres on Letter; the editor should show the page in the book's own size, so this only matters after a paper change.
The editor and the server validate the stored numbers (bounds are generous: a box may be partly off the page, never more than half its size, so a student can bleed a sticker).

## Q2. Drag, resize, rotate, snap (`interact.json`, `size.json`, `phone.json`)

Both candidates ran on the real cover (Chromium, page at 0.685 screen px per design px, drag of +60 px expecting 87.5 design px):

| | in-house (pointer events) | `react-moveable` 0.56.0 (MIT) |
| --- | --- | --- |
| glue code (non-blank lines) | 90 (drag, resize, rotate, snap to centre, edges, safe margin and other items, guides, keyboard, touch rule) | 40 (no keyboard) |
| bundle over react-dom | **+2.7 KB gzip** (+8 KB min) | **+81.6 KB gzip** (+297 KB min) |
| drag at stage scale 1 | exact | exact |
| drag when the stage is drawn at scale 0.6 (the book scales sheets with `transform: scale`) | **exact (121.6)** | **wrong: 72.9 instead of 121.6** (it ignores the ancestor's scale) |
| touch drag on 768 px (CDP touch events) | exact; first tap selects, then drag; swipe over an unselected item still pans the page (143 px) | exact |
| keyboard move, resize, rotate | built in (arrows 1 px, Shift 10 px, Alt+arrows resize, `[` `]` rotate), focusable items | none |
| snap to page centre | exact (0 px) | within 0.4 px |
| resize of a rotated item | along screen axes (a gap: about 15 lines to project onto the item's axes) | rotation-aware |
| frame time, drag one item among **100**, 4x CPU throttle | **60 fps**, p95 frame 16.8 ms | **59.8 fps**, p95 16.8 ms |
| same among **300** | 53.8 fps, p95 49.9 ms | 59.4 fps, p95 16.7 ms |
| maintenance | n/a | last npm release 2023-12-03, last push 2024-06-03, 454 open issues, not archived |

Both beat the 30 fps target at 100 items by a wide margin. **Decision: in-house.** The deciding facts are the 30x smaller bundle, the stage-scale bug (the editor must fit pages to the window; making Moveable work would mean changing how the book scales), keyboard control being a requirement (E09) that Moveable does not give,
and a library with no release for two years. In-house needs: rotation-aware resize, larger touch handles, multi-select later; none is hard. The prototype keeps the one per-gesture cost in check by rendering items memoised and indexing snap lines once at pointer down.
The 300-item result shows the in-house limit: past about 200 items the scan of all snap lines per move would need a spatial index (the product limit is 150 items a book, so it is not reached).
**Touch (768 px and up):** works as above. **Phone, 375 px (note, not a full proof):** swipe pans, tap selects, a selected item drags without panning; but handles are 22 design px, which is **9 screen px** at fit-to-width scale, far below a 44-px target. The phone mode needs a property panel with move up/down and larger handles or none, as the plan already says.
**Accessibility path:** items are focusable buttons with an `aria-label`; arrow keys move, Alt+arrows resize, `[` and `]` rotate; the prototype calls an announce hook but renders no live region, so announcements ("moved to 12 mm, 40 mm") remain to be built.

## Q3. Text in place (`text.json`)

Chromium's real composition path was driven through the DevTools protocol (`Input.imeSetComposition`, then commit): "Việt" (V, Vi, Vie, Viê, Việ, Việt), "Nam", and "đường" (đ, đu, đư, đươ, đươn, đương, đường) typed into both techniques gives **"Việt Nam đường" in both**, with 3 compositionstart, 19 compositionupdate and 3 compositionend events each;
inserting "đẹp" with composition at the caret in the middle gives "Việtđẹp Nam đường" (correct); Enter inserts a line break in both.

| | `<textarea>` over the printed text | `contenteditable="plaintext-only"` on the printed element |
| --- | --- | --- |
| pixel difference to print, 8 cases (4 texts up to 500 runes, 0 and -4 degrees) | **0 pixels in all** (same box, font, size, line height and wrapping) | 0 to **0.6 %** (glyph edges differ) |
| paste with HTML on the clipboard (`<b style="font-size:99px">`) | plain text only, `\r\n` becomes `\n` | same (needs the same explicit paste handler) |
| 600 pasted emoji | **500 runes kept** (983 UTF-16 units: `maxlength` counts units, so the cap is our own rune count) | 500 runes |
| composition at 499 runes (3 more composed) | finishes the word (502), cut to 500 at `compositionend` | state capped, DOM not; caret jumps when cut |
| caret, selection, undo of typing | native control | native, but ours to repair when the DOM text is rewritten; trailing newline needs special cases |
| accessibility name and role | a real text field | needs `role="textbox"`, `aria-multiline` |

**Decision: textarea overlay.** The printed text stays a `div` (React text, `white-space: pre-wrap`, never HTML); in edit mode a transparent textarea with the same box, font, size (the size the fit rule chose), line height and padding lies over it. 500 runes are counted as code points (Go `utf8.RuneCountInString` on the server; the web side `Array.from(s).length`), and the cap applies at `compositionend`, never in the middle of a composition.
**Text that is too long:** fixed box, the template rule that already exists: shrink with `fitElement` (0.5 px steps) down to 50 % of the box's own size, then clip, with the editor warning "text is cut" (the book's fit report). 500 runes of Vietnamese with an emoji fit a 440 x 200 design px box at 18 px (from 30 px), shrunk, not cut.
**Editor shows what prints:** same CSS and the same function; the textarea view equals the printed view pixel for pixel, rotated or not, in all 8 cases. Fonts: the box uses the book's font set; every font a text box can pick must be a font the print already embeds (the parity run embedded 9 font files, all `emb yes`).
**Not done:** real macOS Telex by hand (no one at a keyboard); Safari and Firefox (Playwright's Chromium only). iOS Safari zooms the page when a field below 16 css px gets focus; a text box smaller than that on screen will need `font-size: 16px` on the editing field plus a counter-scale, or a phone mode that edits text in the property panel. These are the first manual checks for QA of the text task.
Fallback if the textarea fails by hand on a browser: edit the text in the property panel (a normal field) and keep the box a display element; the layout model does not change.

## Q4. Undo and redo (`undo.json`)

A synthetic document of 129 162 bytes (504 items, more than the product limit of 150), 2000 gestures, depth 100:

| | memory | per gesture | notes |
| --- | --- | --- | --- |
| snapshot stack of **references** (React state is immutable: the old object is the snapshot) | **0.58 MB** | **4 µs** | undo of 100 steps: under 1 ms |
| snapshot stack of JSON strings | 24.4 MB | 182 µs | what you need if state were mutated in place |
| command stack (do, undo) | 0.06 MB | 3 µs | needs an inverse for about nine operations (move/resize/rotate, add, delete, z-order, text edit, hide, background, bulk paste, restore); every future operation adds one more place for a bug |

**Decision: snapshot stack of references, depth 100.** One step per gesture (pushed at pointer down, at the first key of a burst, at blur for text), so a whole drag is one undo. Autosave is independent of the stack: it debounces the *current* document; an undo simply makes a new current document and a new save (with `base_version`).
On `409` the stack is cleared (the other tab's document replaces ours). Version history (server) is a separate thing from undo and is not affected.

## Q5. Print parity (`parity.json`, `parity-A5-editor.png`, `parity-A5-pdf.png`)

The editor view (screen media, page at 96 dpi, 2x device pixels) against `page.pdf()` of the same page rasterised by `pdftoppm -r 192`: the real cover with a rotated sticker and a rotated text box.

| Paper | PDF page (pt) | pixels differing by more than 48/255 | same, allowing a 1-px shift | sticker placement offset (192 dpi) |
| --- | --- | --- | --- | --- |
| A5 | 420 x 594.96 | 0.98 % | **0.14 %** | 1 px (0.13 mm) |
| A4 | 594.96 x 841.92 | 0.52 % | **0.06 %** | 1 px |
| Letter | 612 x 792 | 0.47 % | **0.04 %** | 0 px |

**The largest difference is 0.14 % (A5).** The residue is anti-aliasing of glyph and shape edges (screen compositor against the PDF rasteriser; the image of the differences shows only outlines), not displacement. Fonts: 9 files embedded, all `emb yes`; rotation survives as vector (the extracted text of the rotated box is intact: "Việt Nam — đường về nhà" on all three sizes); one page per sheet.
The safe margin is an editor-only overlay and is never drawn on the page; the printed items sit within 1 px of the editor's. Proposed assertion for the product parity test (rasterise with poppler as print-check already does): a 1-px tolerant difference below 0.5 % and each placed item within 2 px at 192 dpi. A real book page has photos and several items; the number should be re-measured there when the runtime exists.

## Risks left

1. **Vietnamese IME by hand and other browsers.** Chromium's composition events pass; real macOS Telex, Safari (including iOS) and Firefox are unverified (focus zoom, `textarea` inside a transformed ancestor, caret). Highest risk of the stage; the text task must include a manual check on macOS and iOS before it is READY_FOR_QA.
2. **Touch handles and phone mode.** 9 screen px handles at phone width; needs the planned panel with buttons.
3. **Stacked items on Letter** get closer by up to 8.8 % of their centre distance after a paper change; the editor may warn on overlaps created by a paper change (optional).
4. **Resize of rotated items** is written for screen axes in the prototype; a rotation-aware version is a known small piece of work.
5. **Measured on the cover with synthetic items** (no photo). A page with photos, many items and the notes pages needs the same parity and frame-time check once the runtime exists (the plan already lists budgets).
6. **Snap scan** is O(items) per move: fine at the 150-item limit, not at 300 (53.8 fps with p95 50 ms).
7. **Rune count is code points.** A flag emoji or a family emoji is several; the cap counts them as several (documented, same on the server).

## Changes this implies for the stage 2 sketch (E09 PRD)

1. **Layout model task** (after this spike): box = `x, w, h` in thousandths of page width, `cy` in thousandths of page height, `r` degrees; the manifest's safe margin and bounds in the same units; the draw rule with clamp lives in one shared function used by the layout runtime and the editor. Server bounds: a box may leave the page by at most half of its size.
2. **No gesture library and no new dependency in `web/`** (drops the "editor SDK/library" risk and the licence check from the plan). The move/resize/rotate task owns a small gesture module (pointer events, snapping, guides) with **keyboard control in the same task** (it moves out of "2c accessibility"), rotation-aware resize and touch handles sized for 44 px.
3. **`Sheet` (web/src/book/env.tsx) gets a layer slot** inside `.bk-inner` (the prototype portals into it); the layout runtime draws items there, the editor draws handles there. The editor's zoom may use the book's existing stage scale (the in-house code reads the scale from the layer's bounding box).
4. **Text task:** textarea overlay, `fitElement` shrink to 50 % then clip, shared rune cap (code points, applied at `compositionend`), plain-text paste handler, manual macOS Telex and iOS Safari checks as acceptance criteria. Fallback is editing in the property panel.
5. **Undo task** is part of the editor shell, not a task of its own: snapshot of references, depth 100, one step per gesture; autosave and the 409 handling as described.
6. **Parity test** (print-check coverage): editor view against `page.pdf()` rasterised at 192 dpi, 1-px tolerant difference below 0.5 %, items within 2 px, on A5, A4 and Letter; reuse the method of `docs/spikes/editor/scripts/parity.mjs`.
7. **Phone mode** stays "view and light edit": selection, a property panel with move up/down and size buttons, text edited in the panel; no drag handles on phones.
8. T-151 (assets) and T-152 (uploads) are unaffected.
