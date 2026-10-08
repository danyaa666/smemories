# Templates

A template is a JSON file that says where things go on each page of a yearbook PDF. The Go renderer
(`internal/pdf`) turns a template plus the book data into a PDF; adding a template needs no Go code:
drop a file in `internal/templates/embed/` and the tests in `internal/templates` validate it.

Sample output for review (synthetic photos and invented names only): [classic.pdf](templates/classic.pdf),
[modern.pdf](templates/modern.pdf). Regenerate with `go test ./internal/pdf -run TestSamples -write-samples`.

## Units and coordinates

All numbers are **millimetres** on the template's **reference page**, origin at the top-left, `y` growing
down. Font sizes are points. The reference page is `A5` (148 x 210 mm, the default) or `Letter` (US Letter,
215.9 x 279.4 mm); see `reference` below. The renderer scales positions and sizes to the requested page size
(A4 is 210 x 297 mm), so one spec serves every size listed in `page_sizes`. `"unit": "mm"` is required; any
other value is rejected.

| Page size | Width x height | PDF MediaBox (pt) |
|---|---|---|
| `A5` | 148 x 210 mm | 419.5 x 595.3 |
| `A4` | 210 x 297 mm | 595.3 x 841.9 |
| `Letter` | 215.9 x 279.4 mm | 612 x 792 |

## File layout

```json
{
  "id": "classic",
  "name": {"en": "Classic", "vi": "Cổ điển"},
  "unit": "mm",
  "reference": "A5",
  "page_sizes": ["A5", "A4"],
  "theme": {"font": "BeVietnamPro", "colors": {"ink": "#1b1b1b", "accent": "#7a2e2e", "paper": "#fffdf8"}},
  "pages": [ ... ]
}
```

- `id`: lower case letters, digits, `-` and `_`, at most 32 characters; also the file's logical name.
- `name`: display name for both `en` and `vi`.
- `reference`: the page the millimetre coordinates refer to, `A5` (default, may be left out) or `Letter`. Every
  element must lie inside the reference page.
