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
- `theme.font`: the body font family; `theme.fonts` maps the roles `body` and `display` to families (see
  [Font families](#font-families)). Set `font`, `fonts.body` or both (they must then agree); `display` defaults to
  the body family. Emoji come from a bundled monochrome fallback font automatically.
- `theme.colors`: named colours as `#rrggbb`. `ink` (default text), `accent` and `paper` (page background) are
  required; add any others (for example `tint`) and refer to them by name from elements.
- `pages`: exactly one page of each kind, in the order they appear in the book: `cover`, `profile`, `notes`,
  `back`. Limits: at most 16 pages, 64 elements per page, 256 KB of JSON.

## Elements

Every element has `type`, `x`, `y`, `w`, `h` (the box, which must lie inside the page, or inside the item for
note elements). Fields that do not belong to the type are rejected.

| `type` | Fields | Notes |
|---|---|---|
| `text` | `slot` or `text`, `size`, `min_size`, `align`, `bold`, `color`, `line_height`, `label`, `font`, `rotate` | See fitting rules below. Exactly one of `slot` and `text` ([static text](#static-text)). `color` is a theme colour name (default `ink`). `align` is `left` (default), `center` or `right`. `line_height` is a multiple of the font size (1 to 3, default 1.3). `label` maps `en`/`vi` to a prefix such as `"Hobbies: "`, drawn only when the slot has text (not allowed with `text`). `font` is the role `body` (default) or `display`. |
| `image` | `slot`, `fit`, `shape`, `radius`, `rotate` | `fit` is `cover` (the only value): the photo is scaled to cover the box, centred, the overflow clipped. `shape` is `rect` (default), `rounded` (needs `radius`, at most half the shorter side), `circle` (needs `w` equal to `h`) or `ellipse` (the oval inscribed in the box, a circle when `w == h`; `radius` is rejected). |
| `rect` | `fill`, `radius`, `rotate` | A filled decoration, no slot. `fill` is a theme colour name; `radius` rounds the corners. |
| `background` | `asset` | A page-level image that fills the whole page, see [Backgrounds](#backgrounds). Takes no box. |

Elements are drawn in order, later ones on top. The page is first filled with the `paper` colour, then the
page's `background` (wherever it stands in the list) is drawn, then the other elements.

## Backgrounds

```json
{"type": "background", "asset": "memory-book/cover.png"}
```

A decoration image behind everything else on a page (gradients, shadows and borders live in the image, not
in elements). At most one per page; not allowed inside a notes `flow`. On the notes page it repeats on every
copy of the page. `asset` is `<template id>/<file>`: a file in the template's own folder
`internal/templates/embed/<id>/` (embedded with `go:embed`), a plain file name ending in `.png`, `.jpg` or
`.jpeg`, no sub-folders, no `..`. The same file may be used on several pages; it is stored once in the PDF.
Vector (SVG/PDF) backgrounds are not supported.

The validator checks every asset when the templates load and the tests check it again (a bad asset fails the
tests, never a request). Each rule names the template, page and file:

| Rule | Limit |
|---|---|
| The file exists in `embed/<id>/` | |
| Content is PNG or JPEG (the content decides, not the extension; a JPEG named `.png` is accepted) | |
| File size | at most 1.5 MiB |
| Pixel shape | width / height equal to the reference page's within 1 % |
| Effective resolution on the reference page | 150 to 400 DPI (A5 reference: about 874 to 2331 px wide; Letter: 1275 to 3400 px) |
| All assets of one template together | at most 8 MiB |

Prefer RGB PNGs without transparency: a PNG with an alpha channel is decoded in memory by the PDF library
(about 170 MB at the largest allowed size), an opaque PNG or a JPEG is copied through.

## Static text

```json
{"type": "text", "text": {"en": "All About Me", "vi": "Về mình"}, "x": 20, "y": 18, "w": 90, "h": 14,
 "size": 22, "font": "display", "color": "accent", "rotate": -2}
```

Fixed wording that does not come from the book, in both languages (`en` and `vi`, both required, each 1 to 500
characters, no control characters, so no line breaks). It is drawn in the yearbook's language (`Options.Lang`;
English when the language is unknown) and follows the same fitting rules as slot text. It is not a slot, so it
is never skipped for missing data. Text that does not fit still ends with `…` and reports `text_truncated` (with
no `slot`), and a character in no font reports `missing_glyph`.

## Rotation

`rotate` (degrees, -45 to 45, default 0, **clockwise**) turns a `text`, `image` or `rect` around the centre of
its box. The box is placed, wrapped and fitted as if it were not rotated, and must lie inside the page that
way; an image's clip (`rounded`, `circle`, `ellipse`) rotates with it. The rotated corners may extend past
the page edge by the amount the rotation implies.

```json
{"type": "image", "slot": "photo", "x": 30, "y": 40, "w": 60, "h": 60, "shape": "ellipse", "rotate": 3}
```

## Font families

The embedded families live in a registry in `internal/pdf/fonts` (`fonts.Register`, called from `init`): a name
and the TTF data of the regular and the bold face (a family may have only one; the renderer then uses it for
both weights). `theme.fonts` maps roles to family names:

```json
"theme": {"fonts": {"body": "BeVietnamPro", "display": "BeVietnamPro"}, "colors": {...}}
```

A text element picks a role with `"font": "display"` (default `body`). `theme.font` keeps meaning "the `body`
family", so `{"font": "BeVietnamPro"}` is still valid. An unknown family or role fails validation. A family is
embedded in the PDF only when an element uses it. Registered today: `BeVietnamPro` (SIL OFL, regular and bold).

`Register` refuses (panics) a family that is not a valid TTF or lacks any Vietnamese letter with each of the five
tone marks, upper and lower case; a test checks the whole registry as well. Emoji work with every family
through the bundled fallback font. A new family also needs its licence text next to the font file and an entry
in `THIRD_PARTY_NOTICES.md`.

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

The element's font family is used first. Characters it lacks (emoji, some symbols) come from the bundled monochrome emoji
font; emoji are outlines in the text colour, never colour pictures. A character found in no font is drawn as `?`
and reported as `missing_glyph` with the character and where it was.

## Images

JPEG and PNG are supported. JPEG data is embedded as is (no re-encoding). The renderer reports
`low_resolution` (with the media id) when a photo lands below 300 DPI at its printed size, and draws a neutral
grey placeholder plus `missing_image` when a photo cannot be fetched or decoded.

## Report warnings

`pdf.Render` returns a `Report` whose `Warnings` carry a `Code`, the 1-based `Page`, and as applicable `Slot`,
`NoteID`, `MediaID`, `Rune`: `missing_glyph`, `text_truncated`, `low_resolution`, `missing_image`,
`extra_photos`. None of them fails the render. Warnings of static text carry no `slot`.

## Deterministic output

`Options.Now` sets the PDF creation and modification dates. With it fixed, the same input gives byte-identical
output, with one caveat from the PDF library: it writes image objects sorted by pixel width and, for photos of
the *same* width, in random order, so such photos can swap object numbers between renders (the pages look
identical). Text, layout and fonts are always stable.

## Adding a template

1. Copy `internal/templates/embed/classic.json` to a new file and change `id`, `name` and the layout. Put its
   background images in `internal/templates/embed/<id>/`.
2. Run `go test ./internal/templates ./internal/pdf`: the validator reports any bad element by page and number.
3. Run `go test ./internal/pdf -run TestSamples -write-samples` and open `docs/templates/<id>.pdf` (add the new id to
   the loop in `TestSamples` first).
