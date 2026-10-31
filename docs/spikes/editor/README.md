# Layout editor spike (T-150, ADR 0004)

A throw-away prototype that answers the five questions of [ADR 0004](../../adr/0004-layout-editor.md). It renders the **real Memory Book cover**
(`web/src/book`, imported through the alias `@web`) and puts a layout layer inside the sheet, which is how the real runtime would work.
Nothing in `web/`, `internal/` or CI changes; this folder has its own `package.json`. Do not ship any of it: the product tasks write their own code
using what the ADR decides.

## Run it

```sh
(cd web && npm ci)                      # the real book components need web's dependencies (once)
cd docs/spikes/editor && npm ci         # vite, react (one copy for everything), react-moveable, playwright, pngjs
npx playwright install chromium         # only if Playwright's Chromium is not installed yet (1.64.0)
brew install poppler                    # pdftoppm, pdfinfo, pdffonts, pdftotext (the parity step), as for `make print-check`
npm run measure                         # about 3 minutes: starts the dev server itself, writes results/*.json
npm run dev                             # to look at it: http://localhost:5391/?view=editor&lib=inhouse&n=2
```

`npm run measure` runs the steps below in order; each writes one file in `results/` and can be run alone with the dev server up
(`SPIKE_URL=http://localhost:5391 node scripts/<step>.mjs`). Numbers move with the machine: the ADR gives what this machine measured (Apple silicon, Chromium 1248
from Playwright 1.64); frame times and sizes are reproducible within noise, the pixel and unit results exactly.

| Step | Question | Result file | What it shows |
| --- | --- | --- | --- |
| `units.mjs`, `grid.mjs` | 1 units and page sizes | `units.json`, `units-grid.png` | four ways to store a box, drawn on A5, A4 and Letter |
| `size.mjs` | 2 library | `size.json` | minified and gzip bundle of each candidate over react-dom |
| `interact.mjs` | 2 library | `interact.json` | drag, resize, rotate, snap, touch at 768 px, keyboard, frame rate with 100 and 300 items at 4x CPU throttle |
| `phone.mjs` | 2 touch note | `phone.json` | 375 px phone: swipe pans the page, tap selects, a selected item drags |
| `text.mjs` | 3 text | `text.json` | Vietnamese composition through Chromium's IME events, paste, 500 rune limit, editor against print |
| `undo.mjs` | 4 undo | `undo.json` | snapshot of references, snapshot of JSON, command stack on a 126 KiB document |
| `parity.mjs` | 5 print | `parity.json`, `parity-A5-*.png` | the editor view against the browser's PDF on A5, A4 and Letter |

## Map of the code

- `src/units.mjs`: the four unit candidates (`pct`, `mm`, `centre`, `anchor`), pure functions.
- `src/book.jsx`: the real `BookPages` cover and a portal into its `.bk-inner` (design px, origin at the page corner).
- `src/items.jsx`: items (a sticker is an inline SVG, a text box uses the one shared text CSS).
- `src/inhouse.jsx` (pointer events, snapping, handles, keyboard) and `src/moveable.jsx` (react-moveable): the two editors compared.
- `src/text.jsx`: the text box in its two techniques (`tech=ta` textarea, `tech=ce` contenteditable) and in print (`mode=view`).
- `scripts/*.mjs`: the measurements. `src/size/*.jsx` are the three bundles of `size.mjs`.

## Not covered (see the ADR, "Risks left")

Only Chromium was driven (the print engine of record, ADR 0003). Real macOS Telex typing, Safari and Firefox editing, and screen readers were not tried by the author of the spike.