- `page_sizes`: any of `A5`, `A4`, `Letter`, but only sizes with the same aspect ratio as the reference (within
  1 %): an `A5` reference allows `A5` and `A4`; a `Letter` reference allows `Letter` only (its ratio, 0.773, is
  not the A series' 0.707, so a Letter design prints on Letter only). Anything else fails validation naming the
  template. A template can only be used for a book whose page size it lists (`templates.ForPageSize`); the
  renderer refuses any other size.
- `theme.font`: the font family. The only one embedded is `BeVietnamPro` (SIL OFL, regular and bold, full
  Vietnamese coverage). Emoji come from a bundled monochrome fallback font automatically.
- `theme.colors`: named colours as `#rrggbb`. `ink` (default text), `accent` and `paper` (page background) are
  required; add any others (for example `tint`) and refer to them by name from elements.
- `pages`: exactly one page of each kind, in the order they appear in the book: `cover`, `profile`, `notes`,
  `back`. Limits: at most 16 pages, 64 elements per page, 256 KB of JSON.

## Elements

Every element has `type`, `x`, `y`, `w`, `h` (the box, which must lie inside the page, or inside the item for
note elements). Fields that do not belong to the type are rejected.

| `type` | Fields | Notes |
|---|---|---|
| `text` | `slot`, `size`, `min_size`, `align`, `bold`, `color`, `line_height`, `label` | See fitting rules below. `color` is a theme colour name (default `ink`). `align` is `left` (default), `center` or `right`. `line_height` is a multiple of the font size (1 to 3, default 1.3). `label` maps `en`/`vi` to a prefix such as `"Hobbies: "`, drawn only when the slot has text. |
| `image` | `slot`, `fit`, `shape`, `radius` | `fit` is `cover` (the only value): the photo is scaled to cover the box, centred, the overflow clipped. `shape` is `rect` (default), `rounded` (needs `radius`, at most half the shorter side) or `circle` (needs `w` equal to `h`). |
| `rect` | `fill`, `radius` | A filled decoration, no slot. `fill` is a theme colour name; `radius` rounds the corners. |

Elements are drawn in order, later ones on top. The page is first filled with the `paper` colour.

## Slots

Slots are a closed set defined in code (`templates.Slots`); an unknown slot, or one used on the wrong page kind
or element type, fails validation with the template, page and element named. A slot with no data is skipped
(nothing is drawn, not even its label).

| Page kind | Text slots | Image slots |
|---|---|---|
| `cover` | `title`, `school`, `class`, `year` | `cover_photo` |
| `profile` | `full_name`, `nickname`, `quote`, `hobbies`, `plans` | `photo` |
| `notes` (page) | none (decoration only) | none |
| `notes` (each note, see below) | `note_author`, `note_relationship`, `note_message` | `note_photo_1`, `note_photo_2`, `note_photo_3` |
| `back` | `motto`, `title`, `school` | none |

## Notes pages

A `notes` page repeats as often as needed. Its own `elements` (decoration) appear on every copy; the `flow`
describes the repeating block:

```json
"flow": {"x": 12, "y": 16, "w": 124, "h": 182, "gap": 4, "item_h": 56, "elements": [ ... ]}
```

Items of height `item_h` are stacked in the box (`x`, `y`, `w`, `h`) with `gap` between them; as many as fit go
on a page (`floor((h + gap) / (item_h + gap))`, at least 1, at most 20), then a new page starts. Element
coordinates inside `flow.elements` are relative to the item's top-left corner and must stay inside `w` x `item_h`.
A book with no notes still gets one (empty) notes page. A note with more photos than image slots shows the
first ones and the renderer reports `extra_photos`.

## Fitting rules

Text never leaves its box. For each text element the renderer:

1. normalises the text to NFC, removes control characters and invisible joiners (so an emoji ZWJ sequence draws
   as its separate emoji), and cuts it at 20,000 characters;
2. wraps by words, and by character when a single word is wider than the box;
3. if it does not fit, shrinks the font in 0.5 pt steps down to `min_size` (default: `size`, no shrinking);
4. if it still does not fit, keeps the lines that fit and ends the last one with `…`, and reports
   `text_truncated`.

A text box must be tall enough for one line at its smallest size (the validator checks).

## Fonts and characters

Be Vietnam Pro is used first. Characters it lacks (emoji, some symbols) come from the bundled monochrome emoji
font; emoji are outlines in the text colour, never colour pictures. A character found in no font is drawn as `?`
and reported as `missing_glyph` with the character and where it was.

## Images

JPEG and PNG are supported. JPEG data is embedded as is (no re-encoding). The renderer reports
`low_resolution` (with the media id) when a photo lands below 300 DPI at its printed size, and draws a neutral
grey placeholder plus `missing_image` when a photo cannot be fetched or decoded.

## Report warnings

`pdf.Render` returns a `Report` whose `Warnings` carry a `Code`, the 1-based `Page`, and as applicable `Slot`,
`NoteID`, `MediaID`, `Rune`: `missing_glyph`, `text_truncated`, `low_resolution`, `missing_image`,
`extra_photos`. None of them fails the render.

## Deterministic output

`Options.Now` sets the PDF creation and modification dates. With it fixed, the same input gives byte-identical
output, with one caveat from the PDF library: it writes image objects sorted by pixel width and, for photos of
the *same* width, in random order, so such photos can swap object numbers between renders (the pages look
identical). Text, layout and fonts are always stable.

## Adding a template

1. Copy `internal/templates/embed/classic.json` to a new file and change `id`, `name` and the layout.
2. Run `go test ./internal/templates ./internal/pdf`: the validator reports any bad element by page and number.
3. Run `go test ./internal/pdf -run TestSamples -write-samples` and open `docs/templates/<id>.pdf` (add the new id to
   the loop in `TestSamples` first).
