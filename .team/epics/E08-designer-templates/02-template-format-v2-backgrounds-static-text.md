# T-038 — Template format v2: backgrounds, static text, rotation, ellipse, font families

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
The designer templates need what the JSON format cannot say today: a decoration image behind each page, labels that are fixed text in two languages, tilted frames, oval portrait masks and fonts other than Be Vietnam Pro (L-12, D-18).
This task extends the format and the renderer, keeping `classic` and `modern` working unchanged. It adds no design.

#### Scope
- In: element type `background`; static localised text; `rotate`; image `shape: "ellipse"`; per-template font families; validator rules; docs; sample regeneration.
- Out (do not do): new page kinds or slots (D-17), gradients or shadows as elements (they live in the background image), vector (SVG/PDF) backgrounds, the fonts themselves (each template task adds the fonts it needs), user-supplied assets.

#### Acceptance criteria
- [ ] AC1 — `{"type":"background","asset":"<id>/<file>.png"}` fills the whole page, drawn right after the `paper` fill and before every other element; at most one per page; the asset path is relative to `internal/templates/embed/`, inside the template's own folder `embed/<id>/`, no `..`, embedded with `go:embed`. Validation: the file exists; the content (not the extension) is PNG or JPEG; at most 1.5 MiB; the pixel ratio matches the reference page within 1%; the effective resolution is between 150 and 400 DPI at the reference page size; a template's assets total at most 8 MiB. Each rule has a test that names the template, page and file in the error.
- [ ] AC2 — Static text: a `text` element may carry `"text":{"en":"…","vi":"…"}` instead of `slot` (exactly one of the two; both languages required, each at most 500 characters, no control characters). It is drawn in the language of the yearbook (`en` or `vi`), with the same fitting rules as slot text (docs/templates.md). Static text is not a slot: it never produces a warning for missing data.
- [ ] AC3 — `rotate` (degrees, −45 to 45, default 0, clockwise) on `text`, `image` and `rect` rotates the element around the centre of its box. Text wrapping and fitting happen in the unrotated box; image clipping (rect, rounded, ellipse) rotates with the element. The box must still lie inside the page before rotation.
- [ ] AC4 — Image `shape` also accepts `"ellipse"` (a circle when `w == h`); `radius` is rejected for it.
- [ ] AC5 — Font families: a registry in `internal/pdf/fonts` maps a family name to embedded TTF data for regular and bold (a family may have only one). `theme.fonts` maps roles to family names, for example `{"body":"Nunito","display":"Fredoka"}`; a text element may set `"font":"display"` (default `body`); `theme.font` keeps meaning "the `body` family" so existing files stay valid. An unknown family or role fails validation. Every registered family must pass a Vietnamese coverage test (all Vietnamese letters with every tone mark, upper and lower case) before it can be registered; a test enforces it for the whole registry. The emoji fallback works with every family.
- [ ] AC6 — A template with the new features renders without warnings in a test that covers: a background (page pixel equals the asset at a probe point), static text in both languages, a rotated text and image, an ellipse photo, a second font family. `classic.pdf` and `modern.pdf` samples render exactly as before (bytes may differ only by the library's known image ordering caveat).
- [ ] AC7 — `docs/templates.md` documents every new field with an example and the validation limits. T-035's shared tests (`templates.List()`) cover the new rules for every template.

#### Design
Files: `internal/templates/{spec,validate,load}.go`, `internal/pdf/{render,text,image,fonts}.go`, `internal/pdf/fonts/` (registry; no new font files beyond a test fixture family), `docs/templates.md`, tests in both packages.

Spec examples:
```json
{"type":"background","asset":"memory-book/cover.png"}
{"type":"text","text":{"en":"All About Me","vi":"Về mình"},"x":20,"y":18,"w":90,"h":14,"size":22,"font":"display","color":"accent","rotate":-2}
{"type":"image","slot":"photo","x":30,"y":40,"w":60,"h":60,"shape":"ellipse","rotate":3}
```
fpdf has `TransformBegin`, `TransformRotate`, `ClipEllipse`; use them, wrap each in a deferred end so a panic cannot leave a transform open (the renderer already recovers fpdf panics).

#### Risk
`low`: no endpoint, no migration, no user input; the spec is loaded from embedded files only.

#### Security & performance notes
Assets are validated at test time and again at load; a bad asset must fail the build's tests, never the request path. Reuse of one background across pages relies on fpdf's image registry (register by content hash, once). The export budget (24 pages, 30 photos, 60 s, 512 MB) is re-measured in the PR with a Letter-size book that uses full-page backgrounds.

#### Test plan
- Dev: validator table tests for each rule above; renderer tests per feature; coverage test over the registry; a benchmark or test for the export budget with backgrounds.
- QA should probe: a background with the wrong ratio, a 5 MiB file, a JPEG renamed to .png (accepted by content), a path with `..`; static text with a very long string in a tiny box (truncation warning, no crash); rotation at ±45°; the same template rendered in `en` and `vi`.

#### Leader notes from the T-037 review (2026-10-08)
Two small clean-ups to include here since you touch the same validator and spec code: (1) `templates.Dims` is an exported mutable map; make it unexported behind a function (for example `templates.PageDims(size) ([2]float64, bool)`) and update the callers in `internal/pdf` and `internal/yearbook` if any; (2) a lower-case `"reference": "letter"` currently produces a second, misleading aspect-ratio error next to the correct "invalid reference" one: stop checking `page_sizes` against the reference when the reference itself is invalid.
