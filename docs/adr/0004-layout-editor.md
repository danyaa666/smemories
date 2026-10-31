# ADR 0004: Layout editor on the book renderer (units, gestures, text, undo, print parity)

Status: **Proposed** (spike T-150, 2026-10-31; the leader accepts it). Decision context: owner decisions D-42 and D-43 (free-layout editor before go-live, DOM editor on our own book renderer, no canvas library, no paid SDK),
[ADR 0003](0003-html-print-export.md) (the book is HTML printed by the browser), E09 PRD "Stage 2 plan". The prototype and every number below are in [docs/spikes/editor](../spikes/editor/README.md)
(`npm run measure` reproduces them; each number names its result file).

## Verdict in one paragraph

The DOM editor on the real book components works and prints identically. Recommendations: **store a box in page-width units with the vertical place as a fraction of the page height** (rule "centre", clamped inside the page when drawn);
**write the drag, resize, rotate and snap code ourselves** (about 110 lines with keyboard, touch and the stored-form write-back; `react-moveable` costs +82 KB gzip, ignores the book's stage scale, has no keyboard and has had no release since 2023-12);
edit text in a **`<textarea>` laid exactly over the printed text** with the book's own fit function (pixel-identical to print in all 10 Chromium cases, including the clipped state; `contenteditable` is not, and is the riskier technique);
undo with a **snapshot stack of references** (one step per gesture, 100 deep: 0.6 MB, 4 microseconds a push); parity with print is **0.04 to 0.14 % of pixels** (glyph edges only) with the item placed within 1 px at 192 dpi.
**Not proven here** (the spike was run by an agent without a person at a keyboard): real macOS Vietnamese Telex typing, real Safari and iOS, Firefox IME, Android Gboard Telex, touch drag in Firefox and WebKit, screen readers, and the keyboard shortcuts on Windows and Linux. Section "Risks left" (the owner checks Telex by hand).

## Q1. Units and page sizes (`units.json`, `units-grid.png`)

The book draws every page in a box 816 design px wide whose height follows the paper (`designHeight`: A5 1158, A4 1154, Letter 1056 design px; Letter is 9 % shorter). Four candidates, the same cover (real Memory Book) with one rotated 150-px sticker and one text box near the bottom, drawn on all three sizes (`units-grid.png`: rows are candidates, columns are papers, each box drawn exactly as stored, **without** the clamp, so the picture shows what the table measures):

| Candidate | What is stored | Result on Letter (A5 = reference) |
| --- | --- | --- |
| `pct` | x, y, w, h as fractions of page width and height | sticker stretched **+9.7 %** wider than tall (text box too); position right |
| `mm` | millimetres on A5, uniform scale by width, top-left | **a sticker whose box is 58 px above the A5 bottom edge (y 950 + 150 = 1100 of 1158) has its bottom 11.6 mm (44 px) past the Letter page: cut, 106 of its 150 px visible; a bottom text box hangs 19.6 mm past it and is cut too** (27 mm for a sticker flush to the A5 bottom, which is gone) |
| `centre` | x, w, h in page-width units (shape kept); the box centre as a fraction of page height | shape exact; a box that touched the A5 bottom edge hangs out **1.7 mm** (0.1 mm on A4); the clamp moves it back inside; two boxes 20 px apart on A5 are 10 px apart |
| `anchor` | width units; y from the top edge for the upper half, from the bottom edge for the lower half | nothing leaves the page, but two boxes either side of the middle that were 20 px apart **overlap by 82 px** |

**Decision: `centre`, plus a clamp when drawing.** Stored per box: `x`, `w`, `h` in thousandths of the page width (so a sticker is the same shape and the same physical share of the width on every paper), `cy` the vertical centre in thousandths of the page height, `r` rotation in degrees about the centre.
Drawn as `left = x·W`, `top = cy·H − h/2`, then moved back inside the page if it would leave it (an item can only be pushed up or down by the missing 0.04 of its height on the shortest page; an item *near* the bottom edge on Letter moves up with the page, keeps its distance in proportion, and is never cut).
It is stateless (no migration when the owner changes the paper size later; A5 to Letter and back returns the same drawing) and is design px on any page, so template overrides (which start from the template's own design-px position) use the same form.
Known price: stacked items get closer by up to 8.8 % of the distance between their centres on Letter; the editor should show the page in the book's own size, so this only matters after a paper change.
The draw rule keeps a box fully inside the page, so with it a sticker cannot bleed off the edge; bleed would need the clamp relaxed to "at most half the box outside", a product decision for the layout-model task (the editor's gestures use the same clamp, see Q2, and would follow it). The editor and the server validate the stored numbers.

## Q2. Drag, resize, rotate, snap (`interact.json`, `size.json`, `phone.json`)

Both candidates ran on the real cover (Chromium, A5 page at 0.685 screen px per design px, snap off, drag of +60 px expecting 87.5 design px; paper sizes in the next paragraph):

| | in-house (pointer events) | `react-moveable` 0.56.0 (MIT) |
| --- | --- | --- |
| glue code (non-blank lines) | 112 (drag, resize, rotate, snap to centre, edges, safe margin and other items, guides, keyboard, touch rule, pointerId filter, stored-form write-back; includes imports and comments) | 40 (no keyboard) |
| bundle over react-dom | **+3.1 KB gzip** (+9.9 KB min) | **+81.7 KB gzip** (+298 KB min) |
| drag at stage scale 1 (A5 page, snap off, drag up so it stays on the page) | exact | exact |
| drag when the stage is drawn at scale 0.6 (the book scales sheets with `transform: scale`) | **exact (121.6)** | **wrong: 72.9 instead of 121.6** (it ignores the ancestor's scale) |
| touch drag on 768 px (CDP touch events) | exact; first tap selects, then drag; swipe over an unselected item still pans the page (143 px) | exact |
| keyboard move, resize, rotate | built in (arrows 1 px, Shift 10 px, Alt+arrows resize, `[` `]` rotate), focusable items | none |
| snap to page centre | exact (0 px) | exact (0 px; 0.4 px in the first run) |
| resize of a rotated item | along screen axes (a gap: about 15 lines to project onto the item's axes) | rotation-aware |
| frame time, drag one item among **100**, 4x CPU throttle | **59.8 fps**, p95 frame 16.7 ms | **59.8 fps**, p95 16.8 ms |
| same among **300** | 52.8 fps, p95 49.9 ms | 59.5 fps, p95 16.7 ms |
| maintenance | n/a | last npm release 2023-12-03, last push 2024-06-03, 454 open issues, not archived |

Both beat the 30 fps target at 100 items by a wide margin. **Decision: in-house.** The deciding facts are the 26x smaller bundle, the stage-scale bug (the editor must fit pages to the window; making Moveable work would mean changing how the book scales), keyboard control being a requirement (E09) that Moveable does not give,
and a library with no release for two years. In-house needs: rotation-aware resize, larger touch handles, multi-select later; none is hard. The prototype keeps the one per-gesture cost in check by rendering items memoised and indexing snap lines once at pointer down.
The 300-item result shows the in-house limit: past about 200 items the scan of all snap lines per move would need a spatial index (the product limit is 150 items a book, so it is not reached).
**Touch (768 px and up):** works as above. **Phone, 375 px (note, not a full proof):** swipe pans, tap selects, a selected item drags without panning; but handles are 22 design px, which is **9 screen px** at fit-to-width scale, far below a 44-px target. The phone mode needs a property panel with move up/down and larger handles or none, as the plan already says.
**Gestures edit the stored form on every paper (`gestures.json`).** The first prototype kept every item as a box on the A5 page and wrote pointer deltas of the *displayed* page into it, so on Letter a 100 px drag moved the item 22 px (QA measured ratio 0.22 for `centre` and `pct`, 1.12 for `anchor`); only A5, where all four units coincide, was exact.
Fixed: a gesture starts from the box as drawn on the edited page (`place`, clamped), adds the pointer delta in that page's design px, snaps against the other items' drawn boxes and the page lines, clamps to the page, and writes back `unplace` (the inverse of `place`; `unstore` gives the A5 box that the rest of the prototype reads).
The item also keeps the stored form (`st`) because the A5 box cannot hold everything: for `anchor` the edge (`ay`) is decided on the edited page. Keyboard steps go through the same path. Proof, snap off, Chromium, real cover, 12 runs (4 units x A5, A4, Letter), pointer against item in screen px:

| | A5 | A4 | Letter |
| --- | --- | --- | --- |
| drag text box 50 right, 100 up: pointer error | 0.0 | 0.0 | 0.0 |
| corner-resize sticker by 40 px: width growth | 40.0 | 40.0 | 40.0 (not reachable in `mm`, see risk 8) |
| drag sticker 400 px down, past the bottom (ends flush at the page edge), then 100 px up: moved | 100.0 | 100.0 | 100.0 |
| keyboard: ArrowRight, Shift+ArrowDown (design px) | 1, 10 | 1, 10 | 1, 10 |

The worst pointer error over all runs is 0.0 screen px (the item's `left/top` are read from computed style, 3 decimals). The clamp zone has no dead zone: because the clamp is applied when the gesture writes, the stored box is always the drawn box. A second pointer (pointerId 7 injected during a mouse drag on Letter/`centre`) does not move the item, start a gesture or deselect it; the first pointer keeps dragging (20.0 px). With snap on (6 design px), a drag can end up to 6 design px from the pointer by design.
`react-moveable`'s layer in the prototype still writes into the A5 box and is not fixed: it was only the comparison and is not recommended, so its A5-only numbers stand for what they are. The earlier `interact.json` runs are A5 only (the page where units coincide); `gestures.json` is the paper-size proof.
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
**Editor shows what prints:** same CSS and the same function; the textarea view equals the printed view pixel for pixel, rotated (0 and -4 degrees) or not, in all **10** cases of `text.json` in Chromium: short, medium, long (shrunk to 20.5 px), 500 runes (shrunk to 18 px) and, new, **499 runes in a 220 x 70 box = the clip state** (15 px, text cut).
**The clip state and the caret.** The field's caret is visible (the first prototype hid it with a transparent `caret-color`; the parity runs hide it with `caret=off` only so that a blinking caret is not in the screenshot). In the clip state the textarea stays `overflow: hidden` (a scrollable textarea differs from print by 1.35 % of pixels when rotated, a different glyph rasteriser; hidden is 0): typing at the end of 300 runes in the 220 x 70 box scrolled the field by 99 px to keep the caret in view, the typed tail is there (`clip_state_typing_at_end`, picture `text-clip-caret.png`). A hidden field does not scroll with the mouse wheel; the editor shows the "text is cut" warning and the property panel shows the whole text.
**Cap while typing in the middle:** a controlled textarea rewrites the value when the 500-rune cap cuts it, and the caret jumps to the end. The prototype restores it (500 runes, caret after the 100th character, one more typed: selection 101 as expected, `cap_rewrite_caret_after_typing_in_middle`).
**Fonts loading, and other engines.** The fit function ran once per value; it now re-runs when `document.fonts` is ready and on `loadingdone`, as `web/src/book/fit.tsx` `FitText` already does. Playwright Firefox 157, three loads of the long and the 500-rune text: stable size every load (19.99 px and 18.01 px) and textarea against print 0 % (QA saw 25 to 35 % before, from the missing refit). Firefox's sizes differ from Chromium's (19.99 against 20.5 px, 18.01 against 18.0) because font metrics differ between engines and `fitElement` steps by 0.5 px, so **line breaks in the editor on Firefox or Safari can differ from the Chromium print**; this is not proven away. Requirements for the text task: reuse `FitText` itself (do not copy the call), the textarea takes its size from it, and the print-parity check also runs the editor view in Firefox and WebKit where the editor is supported. Fonts: the box uses the book's font set; every font a text box can pick must be a font the print already embeds (the parity run embedded 9 font files, all `emb yes`).
**Not done:** real macOS Telex by hand (no one at a keyboard); real Safari and Firefox IME (composition events were driven in Playwright's Chromium only; layout parity was also looked at in Firefox and WebKit under Playwright). iOS Safari zooms the page when a field below 16 css px gets focus; a text box smaller than that on screen will need `font-size: 16px` on the editing field plus a counter-scale, or a phone mode that edits text in the property panel. These are the first manual checks for QA of the text task.
Fallback if the textarea fails by hand on a browser: edit the text in the property panel (a normal field) and keep the box a display element; the layout model does not change.

## Q4. Undo and redo (`undo.json`)

A synthetic document of 129 162 bytes (504 items, more than the product limit of 150), 2000 gestures, depth 100:

| | memory | per gesture | notes |
| --- | --- | --- | --- |
| snapshot stack of **references** (React state is immutable: the old object is the snapshot) | **0.58 MB** | **4 µs** | undo of 100 steps: under 1 ms |
| snapshot stack of JSON strings | 24.4 MB | 188 µs | what you need if state were mutated in place |
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

1. **Vietnamese IME by hand and other browsers (still unproven).** Chromium's composition events pass; real macOS Telex, real Safari and iOS, Firefox IME and Android Gboard Telex are unverified (focus zoom, `textarea` inside a transformed ancestor, caret). Highest risk of the stage; the text task must include a manual check on macOS and iOS before it is READY_FOR_QA, and the owner checks Telex by hand.
2. **Touch handles and phone mode.** 9 screen px handles at phone width; needs the planned panel with buttons.
3. **Stacked items on Letter** get closer by up to 8.8 % of their centre distance after a paper change; the editor may warn on overlaps created by a paper change (optional).
4. **Resize of rotated items** is written for screen axes in the prototype; a rotation-aware version is a known small piece of work.
5. **Measured on the cover with synthetic items** (no photo). A page with photos, many items and the notes pages needs the same parity and frame-time check once the runtime exists (the plan already lists budgets).
6. **Snap scan** is O(items) per move: fine at the 150-item limit, not at 300 (52.8 fps with p95 50 ms).
7. **Rune count is code points.** A flag emoji or a family emoji is several; the cap counts them as several (documented, same on the server).
8. **Handles are clipped by the page.** The page sheet (`.bk-inner`) is `overflow: hidden`; the resize handle sits on the item's corner, so for an item flush to the page edge (`mm` on Letter does this) it is outside the clip and cannot be reached with the mouse (`gestures.json`: `handle_clipped_by_page`). Selection chrome (handles, outline, guides) must be drawn in a layer outside the clip (an overlay positioned over the page), not in the item layer; the keyboard path still works.
9. **Still unproven, besides risk 1:** touch drag in Firefox and WebKit (mouse drag, rotation and the keyboard were exact in Chromium, Firefox and WebKit under Playwright per QA); screen readers (no live region is built); keyboard shortcuts on Windows and Linux, where `Alt+Arrow` is browser Back (the prototype calls `preventDefault`, not tried there) and `[` `]` need AltGr on some layouts: the gesture task should pick and test shortcuts that work on those (for example Ctrl+Arrow and `,` `.` or a visible rotate control) and keep them in a help sheet.

## Changes this implies for the stage 2 sketch (E09 PRD)

1. **Layout model task** (after this spike): box = `x, w, h` in thousandths of page width, `cy` in thousandths of page height, `r` degrees; the manifest's safe margin and bounds in the same units; the draw rule with clamp lives in one shared function used by the layout runtime and the editor. Server bounds: a box may leave the page by at most half of its size.
2. **No gesture library and no new dependency in `web/`** (drops the "editor SDK/library" risk and the licence check from the plan). The move/resize/rotate task owns a small gesture module (pointer events with a pointerId filter, snapping, guides) that edits the box as drawn on the edited page and writes the stored form back through the inverse of the draw rule (Q2), with **keyboard control in the same task** (it moves out of "2c accessibility"), rotation-aware resize and touch handles sized for 44 px.
3. **`Sheet` (web/src/book/env.tsx) gets a layer slot** inside `.bk-inner` (the prototype portals into it); the layout runtime draws items there. The editor's handles, outline and guides go in an overlay **outside** the page clip (risk 8), positioned over the page. The editor's zoom may use the book's existing stage scale (the in-house code reads the scale from the layer's bounding box).
4. **Text task:** textarea overlay, `fitElement` shrink to 50 % then clip, shared rune cap (code points, applied at `compositionend`), plain-text paste handler, manual macOS Telex and iOS Safari checks as acceptance criteria. Fallback is editing in the property panel.
5. **Undo task** is part of the editor shell, not a task of its own: snapshot of references, depth 100, one step per gesture; autosave and the 409 handling as described.
6. **Parity test** (print-check coverage): editor view against `page.pdf()` rasterised at 192 dpi, 1-px tolerant difference below 0.5 %, items within 2 px, on A5, A4 and Letter; reuse the method of `docs/spikes/editor/scripts/parity.mjs`.
7. **Phone mode** stays "view and light edit": selection, a property panel with move up/down and size buttons, text edited in the panel; no drag handles on phones.
8. T-151 (assets) and T-152 (uploads) are unaffected.
