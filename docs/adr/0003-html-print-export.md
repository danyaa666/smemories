# ADR 0003: HTML templates printed by the browser instead of server-side PDF rendering

Status: **Accepted** (owner decision D-24, 2026-10-09: export is browser print of HTML templates; the Go renderer stays as a fallback and is not extended). Written as "proposed" by spike T-054 on 2026-10-08; the sections below are the spike's original analysis. The owner is testing Safari, Android Chrome and iOS Safari and will report; the steps are in [OWNER-TESTS.md](../spikes/print/OWNER-TESTS.md). If a device class fails, the exit path (Option B) applies and needs its own decision.
Context: owner idea of 2026-10-08, related to D-06 and D-12 (pure-Go PDF engine, [ADR 0002](0002-pdf-engine.md)), L-12 (designer templates as
pre-rendered backgrounds plus slots) and the epics E05 and E08.

## Question

Can a yearbook be a web page (React components with data props, CSS `@page`), and "Export PDF" simply open the browser's print dialog
("Save as PDF"), so that the Go renderer, the design import pipeline of E08 and the export job of T-014 are not needed?

## Verdict in one paragraph

**On desktop Chromium it works and is good.** A 24-page A5 memory book with 30 photos prints to the exact page size, one sheet per page, 24 pages,
no stray blank page, all fonts embedded, Vietnamese text exact and selectable, photos at 337 ppi or more, 4 MB, about 1.3 s for the PDF after
about 0.9 s of page load. Chrome 154, Edge Dev 157 and Edge Canary 157 give the same pages (mean pixel difference to Playwright's Chromium
0.0 to 0.07 out of 255). **Firefox 157 prints it with two caveats** (tiled-gradient backgrounds come out black, and the PDF is 31 MB: photos are stored
losslessly, 7.8 MB of it, and about 21 MB is page content I did not analyse). **Safari, Android Chrome and iOS Safari were not tested** (no devices, and Safari needs a setting I did not change):
they are exactly the cells that decide the product, because most students will use a phone. Section "What the owner must test" has the steps.
My recommendation is a gate on those tests, see "Recommended decision".

## What was built (throwaway, dev server only)

- Route `/spike/print?design=memory|classic&size=a5|a4|letter&lang=en|vi` in `web/src/spike/print/` (not linked from the UI; `App.tsx` mounts it only when
  `import.meta.env.DEV`, so the production build contains neither the code nor the fonts: checked with `npm run build`).
- **Memory Book** (temp1): cover, All about me, 20 "From a Friend" pages (8 with two photos), Let's stay in touch (table of 20 contacts), back = 24 pages, 30 photos.
  **Navy classic** (temp2): cover, welcome letter, Autographs with the friends' notes paginated by JavaScript (4 pages on A5 and A4, 5 on Letter).
- Layout rules (AC1): each sheet is a box in mm (`width`/`height` in mm), `@page { size: Wmm Hmm; margin: 0 }` per page size (one size per document),
  `break-after: page` (not on the last sheet), `print-color-adjust: exact`, fonts self-hosted as woff2 (34 files, 0.9 MB, no request to Google Fonts: the test asserts that
  every request goes to localhost), photos are plain `<img loading="eager" decoding="sync">` at print resolution, text fits its box by **JavaScript** (`FitText`: shrink the
  font in 0.5 px steps down to 50 %, re-run when web fonts have loaded, flag `data-fit="overflow"` when it still does not fit). The canvas design (816 x 1056 px,
  US Letter) is scaled to the paper with CSS `zoom`; its height follows the page ratio so bottom-anchored items stay at the bottom.
- "Save as PDF" button calls `window.print()` and shows the settings to use (destination, paper size, margins none, scale 100 %, headers and footers off, background graphics on).
- Tooling in `tools/print-spike/` (own `package.json`, Playwright 1.64.0 pinned, not in `web/package.json` or CI): `npm test` (AC3), `node measure.mjs` (the matrix),
  `make-fixtures.py` (30 synthetic JPEGs; 1600 px ones are committed, a 3000 px set for the stress run is generated), `firefox-bidi.mjs`.
