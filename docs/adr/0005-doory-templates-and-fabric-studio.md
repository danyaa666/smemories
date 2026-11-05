# ADR 0005: Templates from Doory and a Fabric.js image studio beside the DOM editor

Status: **Proposed** (spike T-169). It becomes Accepted when the leader accepts it and the owner has answered the licence points in section Q6 ("What the owner must confirm"). Decision context: owner decision D-45 (hybrid: the DOM editor stays the editor for pages, Fabric.js allowed only isolated as an image studio, templates from Doory usable; the owner's stress is **reliability of the integration**), [ADR 0004](0004-layout-editor.md) (its numbers are the bar), [ADR 0003](0003-html-print-export.md), `docs/templates.md` ("Layout block and layout document").
The prototypes and every number below are in [docs/spikes/canvas](../spikes/canvas/README.md) (`npm run measure` reproduces them; each number names its result file). Doory is pinned at commit `a65c43a98133344273e6e13cde9d2c98d0270421` (2026-11-05, tag v0.3.0 is the same day). Information for the owner on licences is not legal advice.

## Verdict in one paragraph

**Bring templates in with a build-time converter, never at run time; ship no studio in the first release.** A converter turns a Doory page into one of our layout pages (a deterministic pure function, 208 conversions give one sha256, every output passes `internal/booklayout` for all four page kinds, three converted templates print within the ADR 0004 thresholds). It converts **text well (155 of 161 text objects, 96 %)** and **decoration badly (0 of 30 lines and paths, 3 of 18 rectangles)**, because our layout has no rectangle that stretches, no line, no path and only five colours: 73 % of the 230 objects convert today, **85 % with one small registry change** (two stretching shapes). Colours always snap to the book's five theme tokens, so an imported template wears the student's theme: a feature for a themed book, a loss for a template that lives on its own palette (owner decision). A template is a **page-level template** (a layout page for one page kind, stored as the validated layout value), shipped as embedded files, saved by students in one owner-only table, applied by copying it into the book's layout in one write with one history snapshot. The Fabric studio, as a lazy chunk behind an error boundary and a flag, **can be made safe** (15 failure cases proven, 87 KiB gzip, absent from the main and print bundles) but **no job on the list needs it** that CSS/SVG, the existing image `fit`/radius controls or a vector item do not do with less weight; recommend none for the first release. **Licence: nothing converted may be committed or shipped until the owner chooses** between accepting Doory's credit line (the starter templates are under the Attribution Assurance License, not CC0) and recreating the templates ourselves with Doory only as inspiration.

## Q1. Template inventory and conversion coverage (`coverage.json`, `objects.json`, `selection.json`)

**Doory's format at the pinned commit** (`backend/data/templates/*.json`, `manifest.json`): `{schemaVersion: 1, width, height, unit, pages: [{id, name, background, objects, notes?}], meta?: {presetId}}`. A background is `solid {color}` or `gradient {colorStops}`. An object is a Fabric.js 7 JSON object with Doory additions. The 10 starter templates hold **231 objects on 13 pages** (230 leaves and 1 group):

| Type | Count | Notes |
| --- | --- | --- |
| `textbox` | 161 | `fontFamily`, `fontSize`, `fontWeight`, `fontStyle`, `lineHeight`, `charSpacing`, `textAlign`, `styles` (per-run style), `textCase`/`caseSource`, `effects` (outline, shadow) on 4 |
| `rect`, `ellipse` | 18, 15 | `shapeKind`, `rx/ry`, fill (string, 5 gradients in all), stroke, 5 ellipses and 1 rect with a stroke |
| `doryline` | 16 | stroke, `arrowStart/arrowEnd` |
| `path` | 14 | SVG path data (botanical template, a group of 2 paths among them) |
| `image` | 6 | `slot {cx, cy, width, height, angle, mask}` for 5 photos, 1 QR code |
| `group` | 1 | `layoutManager`, children relative to its centre |

All objects carry the Fabric base (`left/top/width/height`, `originX/Y`, `scaleX/Y`, `angle`, `flipX/Y`, `skewX/Y`, `opacity`, `shadow`, `visible`, `fill`, `stroke...`). Canvases are 1080 x 1080, 1280 x 720, 1080 x 1920, 1920 x 1080, 336 x 192, 480 x 672, 794 x 1123, 816 x 1056 and 1728 x 2304, in px, in or mm units. Eleven font families appear (Inter 75 texts, Crimson Text 25, Manrope 13, Poppins 10, Josefin Sans 10, Archivo 9, Great Vibes 7, Oswald 5, Anton 3, Playfair Display 3, Pinyon Script 1); 7 objects have a Fabric `angle`, 6 are scaled, 5 have a gradient fill, 2 have a shadow.

**The converter** (`src/convert.mjs`): `convertPage(design, page, {slug, kind, ratio, mode, stretch}) -> {page, report}`. **Classes:** *exact* = converted with nothing lost but the colour and font snapping to the template's tokens; *approximate* = something else was changed or dropped (listed per object by code); *cannot convert* = dropped (the report says why). No object is exact in the strict sense (strict count 0): every one has a colour or font snapped, which is how a theme works.

| Variant | Objects | Exact | Approximate | Cannot | Converted |
| --- | --- | --- | --- | --- | --- |
| **Today** (page "fit") | 230 | 54 (23 %) | 114 (50 %) | 62 (27 %) | **73 %** |
| With the two proposed stretching shapes (`fit+stretch`) | 230 | 54 (23 %) | 141 (61 %) | 35 (15 %) | **85 %** |
| Page "cover" (crop), today's shapes | 230 | 28 (12 %) | 116 (50 %) | 86 (37 %) | 63 % |

By template (converted share today / with stretch; exact share today): bold-sale-announcement 74 / 79 (21), bold-youtube-thumbnail 50 / 75 (0), botanical-celebration-invitation 62 / 90 (10), clean-business-card (2 pages) 67 / 73 (20), grand-opening-flyer 73 / 100 (7), modern-pitch-deck (3 pages) 83 / 100 (48), modern-resume 64 / 64 (16), night-bloom-concert-poster 88 / 100 (6), rooftop-event-story 88 / 100 (50), serif-quote-card 100 / 100 (0). By type today: **text 155 of 161**, photo slots 5 of 6, rectangles 3 of 18, ellipses 5 of 15, lines 0 of 16, paths 0 of 14. The 6 text objects lost are over the 40-items-per-page cap (the resume has 64 objects), not unconvertible.

**Why shapes fail today (a finding, not a converter bug):** `AssetSvg fill` draws a registry SVG with the default `preserveAspectRatio` ("meet"), so in a box that is not square a `shape.rounded-square` or `shape.circle` stays square or round and is centred in the box. Only a (near) square rectangle or circle can be shown (3 of 18 rects, 5 of 15 ellipses); a banner, a bar or a divider line vanishes (`non_square_shape`). The proposal: two registry shapes, `shape.rect` and `shape.ellipse`, drawn edge to edge (`preserveAspectRatio="none"`); the `fit+stretch` rows assume them (their ids are not in the registry, so only the "today" files are sent through the validator).

**3 best and 3 worst** (by converted share with the stretch shapes, ties by object count; before = Doory's own thumbnail, after = A5 with today's shapes, A5, A4 and Letter with the proposed shapes; `out/renders/<slug>.png`, regenerated by `npm run render`, not committed, see Q6): best **modern-pitch-deck** (100 %), **night-bloom-concert-poster** (100 %), **rooftop-event-story** (100 %); worst **modern-resume** (64 %), **clean-business-card** (73 %), **bold-youtube-thumbnail** (75 %). What the pictures show: layout, hierarchy, text and filled boxes survive and read well on all three papers; the poster loses its gradient (averaged to one colour, which snaps to the accent), its outline text and its shadows; the resume loses the 14 vector icons and timeline lines, the circle photo mask and 23 objects in all; the pitch deck's photo panel becomes the book's photo element at the same place.

**Policies (each is a constant or a branch in the converter, so it is testable):**

| Topic | Policy | Why |
| --- | --- | --- |
| Page-size mismatch | **Fit and letterbox**: scale the canvas to lie wholly on the page, centre it, fill the rest with the page background (the canvas background's nearest token). Crop ("cover") only on request. | Cover drops 10 more points of objects (63 % against 73 %), and a 1080 x 1920 story falls from 88 % to 25 % converted; fit never loses an object for its place. Price: wide canvases are small on a portrait page (16:9 and 336 x 192 fill 40 % of the page area, 1:1 70 %, 3:4 posters 94 %). Templates meant for pages should be authored portrait. |
| Paper | One conversion at the A5 ratio (the validator's own reference, 1.419). On Letter stacked items get up to 8.8 % closer (ADR 0004 Q1); sizes are in page-width units and do not change. | The layout is paper independent by design. |
| Colours | The nearest of the manifest's five tokens (`ink soft paper white accent`), measured in RGB against the default theme; a gradient is averaged first. Always *approximate*. | The validator accepts tokens only; the book re-themes everything. A template-private palette (up to 8 hex colours that become tokens of that template) would keep the art but changes the layout document and the validator: **leader/owner decision**, not in this spike. |
| Fonts | Doory family -> `display` (Anton, Oswald, Great Vibes, Pinyon Script, Playfair Display) or `body` (the rest); weight >= 600 -> `bold`; the book's Vietnamese-capable OFL families draw them. Italic, letter spacing, line height, underline and per-run styles are dropped and listed (62 line-height and 50 letter-spacing notes). | `text_fonts` is `display`/`body`; no font ships from Doory. |
| Text | Plain text, NFC, trimmed, control and format characters removed, cut at 500 code points; `textCase` is already in the text; size scaled to design px and clamped to 8..96 (3 clamps); a text box keeps its width and height, the book's fit rule shrinks it down to 50 %. Text stays text, never pixels. | Vietnamese typing, fit and print (ADR 0004 Q3). |
| Shapes | `rect` -> `shape.rounded-square` (corners become the asset's), `ellipse` -> `shape.circle`, tinted; box enlarged by the asset's inset (1/0.88, 1/0.92) so the shape covers the original; stroke, shadow, opacity dropped. Outline-only shapes are *cannot convert*. | Registry shapes only. |
| Lines | `doryline` -> a thin `shape.rect` (only with the proposed shape); arrowheads and dashes dropped; a diagonal line is *cannot convert*. | |
| Paths and QR codes | *Cannot convert*: dropped and counted. Flattening them into a design image is not done by the converter (an image needs a media id of the book's own; a shipped template cannot carry one). A vector `draw` item would carry paths (Q5), not in this plan's first tasks. | |
| Images | Never hot-linked and never copied. A photo slot with a slot box becomes the position of the template's **own photo element** (`overrides["cover.photo"]` or `profile.photo`); mask, crop and rotation are not kept; the notes and back kinds have no photo element, so the slot is dropped (counted). | The five stock photos are people (Q6); a template must not carry media ids. |
| Groups | Flattened into their children when the group is not rotated or scaled (the one group of the set is neither). | |
| Rotation, scale | Rotation about the box centre (Fabric rotates about the origin, the converter moves the centre), normalised to -180..180; scale folded into the box; skew and flip dropped (none in the set). | |
| Opacity, shadows, effects | Dropped and listed (opacity: none in the set; shadow 2, effects 4). | |
| Item cap | More than 40 items: the smallest shapes go first, then the smallest texts; deterministic; counted as `over_item_cap` (15 objects today, all in the resume). | The validator's 40 per page. |
| Backgrounds | Solid -> the nearest token; gradient -> its mean colour, then the token; image backgrounds do not occur in the set. | |

## Q2. Round trip and determinism (`determinism.json`, `validate.json`, `parity.json`)

- **Deterministic.** The converter reads no clock, no random source and no file; item ids are the first 12 hex characters of `sha1(slug|page|object id)` behind an `x`; output is canonical JSON (compact, keys sorted, as the validator re-encodes). 208 conversions (13 pages x 4 kinds x 2 shape variants x 2 page modes), 1,217,568 bytes, are **one sha256** in two separate processes with different `TZ`, `LANG` and working directory (`c8c54bab...bb264`).
- **Valid.** All 52 converted files (13 pages x the 4 page kinds `cover`, `profile`, `notes`, `back`) pass `booklayout.Validate` against the `memory-book` manifest, and each stored form is already the validator's canonical re-encoding (`canonical_same`). Before that, four negative controls (unknown asset, colour that is not a token, box off the page, 501 code points) must be refused or the run aborts, so "0 rejected" is not vacuous. `go run -tags spike ./docs/spikes/canvas/validate`; the build tag keeps it out of `go build ./...` and CI.
- **Print parity** (ADR 0004 Q5 method: the sheet on screen at paper size, device scale 2, against the browser's PDF rasterised at 192 dpi): the three best templates on A5, A4 and Letter, 9 cases. Whole-page 1-px-tolerant difference **0.03 to 0.20 %** (ADR 0004: glyph edges only, an added 0.5 % is the limit); every unrotated shape's ink box within **1 px** between screen and PDF and within 1 px of the draw rule's own geometry (limit 2 px); one page, all fonts embedded and the text in the PDF in all 9. **Limit of this proof:** the page is drawn by the spike's own renderer (`src/render.mjs`), a copy of `place()` and of the text and asset styles, not by the product's `LayoutLayer` and `FitText`; web dependencies cannot be installed in this spike. It shows that converted data stays inside the geometry the product already proved; the product's print-check must run the real runtime on the converted templates in the task that ships them.
- **Round trip back to Doory** is not offered: the conversion drops information on purpose, so an exported page is not a Doory design.

## Q3. Where templates live and how a student uses them

**A template is a page template: the value of `pages.<kind>` of a layout document** (`bg`, `overrides`, `items`) with metadata. It may contain only what the layout validator allows: `text` and `asset` items, a `bg` colour token or registry asset, overrides of the kind's editable elements (the photo position), at most 40 items. It **must not** contain: a media id (a shipped template has none; an own template has `image` items and `bg.media` stripped when saved, counted and shown to the student), user SVG, a URL (text is data, the book never turns it into a link or HTML), a script, or any id of another user's media. Names are plain text of at most 60 characters.

| Part | Model |
| --- | --- |
| **Shipped templates** | Files in the repo like the template manifests (`internal/templates/embed/...`), each `{id, label_en, label_vi, kinds, licence, source, page}`, loaded and validated in `go test` against the `memory-book` manifest with `booklayout.Validate`: **a template that fails the validator fails the build.** Licence file next to each (Q6). |
| **Own templates** | One table `book_template_tab` (id, public ULID, `owner_id`, name, `kind`, `layout_json`, `item_count`, `bytes`, `created_at`; `_tab`, BIGINT ms timestamps, no foreign key, unique `(owner_id, public id)`, index `(owner_id, created_at DESC)`). Owner only (another user's id is `ERROR_NOT_FOUND`), at most **20 per user** and **64 KiB each** (about 1.3 MB per user worst case), per-user rate limit like layout saves, **deleted with the account and the book's data export lists them** (T-025). Validated at save time against the book's manifest. |
| **Per-section choice** | There is no extra state: choosing a template for a section **is** applying it to that page kind. The section's current look is whatever the book's layout holds for that kind. |
| **Apply** | `POST /api/book-layout/apply-template {yearbook_id, base_version, kind, source: shipped|own, template_id}` under the book's row lock: copy the template's page into `pages.<kind>` (replacing that kind's `bg`, `overrides` and `items`, nothing else), re-validate the whole document like a save (`ValidateRestore` semantics: an asset no longer in the registry is dropped and counted, anything else is `ERROR_INVALID_LAYOUT`), write **one version** and **one history snapshot of the document before it** (T-163 rules, a new snapshot kind `apply`), `ERROR_VERSION_CONFLICT` on a stale `base_version`. All or nothing in one transaction: a bad template never leaves a half-applied page, and "undo" is the restore of that snapshot. |
| **Previews** | Drawn by the book's real runtime from the template's page (no stored images to keep in step). |

**Alternatives compared.** (B) *Templates as separate book-level templates* (one choice for the whole book, like `template_id`): changing one deletes the layout and its history (`OnTemplateChange`), so a student could not mix a cover from one and notes from another, and "save my page" has no home. (C) *Layout presets inside one template manifest*: only the template's author can add one, they ship with code, and a student's own page has no place to be saved; it also couples a preset to one template's editable ids. **The chosen model is the most reliable** because it adds no new document form: it reuses the one validator on every path (CI for shipped, save time for own, apply time for both), a template is paper independent, apply is a single transaction with a snapshot, and a failure leaves the book as it was.

## Q4. Studio integration reliability (`failures.json`, `bundle.json`)

The prototype (`studio/`): a book stand-in with an autosave timer, a feature flag (`?studio=off`), the studio behind `React.lazy` and an error boundary, flatten, and the upload call to `POST /api/media/create` (`purpose=design`) with the answers the product gives. Each row is a case of `npm run studio:failures` unless marked.

| Failure | Expected outcome | Evidence |
| --- | --- | --- |
| Studio chunk does not load | One message ("The studio could not be used. Your book is safe."), the book keeps its text and its autosave | **proven** (chunk request aborted) |
| Studio crashes mid-edit | Boundary message; the book is untouched and autosaves on; the studio's draft stays in the browser | **proven** (render error thrown inside the studio) |
| Export cannot allocate (a phone's `toBlob` answers `null`) | One message, no upload, draft kept | **proven** (`toBlob` stubbed to `null`) |
| Export too large | Never attempted: the flatten size is capped before any canvas exists (below) | sizes computed and timed; a real phone not tried |
| Upload fails (500) / network drops | "upload failed, your draft is kept"; the button works again | **proven** (1 request each) |
| `429` | Message with the `Retry-After` seconds, the button disabled for that time, draft kept | **proven** (`Retry-After: 7`) |
| 30-image cap (`ERROR_DESIGN_IMAGE_LIMIT`, 409) | "design image limit (30) reached: delete one first", draft kept | **proven** |
| Quota (`ERROR_QUOTA_EXCEEDED`, 409) | "storage quota exceeded", draft kept | **proven** |
| Double click on Save | Exactly one upload | **proven** (`dblclick`: 1 request) |
| Leaving with unsaved studio work | The browser asks (`beforeunload`); the draft comes back when the studio is reopened | **proven** (dialog seen, 1 object restored) |
| Success | "saved", draft deleted | **proven** |
| Flag off / studio closed | No button and no request for the chunk; the chunk is not fetched until the button is pressed | **proven** |
| Fabric version bump changes behaviour | The version is pinned exactly (`7.4.0`, no `^`); a bump is its own dependency PR that must pass this suite; the working document survives `toJSON` -> `loadFromJSON` -> `toJSON` unchanged; the document is **never stored on the server**, so a bump cannot corrupt stored data; a local draft that no longer loads is discarded with one message | round trip **proven** for rect and ellipse; draft discard on a failed load not built |

**Decisions.**
- **Flatten size.** 300 dpi on the paper's width where it fits **4.5 megapixels** (18 MB of RGBA) and 3000 px on the long edge (the server keeps at most 3000 px, `docs/media.md`); otherwise the largest size that does: **A5 1748 x 2480 (300 dpi, 4.3 MP), A4 1783 x 2521 (216 dpi), Letter 1864 x 2412 (219 dpi)** instead of 2480 x 3508 (8.7 MP) and 2550 x 3300 (8.4 MP). `flattenSize()` in `studio/src/flatten.mjs`.
- **Cost measured** (Chromium here, a noise-and-gradient image as the worst case for PNG): A5 PNG **65 ms and 5.5 MiB** (275 ms with the CPU 4x slower), JPEG q85 27 ms / 1.2 MiB (109 ms), WebP q85 354 ms / 1.3 MiB (373 ms); A4 and Letter within 5 %. The upload limit is 10 MiB (`SMEM_MEDIA_MAX_BYTES`), so even the worst PNG fits; real photographs are smaller. **Not measured:** a real phone's memory, and Safari's canvas area limit (iOS Safari has had a limit near 16.7 MP per canvas; at 4.5 MP the cap is far below it, but it was not tried). If `toBlob` fails the studio may retry once at half the pixels before it gives up (not built).
- **Format and transparency.** PNG for a picture with transparency or flat art (stickers, doodles), JPEG q85 for a full-bleed picture without transparency; the server re-encodes either way (`internal/media`). No user SVG ever enters; the studio exports pixels only.
- **The studio's working document is not stored on the server.** At most a local draft (`localStorage`, one per book, 589 bytes for one rectangle); it is deleted after a successful save and never used as the shared document of a page. A studio image is a **design image**: it counts toward the 30 per book and the 500 MiB of the owner, and once placed it is an `image` item like any upload.
- **Keyboard and screen reader.** Fabric's canvas has no keyboard model. The studio would need arrow-key nudge, delete, undo and buttons for every tool (not built, not measured). The accessible path for the essentials is the DOM editor, which already has keyboard operation; the studio is an extra, never the only way to do something.
- **Text tool: none.** Text stays real text in the DOM editor (Vietnamese Telex/VNI, 500 code points, fit, print). If the owner wants text inside the studio, it must first be measured with CDP `Input.imeSetComposition` as `docs/spikes/editor` did, plus an owner checklist on a real keyboard; not done because the recommendation is not to build it.
- **Bundle** (`bundle.json`): the studio chunk is **282,702 bytes, 87,302 gzip** (Fabric 7.4.0; React is shared with the book). The main entry's static chunks are 69,546 bytes gzip and the print entry's 68,035; **neither contains the studio chunk or the string "fabric"**, and the studio is a dynamic import only (the build manifest lists it under `dynamicImports` of the main entry and nothing for the print entry).

## Q5. Which jobs justify the studio

| Job | Cheaper way | Raster needed? |
| --- | --- | --- |
| Photo crop and filters | The `image` item's `fit` and `radius` today; a focal point and CSS filters (`grayscale`, `sepia`, `brightness`) as item fields later. The CSS filter print path was **not measured** here. | No |
| Collage | Several `image` items on one page (up to 40 items; the DOM editor places them). | No |
| Cut-out sticker (background removal) | Not a Fabric job: Doory does it with an ONNX model (U2-Net) it downloads; a new heavy dependency and its own licence, not proposed. | Yes (model) |
| Doodle | A vector `draw` item (a path from pointer events, a few dozen lines, stored in the layout, validated by size and point count); no Fabric. | No |
| Shapes with effects | Fields on the `asset` item (shadow, outline) rendered with CSS/SVG; the two stretching shapes of Q1. | No |
| Curved text | SVG `textPath` on a text item (text stays text); not measured in print here. | No |
| Brush painting, layered compositing of photos | Only here does the studio earn its weight. | Yes |

**Recommendation: no studio in the first release.** Templates first (Q8); the jobs above are reachable with items that stay vectors and stay in the validated layout. If the owner still wants a studio, the smallest first one is: shapes, free drawing and placing an uploaded photo on a page-shaped canvas, flatten at the Q4 sizes, no text, behind the flag.

## Q6. Licence and obligations (information for the owner, not legal advice)

Read at the pinned commit; quotations are verbatim.

| Obligation | Source file | Applies when |
| --- | --- | --- |
| "Redistributions of source code, in whole or part and with or without modification (the "Code"), must prominently display this GPG-signed text in verifiable form." | `LICENSE`, condition 1 | We redistribute any part of Doory's licensed material in source form |
| "Redistributions of the Code in binary form must be accompanied by this GPG-signed text in any documentation and, each time the resulting executable program or a program dependent thereon is launched, a prominent display (e.g., splash screen or banner text) of the Author's attribution information, which includes: (a) Name ("Siam"), (b) Professional identification ("Creator and maintainer of Doory"), and (c) URL ("https://github.com/shaongitbd/doory")." | `LICENSE`, condition 2 | Our product is the "program dependent thereon" of redistributed material |
| "Neither the name nor any trademark of the Author may be used to endorse or promote products derived from this software without specific prior written permission." | `LICENSE`, condition 3 | Always: no "made with Doory" marketing without written permission |
| "Anyone who redistributes it must keep this credit with it and display it prominently each time the program starts (for a web app: on its pages): "Doory by Siam, creator and maintainer of Doory, https://github.com/shaongitbd/doory"." | `NOTICE` | As condition 2; the exact credit line and where it goes |
| "Starter templates: `backend/data/templates/*.json` and `thumbs/*.webp` (ten templates), generated by `frontend/scripts/templates/` from original compositions, shapes and sample copy; no third-party artwork or logos. ... Attribution Assurance License, like the code" | `THIRD_PARTY.md`, "Assets in this repository" | **The templates' own licence is the AAL, not CC0** |
| "Built-in shapes ... CC0-1.0", "Built-in graphics ... CC0-1.0" | `THIRD_PARTY.md` | The shapes and graphics could be reused freely; the converter does not copy any (it uses our registry) |
| Five starter photos: "StockSnap, which publishes every photo under CC0 1.0 (public domain dedication: no attribution required, commercial use and changes allowed) ... The photographers are credited anyway." | `THIRD_PARTY.md`, "Starter template photos" | Free to use; **they show real people** (a CC0 dedication of the photo does not clear a person's likeness), and our books are made by minors: do not ship them |
| Fonts the starter templates use (Anton, Archivo, Lora, Oswald, Pinyon Script, Playfair Display, and Great Vibes, Inter, Manrope and others in the files): "OFL-1.1", fetched from Fontsource | `THIRD_PARTY.md`, "Fonts" | We ship none: the converter maps to our own Vietnamese-capable OFL families |
| Fabric.js "(MIT) Copyright (c) 2008-2015 Printio (Juriy Zaytsev, Maxim Chernyak); Copyright (c) 2016-present Andrea Bogazzi, Shachar Nen and Fabric.js contributors" | `NOTICE` (Doory's notices), the package's own `LICENSE` | Only if a studio ships: keep the MIT notice with the bundle (a `THIRD_PARTY` line and the licence text) |
| "Each keeps the copyright in their own contributions and licenses them under the project's Attribution Assurance License" (4 named contributors besides Siam; 13 distinct committer names in `git log`) | `AUTHORS.md` | One grantor is not enough to give a written exception: the author alone cannot waive terms for others' templates (unclear) |

**Answers.** *Does using only Doory's data format, with none of its code, trigger the credit?* The **format** (the shape of a JSON file, which we re-derived and implemented ourselves) carries no obligation we can find. The **templates' content** is under the AAL, and a converted template is a modified copy of that content, so the conservative reading is that conditions 1 and 2 apply to whatever we ship that is derived from them, including the signed `LICENSE` text and the credit line. *Where would the credit appear if it does?* The AAL says "each time the program starts (for a web app: on its pages)": on the template gallery screen and on the app's footer or "about" page, with the signed `LICENSE`, `NOTICE` and `licenses/siam-gpg.asc` kept in a `docs/third-party` folder. A printed book is output, not the program starting; we read the credit as **not** going on the printed pages (owner and, ideally, the author to confirm).
*Which templates may be shipped as is?* **None, until the owner chooses.** Under choice A below all 10 may ship as converted files (photos replaced by our photo element), with the credit; under choice B none of them ships and ours are authored by hand. Independently, none ships with its English sample copy: the shipped texts are Vietnamese and ours.

**What the owner must confirm:**
1. **Choice A or B.** (A) Accept the credit line (name, role, URL) on the gallery and the app footer, keep the signed licence text, convert the templates we like. (B) Recreate: our designers author our own pages in our format, using Doory's only as inspiration (the converter stays a development tool to bootstrap and measure; nothing converted is committed). A third route is asking the maintainers in writing for an exception, which (see `AUTHORS.md`) may need the contributors' agreement.
2. That the credit stays off the printed pages.
3. That no stock photo from Doory ships (people).
4. Whether a "made with / templates from Doory" mention may be used in marketing (condition 3 forbids it without permission).
5. Fabric.js (MIT) only if the studio is built.

Until the owner answers, this PR commits the converter and the numeric report (counts, object ids, types, classes) and **not** the converted templates or the renders.

## Q7. Dependency and supply-chain health (`bundle.json`)

- **Fabric.js**: pin **7.4.0** exactly (latest on npm, released 2026-05-18, MIT, no required dependencies; `jsdom` and `canvas` are optional and for Node only, not bundled). Cadence from npm: 6.9.1 2025-12-15, 7.0.0 2025-12-22 (a major), 7.1.0 2025-12-31, 7.2.0 2026-02-18, 7.3.1 2026-04-19, 7.4.0 2026-05-18: a minor every one to two months and a major in the last year, so **bumps are deliberate** (a dependency PR with the failure suite, manual CI run). `npm audit` on the prototype (123 packages including dev, 6 production): **0 vulnerabilities**.
- **Bundle cost**: 87,302 bytes gzip lazy (Q4), absent from the main and print bundles.
- **Doory's health, as far as a one-time import matters** (local clone only): first commit 2026-10-10, **277 commits in 26 days**, 13 distinct committer names (the top three by commits are not Siam), tags v0.1.0 (2026-10-19), v0.2.0 (2026-10-23), v0.3.0 (2026-11-05), no GitHub releases, 2,277 files. Young and fast-moving: its JSON schema (`schemaVersion: 1`) and its licence may change. **No Doory release can affect production**: nothing from Doory runs, the converter reads files at a pinned SHA, and what we ship are our own converted files, which keep the licence they had at the pinned commit. A future Doory template is brought in by a deliberate re-run at a new SHA, a diff of the report and a new look at the licence file.

## Q8. Recommendation and task plan

**Recommendation:** bring templates in through the converter, store them as validated page templates, apply them per section in one transaction; do not build the studio now; let the owner decide the licence route first. Reliability comes from one rule: **every template, shipped or student-made, passes the same validator, and apply is one write with one snapshot.**

**Task plan** (replaces the rest of the stage 2 plan; `O` = needs an owner decision, `L` = leader decision):

| # | Task | Notes | Depends |
| --- | --- | --- | --- |
| 1 | **[O] Licence route for Doory templates** (A credit, B recreate) | Owner answers Q6 points 1 to 4; nothing converted is committed before this | none |
| 2 | **Stretching shapes**: registry `shape.rect` and `shape.ellipse` (authored, licence file), `AssetSvg` draws a flagged asset edge to edge (`preserveAspectRatio="none"`); update registry and web tests, print-check cell | small; raises the conversion from 73 % to 85 % | none |
| 3 | **Page template model and shipped storage**: embed loader, manifest fields, `go test` validating every shipped template with `booklayout.Validate` (build fails if one is invalid), licence files | the one validator | 1 for content |
| 4 | **Own templates and apply API**: `book_template_tab` migration (expand-only), store/service/handler, `/api/book-layout/apply-template` with row lock, `ValidateRestore` drop-and-count, snapshot kind `apply`, 20 per user, 64 KiB each, media stripped on save, rate limit, account deletion and export hooks, OpenAPI, Postman, integration tests (owner only, conflict, atomicity, limits) | high risk (permissions, migration, concurrency) | 3 |
| 5 | **Converter as a tool** (`tools/doory-import`, node, from this spike's `src/convert.mjs`): pinned SHA, report, deterministic output, `--check` in CI against the committed files; ours-only after route B | | 1, 2 |
| 6 | **Shipped templates**: 6 to 10 page templates per kind with Vietnamese copy, either converted (A) or authored (B), licence files, print-check on A5/A4/Letter with the real runtime | content + design | 1, 3, 5 |
| 7 | **Template gallery and apply UI per section** (cover, profile, notes, back): previews by the real runtime, keyboard, i18n, the credit line if A | web | 3, 4 |
| 8 | **"Save my page as a template"**: create endpoint use, name, limits, delete, "images are not saved" message | web + api | 4 |
| 9 | **Page-by-page editing flow**: edit a page or the whole book page by page, save, print, return to the book manage page | web; uses the existing editor | none |
| 10 | **[L/O] Template palette** (optional): a template-private palette of up to 8 hex colours so imported art keeps its colours; changes the layout document and the validator | decide after seeing task 6 results | 6 |
| 11 | **[O] Image studio** (only if the owner wants it after Q5): Fabric 7.4.0 (new dependency, MIT notice), lazy chunk, flag, error boundary, flatten caps of Q4, the 15 failure cases as a test suite, no text tool, keyboard operation, security review (no user SVG) | not in the first release | 1, 7 |
| 12 | **Vector `draw` item and item effects** (optional): doodles and curved text as items that stay in the layout | replaces most of the studio's jobs | 9 |
| 13 | **Tests, security review and docs** for 3 to 8: Postman collection, e2e for apply per section, print-check of an applied template, `docs/templates.md` and a `docs/spikes` pointer | | 3 to 8 |

## Not proven

- The prototype's pages are drawn by a **copy** of the product's draw rule, not the product's `LayoutLayer` and `FitText`; parity on the real runtime is a requirement of task 6.
- Fonts: texts are English sample copy drawn in Nunito and Baloo 2; Vietnamese composition is ADR 0004's result and was not repeated; the studio has no text tool.
- Real phones: the 4x CPU throttle is Chromium on one machine; iOS Safari memory and canvas limits were not tried; Firefox and WebKit were not driven.
- Studio keyboard operation and screen readers are not built.
- Colour snapping was judged against the default `lavender` theme only; the other themes shift the five tokens.
- The `fit+stretch` outputs are not validator-tested (their two asset ids are not in the registry yet); they are the converter's own claim until task 2.
- The licence reading is mine, from the files; it is not legal advice.
