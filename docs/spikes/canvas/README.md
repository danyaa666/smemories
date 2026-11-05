# Canvas spike (T-169, ADR 0005)

A throw-away spike that answers the questions of [ADR 0005](../../adr/0005-doory-templates-and-fabric-studio.md): can templates from
[Doory](https://github.com/shaongitbd/doory) be brought into our layout documents reliably, and can a Fabric.js image studio sit beside the DOM
editor without being able to hurt a book or its print. **Nothing here ships**: no change in `web/`, `internal/` (read only), `cmd/`, CI or the product's
dependencies; this folder has its own `package.json`. Doory's source is **not vendored**: step 0 clones it at a pinned commit into a git-ignored folder.

## Run it

```sh
cd docs/spikes/canvas
npm install                    # fabric 7.4.0 (exact), react, vite, playwright 1.64.0, pngjs; the only network use besides the clone
npx playwright install chromium   # only if Playwright's Chromium 1248 is not installed yet
brew install poppler           # pdftoppm, pdfinfo, pdffonts, pdftotext (the parity step), as for `make print-check`
npm run clone                  # clones https://github.com/shaongitbd/doory at a65c43a98133344273e6e13cde9d2c98d0270421 into ./doory (git-ignored)
npm run measure                # about 2 minutes; every step below can also be run alone
```

Needs Node 22, Go (the module's version) and a checkout of this repository (the steps read `web/src/book/` fonts and asset SVGs and import
`internal/booklayout`). No Docker and no database. Servers use a free port chosen by the OS and are closed by the script that started them.
Numbers move with the machine only where they are timings; counts, sizes, hashes and pixel results are exact.

| Step (`npm run ...`) | Question | Writes | What it shows |
| --- | --- | --- | --- |
| `coverage` | Q1 | `results/coverage.json`, `results/objects.json`, `out/layouts/*.json` | every object of the 10 starter templates classified exact / approximate / cannot convert, in 3 variants (fit, cover, fit with the proposed stretch shapes) |
| `validate` | Q2 | `results/validate.json` | every converted page, for each of the four page kinds, through the product's `booklayout.Validate` against the `memory-book` manifest; 4 negative controls must be refused first |
| `determinism` | Q2 | `results/determinism.json` | the converter run in two processes (other TZ, LANG, cwd): same sha256 over 208 conversions |
| `render` | Q1 | `results/selection.json`, `out/renders/<slug>.png` | before (Doory's own thumbnail) and after (A5 today's shapes, A5, A4, Letter with the proposed shapes) of the 3 best and 3 worst templates. `out/` is git-ignored (see the licence section of the ADR) |
| `parity` | Q2 | `results/parity.json` | the 3 best templates on A5, A4, Letter: screen against the browser PDF rasterised at 192 dpi |
| `studio:bundle` | Q4, Q7 | `results/bundle.json` | gzip cost of the lazy studio chunk and proof it is in neither the main nor the print entry |
| `studio:failures` | Q4 | `results/failures.json` | 15 cases: chunk load failure, crash, export failure, 500, network drop, 429, 30-image cap, quota, double click, leaving with unsaved work, feature flag, Fabric round trip, sizes and timings |

## Map of the code

- `src/convert.mjs`: the converter, a pure function `convertPage(design, pageIndex, {slug, kind, ratio, mode, stretch}) -> {page, report}`. The policies (page size, colours, fonts, text, shapes, lines, photos, item cap) are the comments and constants at the top; the ADR explains them.
- `src/render.mjs`: a static renderer that follows the product's draw rule (`web/src/book/layout/geometry.ts` `place`, `runtime.tsx` text and asset styles). **It is a copy of the rule, not the product's runtime**: its own fit loop stands for `FitText`, a grey box for the template's photo.
- `src/serve.mjs`: a static server on a free port, rooted at the checkout (fonts and SVGs are read from `web/`).
- `validate/main.go`: build tag `spike`, so `go build ./...`, `go vet ./...` and CI never see it: `go run -tags spike ./docs/spikes/canvas/validate`.
- `studio/`: the studio prototype. `src/shell.jsx` (book stand-in, feature flag, `lazy()` + error boundary, the upload call), `src/studio.jsx` (Fabric canvas, flatten and save), `src/flatten.mjs` (sizes and caps), two page entries (`index.html`, `print.html`) like the product's two routes.
- `scripts/*.mjs`: the steps. `scripts/clone-doory.sh`: the pinned clone.

## What is committed and what is not

Committed: the code, `results/*.json` (counts, object ids, types and classes; no Doory text and no pictures). **Not committed**: `doory/` (the clone), `out/layouts` (the converted templates) and `out/renders` (the pictures), because they are derived from Doory's starter templates, which are under the Attribution Assurance License (ADR 0005, licence section). Regenerate them with the steps above.

## Not covered (see the ADR, "Not proven")

Real Vietnamese Telex/VNI typing (the studio has no text tool, so none was needed), iOS Safari and a real phone's memory (timings are Chromium on this machine with the CPU throttled 4x), the product's own `FitText` and `LayoutLayer` on converted templates (the parity step uses the copy of the rule), Firefox and WebKit.