- Synthetic data only: invented Vietnamese names, long notes, emoji, generated photos.

## Test matrix (AC2)

Machine: Apple M3 Pro, 11 cores, 36 GB, macOS 15.6.1. All PDFs generated headless from the same URL; numbers from `docs/spikes/print/results.json`
(`node measure.mjs`). "Headless" means the engine that writes "Save as PDF" but not the print dialog.

| Browser | How | Result | Pages / page size | PDF (24-page A5) | Page ready / PDF time | Min photo ppi | Fonts | Vietnamese + emoji text |
|---|---|---|---|---|---|---|---|---|
| Chromium 156 (Playwright headless shell) | `page.pdf()` | **works** | 24 / 420 x 594.96 pt (A5) | 4.08 MB | 0.95 s / 1.35 s | 492 | all embedded | exact, emoji 4/5 as text (the fifth is in the other design) |
| Chrome 154.0.8037.98 | `page.pdf()` via Playwright channel | **works** (headless; dialog not tested) | 24 / same | 4.08 MB | 1.0 s / 2.1 s | 492 | all embedded | same |
| Edge Dev 157.0.4322.0, Edge Canary 157.0.4325.0 (Edge stable is not installed) | same | **works** (headless; dialog not tested) | 24 / same | 4.17 MB | 1.0 s / 2.0 s | 492 | all embedded | same |
| Firefox 157.0.1 (stock build) | WebDriver BiDi `browsingContext.print` | **works with caveats** | 24 / 420 x 596 pt | **30.8 MB** | 0.9 s / 8.0 s | photos stored losslessly at full size (the 72 ppi in the raw list are page-size rasters) | all embedded | names exact; emoji 3/5 |
| Safari 18.6 desktop | not run | **not tested**: `safaridriver` refuses without "Allow Remote Automation" in Safari's settings, which I did not change on the owner's browser | | | | | | |
| Android Chrome | not run | **not tested**: no device | | | | | | |
| iOS Safari | not run | **not tested**: no device (and Playwright's WebKit cannot print to PDF) | | | | | | |

Peak memory of the browser process tree while printing (sum of resident sizes, shared pages counted per process, so an upper bound): Playwright Chromium about 670 MB;
full Chrome and Edge about 3.2 GB (their whole process tree, not comparable); Firefox about 2.6 GB. Time to open the interactive print preview was **not measured**
(no automation for the dialog); page ready plus PDF time above are the proxies.

Checks that are not browser-specific, all measured on the PDFs (`pdfinfo`, `pdffonts`, `pdfimages`, `pdftotext`, poppler 26.07; PDFs regenerated by `measure.mjs`, not committed):

| Check | Chromium family | Firefox |
|---|---|---|
| Page size | A5 420 x 594.96 pt (148.2 x 209.9 mm; the exact size is 419.53 x 595.28 pt, ADR 0002 asserts the exact one), A4 594.96 x 841.92, Letter 612 x 792 | A5 420 x 596 pt |
| Orientation | portrait (follows `@page size`) | portrait |
| Margins, headers, footers | none in the PDF: `margin: 0` and the PDF has no header/footer text (headless: `displayHeaderFooter` off). **Interactive default not tested** | none in the PDF; interactive not tested |
| Backgrounds | printed (`printBackground: true`); `print-color-adjust: exact` is set; **the dialog's "Background graphics" checkbox not tested** | printed, **except tiled gradients, which come out black** (below) |
| Rotation, SVG, flat colours, border radius | correct (rotated polaroids, hearts and stars as SVG, rounded photos) | correct |
| Gradients | linear stripes (washi tape) correct. **Tiled gradients (`background-size`: the dotted paper and the ruled lines) are rasterised at 72 dpi**: visible as small squares and thick blurred rules when zoomed to 300 dpi ([crop](../spikes/print/chromium-tiled-gradient-300dpi-crop.png)) | **black**: with the dots the whole page background is black ([page 1](../spikes/print/firefox-memory-a5-p01-black-background.png)); without them the ruled-line boxes are black ([page 3, Chromium left, Firefox right](../spikes/print/chromium-left-firefox-right-memory-a5-p03.png)). Seen in poppler and in macOS PDFKit, so it is the PDF, not the viewer |
| Photos | the original JPEG bytes are passed through (no re-compression, no downsampling); 337 to 1556 ppi on these pages | decoded and stored losslessly (flate): 30 photos = 7.8 MB; the PDF is 30.8 MB, the rest is page content (not analysed; the tiled gradients are the likely cause) |
| 24 pages, 30 photos, page breaks | exactly 24 pages at A5, A4 and Letter, no blank trailing page, no page spills (sheet height equals page height) | 24 pages |
| Vietnamese text | exact and selectable (NFC). Titles set in Fredoka get a spurious space at the font switch when extracted ("ngườ i"): the diacritics Fredoka lacks come from the fallback font | same |
| Emoji | drawn with the system emoji font (Apple Color Emoji here), embedded as bitmap glyphs, extractable as text. On Windows/Android they would be Segoe UI Emoji / Noto Color Emoji: **not tested** | drawn, partly extractable |
| External requests | none (only `localhost`): fonts and photos are local | n/a |

Findings that matter beyond the matrix:

1. **Design fonts do not cover Vietnamese** (fontTools, 148 Vietnamese letters): Nunito 148, Cormorant Garamond 148, Jost 56, Fredoka 46, **Gaegu 14** (it lacks even à and é).
   The canvases embed only the Latin subset of each family and fetch the rest from Google at view time (`vietnamese` subsets exist only for Nunito and Cormorant Garamond).
   Every Vietnamese title in the handwriting (Gaegu) and rounded (Fredoka) styles falls back letter by letter to Nunito. This is true for **any** renderer, including the Go one (L-12 pre-renders
   the English text away and draws the Vietnamese text itself in Be Vietnam Pro), so the designs need Vietnamese-capable replacement fonts regardless of this decision.
2. **Photos at 3000 px** (the longest side T-009 stores): the 24-page A5 book is **62.8 MB** and takes **10.6 s** (peak tree RSS 830 MB) because Chromium passes the JPEGs through at 923 to 2909 ppi.
   The browser path needs print-size derivatives (about 1750 px for A5 at 300 ppi, about 2550 px for Letter) served for printing. With the 1600 px fixtures the same book is 4.1 MB.
3. **Scaling method**: CSS `zoom` and `transform: scale()` give the same page count and size; `transform` doubled the time (3.0 s against 1.3 s) for a 5 % smaller file. Zoom is the default.
4. **Text that shrinks to fit has a floor**: a 1,496-character note (the form allows 2,000) in the friend page's "best memory" box still overflows at 50 % of the font size (10 design px, 5 pt on A5) and is clipped.
   Six (A5) or seven (A4, Letter) other boxes shrank and fit. A real product needs a cap, a continuation page, or a smaller font floor chosen with the designer.
5. **Resolution of tiled gradients in Chromium**: the dotted paper costs 1.7 MB of rasters (4.1 MB with it, 2.3 MB without). The same dots as a data-URI SVG tile are still rasterised (3.4 MB); as an inline SVG `<pattern>` still rasterised (3.3 MB, with the ruled-line backgrounds also removed in that run);
   every dot drawn as a vector path gives no rasters at all but 6.5 MB (same run).
   Designs should avoid tiled gradients for print (use real elements or images), a rule that applies to the pre-rendered PNGs of L-12 too (those are at 192 dpi).
6. The PDF a Chromium family browser writes has page boxes rounded to 420 x 594.96 pt for A5. Invisible when printed, but a print shop's preflight that checks the exact A5 box may notice.

## What the owner must test (not tested here)

Moved to [docs/spikes/print/OWNER-TESTS.md](../spikes/print/OWNER-TESTS.md). Record the results in the matrix above.
A "works" on Chrome and Edge, Firefox (with the gradient fixed), Android Chrome and iOS Safari is what makes Option A safe.

## Effort (AC4)

What had to change to turn a canvas page into a data-bound component (the canvases are **not** JSX: each page is static HTML with inline styles in Claude Design's `x-dc` runtime with `{{year}}` placeholders, a `data-props` island and the
`Component extends DCLogic` stub; the epic text calls them React pages):

- Markup is repeated verbatim (a field row appears 7 times, a ruled box 5 times): factored into `Field`, `Ruled`, `Title`, `Icon`, `Footer`; the 816 x 1056 px absolute layout stays (inline styles kept).
- Placeholders (`{{year}}`, `[Your Name]`) become props; empty form lines become text boxes filled from data; photo placeholders (`.ph`) become `<img>`.
- Canvas quirks: `sc-camel-view-box` becomes `viewBox`; `<sc-raw-table>` elements become a real `<table>`; `repeating-linear-gradient` ruled lines become gradients measured in `1lh` so they follow a shrunk font; Google Fonts links become self-hosted woff2;
  a text-fit wrapper is added around every box that holds user text; static strings need an English and a Vietnamese dictionary; the fixed 1056 px height must follow the page ratio for A5 and A4.
- Missing in the canvas and added: print CSS, ready signal for automation, pagination.

Estimate per page (my judgement, not measured hours; the spike built 7 page types plus the infrastructure in one working session): cover, back, letter 1 to 2 h each; form page with fields, ruled boxes and photos (profile, friend) 3 to 4 h; table page 2 h;
a paginated notes page 5 to 6 h the first time (the greedy paginator is about 50 lines, the hours go into the product rules). The first design adds roughly 2 days of infrastructure (sheet and zoom, text fit, fonts, ready signal, print CSS, Playwright check); a second design adds none of that.

**Pagination of friends' notes needs**: (1) measuring every note at the real column width after fonts have loaded (the prototype renders a hidden probe column), (2) a greedy fill of columns and pages,
(3) a rule for a note taller than a column (the prototype shrinks it, it never splits), (4) page numbers and any table of contents computed after pagination, (5) one pagination per page size, because the page count changes (Letter 5 pages, A5 4),
(6) a cap on notes per book (T-034 allows 300) and a time limit. None of this exists in the Go path either: T-010 puts notes on fixed pages.

**Comparison with E08** (T-039 import tool, T-040 and T-041 pilots): the E08 path keeps the designs as pictures. It needs a dev-only Node and Playwright tool, a mapping file per page (hidden selectors, slot selectors), about 1 MiB of PNG per page in the repo,
the template v2 format, hand-written Vietnamese for every static string, and a side-by-side comparison per design. The HTML path needs a component per page type (hours above), no picture assets and no mapping format, but each design is **re-written**
as code rather than imported, and every design must be tested in every browser. I did not time the E08 tasks, so I do not claim one is cheaper; the HTML path has the lower recurring cost per design once the infrastructure exists.

## Risks that remain

- **Phones**: untested. iOS Safari's print path (pinch to PDF) and Android's print service are different engines with their own paper size and background behaviour. If they fail, the product has no export for the majority of users.
- **Dialog settings**: the user must choose the paper size, margins and scale. `@page` sets size and margin and `print-color-adjust: exact` asks for backgrounds, but how each browser's dialog treats them was not tested interactively. A wrong choice gives a wrongly sized or unbackgrounded PDF with no server to detect it.
- **No server artifact**: no stored PDF, no download link to send, no background job, no server-side warnings (low resolution, missing glyph, extra photos from T-014); the checks would have to move into the page.
- **Large books**: 300 notes and 3000 px photos mean a 60 MB PDF and, on a phone, hundreds of megabytes of decoded bitmaps (a 3000 x 2250 photo is 27 MB decoded, so 30 of them are about 800 MB; an estimate, not a measurement). Print-size derivatives are required.
- **Firefox** black tiled gradients; **Chromium** 72 dpi raster of tiled gradients: designs must be written for print, not copied from the screen.
- Fonts and emoji differ per operating system (system emoji font), so the same book looks different on Windows and Android: not tested.
- Reproducibility: output depends on the user's browser version; the server path would pin one.

## What would be retired or kept if adopted

Retired (stop work; remove later once the exit path is not needed): T-014 export job (assemble, render, store), the server-side PDF download and the PDF.js preview in T-019 (the page itself is the preview), the Go renderer path of T-010 and T-038 (`internal/pdf`, template format v2, `docs/templates.md`),
the E08 import tool T-039 and the two pilots T-040 and T-041 as specified (they become React components), and the fonts of ADR 0002 in the runtime (Be Vietnam Pro stays useful as the Vietnamese fallback in the web fonts).
Kept: the notes data model by field id (D-21, `docs/note-fields.md`, T-034), the photo pipeline T-009 (plus a print-size derivative), the book and page size model (A5, A4, Letter), the template picker UI of T-019, E08's template field declarations (spec 07), and the designs themselves.
New: the page-size and photo-derivative rules, self-hosted Vietnamese-capable fonts, a per-design browser test that runs the Playwright check of this spike.

## Recommended decision

(Spike-time text. The owner decided D-24 on 2026-10-09 without waiting for the tests: build Option A, keep the Go renderer as the fallback, and use B if a device class fails.)

I recommend **not retiring the Go path yet, and making the decision depend on the owner's phone and Safari tests above** (about 30 minutes). Reasons: the desktop Chromium result is strong and the HTML templates are a good way to author designs, but the M1 exit needs
a download that works for every student, and the cells that decide that (iOS Safari, Android Chrome, Safari) are untested, while the Go renderer already passes ADR 0002 and the T-010 and T-038 test suites.

- **Option A, browser print only**: adopt if tests 1, 2 (after fixing tiled gradients), 4 and 5 pass. Cheapest to run; no server rendering; the weakest guarantees.
- **Option B, HTML templates with server rendering by headless Chromium (the exit path)**: same components, the server loads the page and calls `page.pdf({ preferCSSPageSize: true, printBackground: true })`. Measured here: 24 A5 pages, 4.1 MB, 0.9 s load plus 1.3 s PDF with 1600 px photos
  (10.6 s and 62.8 MB with 3000 px photos), about 670 MB for the browser process tree. This gives the stored artifact, the warnings and the same result on every device, at the price of a Chromium in the runtime image, which D-06 and ADR 0002 deliberately avoided, and a concurrency limit.
  Adopt if the phone tests fail but the owner likes the HTML authoring; it also makes browser print a free extra "Print" button on desktop.
- **Option C, keep E05 and E08 as planned** (Go renderer, L-12 import): adopt if the owner wants no browser in the runtime and no dependence on the print dialog. Its cost is the per-design import work described above and the 192 dpi raster backgrounds.

Default if the owner does not run the tests: C for M1 (it is built and verified), and revisit B after M1 using this prototype.

**Exit path** from A: B is a server-side wrapper around the same components and the same print CSS (about a week: container with Chromium, signed print route, job and storage from T-014 kept, photo derivatives), so nothing written for A is lost.

## Reproduce

```
cd web && npm ci && npm run dev                      # prototype at http://localhost:5173/spike/print
cd tools/print-spike && npm install && npx playwright install chromium
npm test                                             # AC3: 24 pages, page size, fonts, Vietnamese text, photo ppi, no external request
python3 make-fixtures.py large /tmp/photos3000 && LARGE_PHOTOS=/tmp/photos3000 FIREFOX=/path/to/firefox node measure.mjs   # the matrix, writes docs/spikes/print/
```

The automated check needs poppler (`pdfinfo`, `pdffonts`, `pdfimages`, `pdftotext`) on the PATH. To keep the repository small, `docs/spikes/print/` holds only `results.json` (every measurement, including the file sizes)
and three evidence images; the PDFs are not committed (`*.pdf` there is git-ignored) and `measure.mjs` regenerates them. The interactive output of the same Chromium engine was not compared with the headless output: the dialog adds only the settings listed in OWNER-TESTS.md.
