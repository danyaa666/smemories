# Templates

A template is a JSON file that says where things go on each page of a yearbook PDF. The Go renderer
(`internal/pdf`) turns a template plus the book data into a PDF; adding a template needs no Go code:
drop a file in `internal/templates/embed/` and the tests in `internal/templates` validate it.

Sample output for review (synthetic photos and invented names only): [classic.pdf](templates/classic.pdf),
[modern.pdf](templates/modern.pdf). Regenerate with `go test ./internal/pdf -run TestSamples -write-samples`.

> Since decision D-24 (ADR 0003) templates are HTML components in the web app. The manifest described under
> [Manifest only: html templates](#note-fields-and-html-manifests) is what the API reads from them; the Go renderer
> and the page/element format below remain as a frozen fallback for `classic` and `modern`.

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

## Note fields and html manifests

`note_fields` (optional) lists the questions the friend page asks for this template, in order:

```json
"note_fields": [{"id": "name", "required": true}, {"id": "how_we_met"}, {"id": "message", "required": true}]
```

- 1 to 9 entries, each an `id` from the catalogue in [note-fields.md](note-fields.md), no duplicates, `required`
  optional (default false). A wrong or repeated id fails validation naming the template and the entry
  (`template "x", note_fields entry 2 ("crush"): not in the note field catalogue`).
- Left out, the default set applies (`name` required, `relationship`, `message` required). `classic` and `modern`
  declare exactly that set.
- `templates.NoteFields(id)` returns the set (or the default one; `ok` is false for an unknown template) and
  `notes.FieldsFor(templateID)` uses it for the public note form and for checking a submission. A book without a
  template, or whose template no longer exists, gets the default set.
- **Changing a template's fields later never loses answers.** Answers are stored by field id (see
  [note-fields.md](note-fields.md)); a field the template no longer asks for is simply not shown or printed, and a
  new field is empty for older notes.

`signature` (optional boolean, default `false`; E11, T-095) switches on the handwritten signature of a note: only a template that says
`"signature": true` makes `public-collect/get` report `signature.enabled`, shows the pad on the friend page and lets `submit-note` accept a
`signature` field (otherwise `ERROR_PARAM`). Any other value than a boolean fails validation. `templates.SignatureEnabled(id)` reads it (false for an
unknown id). Format and limits: the section "Signature" of [note-fields.md](note-fields.md). The built-in manifests turn it on in T-097 (print) and T-096 (web).

`"renderer": "html"` (default `"go"`) marks a template drawn by the web app. Its manifest has only `id`, `name`
(`en` and `vi`), `page_sizes` (any of `A5`, `A4`, `Letter`; no aspect-ratio rule) and `note_fields`: no `pages`, `unit`,
`reference` or `theme` (the validator rejects them); it may also carry `signature` and the `design` block (see "Design block and book designs"). `templates.List()` returns Go and html templates, with a
`Renderer` field; `pdf.Render` refuses an html template.

The built-in html template is `memory-book` (T-059, the pastel Memory Book of design `temp1`; the page components are
`web/src/book/templates/memory.tsx`). Its form asks `name` and `message` (required), `how_we_met`, `first_impression`,
`best_memory` and `wish`; the page also prints `nickname`, `relationship` and `advice` when a note has them (older
notes, or a later manifest that asks for them).

Signatures (T-097): `memory-book` and `navy-classic` declare `"signature": true`. A note's drawn signature is printed by the shared
`web/src/book/BookSignature.tsx` (inline SVG, ink only). In `memory-book` it is floated at the bottom right of the message's text box, 220 x 88 design px
(40 x 16 mm on A5); the text wraps beside it, and a signed page gives the message the room (the photo and the fields beside it are 30 px shorter, the card's minimum
height is 150 px), so a message that fits without a signature fits with one (print check: font at most 1 px smaller, never cut off); in `navy-classic` it is at the right end of the name line,
125 x 50 design px (about 23 x 9 mm on A5), inside the 58 px the name block already had, so the four-per-page capacity and the 50 % font floor are
unchanged; with a signature the name wraps to a second line beside it (a 60-character name is not cut off), and a note without a signature keeps today's layout. `classic` and `modern` (the Go renderers) do not declare it and print none.

The second one is `navy-classic` (T-060, the navy and gold canvas of design `temp2`; `web/src/book/templates/navy.tsx`):
cover, profile (welcome-letter layout with an oval portrait), "Autographs" notes pages with four friends each (message on
ruled lines, name as the signature) and a back page in the cover's frame. Its form asks `name` and `message` (required) and
`relationship`. The design has no place for the cover photo or the friends' photos, so the book does not load them.
Since T-145 it is customisable (see "Themes, font sets, sections and texts of `navy-classic`" below).
Samples and the canvas-to-template pictures are in `docs/templates/navy-classic-*`.

## Design block and book designs

E09 stage 1 (T-141, decisions D-24, D-41). A student customises the **choices a template declares**; the web template decides what each id looks like and the
browser prints it, so the screen the student edits is the print itself. The server holds the choices and validates them; it draws nothing.

**The manifest `design` block** (optional, html templates only: the validator fails a Go-renderer template that has one):

| Key | Rule |
|---|---|
| `themes` | 1..8 ids; the first is the default |
| `font_sets` | 1..6 ids; the first is the default |
| `sections` | up to 16 `{id, hideable, movable, fixed}`; `fixed` is `"first"`, `"last"` or `null`. At most one first and one last, listed at the front and the end; a fixed section is not movable. The list is the default order |
| `texts` | up to 16 `{id, max}`: a fixed text of the template the student may replace; `max` 1..200 characters |
| `images` | up to 8 `{id, choices}`: a picture slot; 1..8 choice ids, the first is the default |

Every id matches `[a-z0-9._-]{1,40}` and is unique in its list. `templates.Design(id)` returns the parsed block (`ok` false for none). `memory-book` (the pilot)
declares themes `lavender` (today's colours), `peach`, `mint`; font sets `playful` (Nunito, Baloo 2, Patrick Hand: today's) and `classic`; sections `cover` (fixed first),
`profile` (hideable, movable), `notes` (movable) and `back` (fixed last, hideable); the fixed texts `memory.tsx` prints (`cover.belongs`, `cover.sticker`,
`profile.heading`, `profile.subtitle`, `notes.heading`, `back.heading`, `back.subtitle`, `back.pact`; their default wording stays in the web locale files, per
language; `back.pact` allows 18 characters, because the pact sits in a small box and at 30 emoji it wrapped into two tiny lines); and the image `cover.pattern` with `dots` (today's) and `plain` (no pattern). A web test pins the web ids to the manifest ids (T-142).

**The options document** of a book (`book_design_tab.options_json`, one row per yearbook, at most 4096 bytes, `template_id` records the template it was made for):

```json
{"v":1,"theme":"peach","fonts":"classic","sections":{"order":["cover","notes","profile","back"],"hidden":["back"]},"texts":{"profile.heading":"Về mình"},"images":{"cover.pattern":"plain"}}
```

Every key is optional (`null` is the same as leaving it out). `POST /api/book-design/update` replaces the whole document and answers `ERROR_INVALID_DESIGN` (400, the
message names the field, nothing is stored) for: a document over 4096 bytes, not an object, with data after it, an unknown key at any level, a wrong type, a `v` other than 1;
an unknown theme, font set, section, text, image or image choice; an `order` that does not list every section exactly once, or that puts a non-movable section anywhere but
its place in the template's order (which keeps `fixed` first and last where they are); a `hidden` section that is unknown, repeated or not `hideable`; a text that is
over its `max` characters (counted after trimming), has a control character (a newline or tab included), a bidi or other format character, a line separator or invalid UTF-8.
Texts are NFC-normalised and trimmed; an empty text is dropped (the template's own wording is used). They are plain text and the web renders them as text, never as markup.
A book whose template has no `design` block cannot be customised (`ERROR_INVALID_DESIGN`). The document is stored normalised, so what `book-design/get` returns is what was stored.

**Endpoints** (owner only; another user's or unknown book is `ERROR_NOT_FOUND`; no email verification): `GET /api/book-design/get?yearbook_id=` returns `{template_id, options, updated_at, design}`
(`options` and `updated_at` null while the book uses the defaults; `design` is the template's block, null when it has none); `POST /api/book-design/update` `{yearbook_id, options}` answers
the same shape; `POST /api/book-design/reset` `{yearbook_id}` deletes the row and is idempotent. Update and reset share a limit of 60 per 10 minutes per user.
The contract is `api/openapi.yaml`; the Postman folder is `book-design` in `postman/customisation-editor.postman_collection.json`.

**Lifecycle.** Changing a book's template (`yearbook/update` with another or no `template_id`) deletes its design row in the same transaction, so a design is only ever valid for the
template it was made for (answers and photos survive); deleting the book deletes the row (`yearbook.ChildPurger`, orphan pair registered in `dbtest`). Lock order: the `yearbook_tab`
row first, then `book_design_tab`.

**What the print gets.** `GET /api/book-data/get` carries `design`, the stored options *resolved* against the manifest, so the web never needs the manifest to print:
`{theme, fonts, sections: {order, hidden}, texts, images}` with every default filled in (`order` always lists every section, a non-movable one at its place; `texts` holds only the replaced
texts; `images` lists every slot). On read, an id the manifest no longer has (a retired theme, a removed section) is dropped silently and the default applies, a text that no longer
passes the rules is ignored, a document that cannot be read is the defaults, and a row made for another template is ignored. `design` is null when the book has no template or its
template has no `design` block. Not covered here: the class book (T-146) applies each student's own design to their pages; uploads of backgrounds, free text styling and element editing are stage 2.

### Adding a theme, font set, section, text or picture to an html template (T-147)

A template author does not need to read the runtime. For `<tpl>` (`memory-book`, `navy-classic`) with prefix `<p>` (`mb`, `nc`), the page component file `web/src/book/templates/<name>.tsx` and its `.css`:

**A theme** (a colour set):
1. `internal/templates/embed/<tpl>.json`: add the id to `design.themes` (at most 8, `[a-z0-9._-]{1,40}`; the first is the default, so append).
2. `<name>.css`: add a block `.<p>-page[data-theme="<id>"] { ... }` that defines **every** custom property the other theme blocks define (`--mb-*` or `--nc-*`). Nothing else in the CSS or the TSX may hold a colour (a unit test reads both files).
3. **The contrast rule:** body text must reach 4.5:1 (WCAG 2.1 AA) on every surface it sits on: `memory-book` checks `ink` and `soft` on `paper`, `white` and the six pastels (16 pairs); `navy-classic` checks light on dark, gold on dark, dark on light and bronze on light. `design.test.ts` fails below 4.5; decoration (the accent colours, rules, shadows) is not held to it. Put your ratios in the table of the template's section above.
4. `web/src/book/templates/<name>.tsx`: add the id to the template's `design.themes` array at the bottom of the file (the runtime and the customise screen read it; `design.test.ts` fails if it differs from the manifest or if the CSS lacks the block).
5. `web/src/locales/en.json` and `vi.json`: `customise.tpl.<tpl>.themes.<id>` (the name on the chip; without it the id shows).
6. `tools/print-check/check.test.mjs`: add the id to the theme loop of the template's "themes x font sets" describe (see "What print-check covers"), run `make print-check` and `cd web && npm test`.

**A font set:** the same six steps with `font_sets`, the `--<p>-body`/`--<p>-disp`/... properties under `[data-fonts="<id>"]`, `design.fonts`, `customise.tpl.<tpl>.fonts.<id>`, and a `SETS` entry in print-check. Only Vietnamese-capable OFL families (D-18), self-hosted woff2 under `web/src/book/fonts/` with the licence in its `README.md` and a `@font-face` in `fonts.css`; a font set must request only its own families' files.

**A text:** add `{id, max}` to `design.texts` (id like `profile.heading`, max 1..200), draw the template's own wording through `Own`/`useDesign().text(id, fallback)` (the box shrinks a longer replacement, so give it a `*-fit` class and a floor), add the id to `textDefaults` (the locale key of the default wording, shown as the placeholder) and `customise.tpl.<tpl>.texts.<id with dots as underscores>` in both locales, and add it to the "every text at its maximum length" print-check case.

**A picture choice:** add `{id, choices}` to `design.images` (the first choice is the default), draw the slot from `useDesign().image(id)` (an unknown choice must draw the default), add `customise.tpl.<tpl>.images.<id with underscores>.label` and `.choices.<choice>` in both locales, and a print-check case for the new choice.

**A section:** the runtime knows four page kinds (`cover`, `profile`, `notes`, `back`; `SECTION_IDS` in `web/src/book/design.tsx`). A template can change which of them are `hideable`/`movable`/`fixed` in the manifest and add the name `customise.tpl.<tpl>.sections.<id>`; a new kind of page is a code change in `design.tsx` and `BookPages.tsx`, not a manifest edit.

`go test ./internal/templates` validates the manifest (limits, unique ids, a Go-renderer template may not have a `design` block); `cd web && npm test && npm run lint:i18n` check the ids, the contrast and the locale keys. A retired id is not an error for a stored design: it is ignored and the default applies.

### What print-check covers (T-147)

`make print-check` (`tools/print-check/check.test.mjs`, about 160 tests, 5-6 min) renders fixture books in Chromium, prints them to PDF and inspects the PDF. Every theme x font set x size of both customisable templates is a cell with **two tests**: the render (page count, page size, all fonts embedded, that set's fonts only, self-hosted files, no foreign host, no blank page, names and emoji present) and the stress test (long Vietnamese names and emoji everywhere: no text cut off, nothing outside its page, none wider than its box, the set's fonts loaded at the ready signal).

| Template | Themes | Font sets | Sizes | Cells | Tests |
|---|---|---|---|---|---|
| `memory-book` (24 pages, 21 notes) | `lavender`, `peach`, `mint` | `playful`, `classic` | A5, A4 | 12 | 24 |
| `navy-classic` (9 pages, 21 notes) | `navy`, `burgundy`, `forest` | `classic`, `modern` | A5, A4 | 12 | 24 |

Not per cell, but on both templates: hidden and reordered sections (A5 and A4: page kinds in order, none blank, photos loaded at ready), every text at its maximum length in both font sets, a design this build does not know draws the defaults, `memory-book` also the plain cover pattern, a hidden profile, a hidden back page and both hidden, and a class name with a 1 in `classic`; the class book with each student's own design (T-146, six students). Letter is covered for the design-less book and the signature check only. The manual run of the journey is `docs/e2e.md` ("The customisation journey").

**Adding a theme to the table:** add its id to the loop of the template's describe (`for (const theme of ["lavender", "peach", "mint"])` in "memory-book themes x font sets", the same in the `navy-classic` one, which reads the ids from the manifest for the section cases), run `make print-check`, and add 2 x sizes to the Tests column here.

## The picker and the print screen (T-061)

- `GET /api/template/get-list?page_size=` (session, `internal/templateapi`) lists `templates.List()` (all) or
  `templates.ForPageSize(size)` with `id`, `name` (`en`, `vi`), `page_sizes`, `renderer` and `preview`, the path
  `/template-previews/<id>.jpg` on the web app's own origin. The previews are files in `web/public/template-previews/`;
  **an html template needs one** (a Go test fails without it). Render the first page of its sample PDF, for example
  `pdftoppm -jpeg -jpegopt quality=82 -f 1 -l 1 -scale-to-x 360 -scale-to-y -1 -singlefile docs/templates/<sample>.pdf web/public/template-previews/<id>`.
- The owner's choice is saved by `POST /api/yearbook/update` (or `create`) with `template_id`; `null` clears it (the
  built-in plain layout). The yearbook service refuses (`ERROR_INVALID_TEMPLATE`, 400) an id the registry does not know, one that
  does not declare the book's page size (the size after the same request), and a page-size change on a book whose
  chosen template does not support the new size. A template id the registry no longer knows does not block a size change.
- The picker (`web/src/book/TemplatePicker.tsx`) offers the plain layout plus every listed template whose `renderer`
  is `html` **and** which `web/src/book/templates/index.ts` can draw, so a new html template appears in the picker when its
  manifest and its web component are both in. A Go-renderer template (`classic`, `modern`) is not offered: the print
  screen cannot draw it.
- The print guide (`web/src/book/guide.ts`, `PrintGuide.tsx`) picks the steps from the user agent: Chrome and Edge on a
  computer, Safari on a Mac, Firefox, Chrome on Android (Samsung Internet and others that say `Chrome/`), Safari on an iPhone or
  iPad (every iOS browser is WebKit; an iPad in desktop mode is told apart by its touch screen), and generic steps otherwise.
  The page cannot see the print dialog, so it cannot warn about a wrong paper size or missing backgrounds: the guide says so.
- `POST /api/print-feedback/create` (public, CSRF origin check, 30 per IP per hour) counts "Does the PDF look right?" in
  `print_feedback_tab`, one row per (`browser_family`, `verdict`) with `hits`; no user, book, IP or user agent is stored.
  `browser_family` is the guide id: `chromium_desktop`, `safari_desktop`, `firefox`, `chrome_android`, `safari_ios`, `other`.
  Read it with `SELECT browser_family, verdict, hits FROM print_feedback_tab ORDER BY browser_family, verdict`.

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
| Content is PNG or JPEG (the content decides, not the extension; a JPEG named `.png` is accepted); a PNG must not be interlaced (Adam7), the renderer cannot read it | |
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

### Themes and font sets of `memory-book` (T-142)

The web template draws the ids the manifest lists (`web/src/book/templates/memory.tsx` registers them, `memory.css` defines them). `BookPages` hands `book-data/get`'s `design` to the pages
through `DesignProvider` (`web/src/book/design.tsx`); `useDesign()` returns `{theme, fonts, text(id, fallback), image(id)}`, and a missing design or an id this build does not know is the first
(default) id of the manifest. Each page carries `data-theme` and `data-fonts`; `web/src/book/design.test.ts` pins the ids to `memory-book.json`.

**Themes** are CSS custom properties `--mb-*` defined once per theme under `.mb-page[data-theme="..."]`; nothing else in `memory.css` or `memory.tsx` holds a colour. `lavender` is exactly the
colours of design `temp1` (a book with no design prints the same pixels as before: checked page by page against the previous build, A5 and A4). The slot names (`lav`, `yellow`, `pink`, `peach`,
`mint`, `sky`) are the hues of `lavender`; `peach` (warm apricot and coral) and `mint` (green and blue) put their own hue in the slot. The dotted paper picture (ink at 13 %) is shared by the themes.
Text is `ink` (body) or `soft` (footer, subtitles, table cells) on the page paper, white and the six pastels. Contrast ratios, `ink / soft` (WCAG 2.1; the unit test fails below 4.5:1):

| Theme | Text colours | paper | white | lav | yellow | pink | peach | mint | sky |
|---|---|---|---|---|---|---|---|---|---|
| `lavender` | `ink #3b2f4a`, `soft #5e5170` | 12.0 / 7.1 | 12.4 / 7.3 | 9.6 / 5.6 | 10.9 / 6.4 | 9.3 / 5.5 | 9.7 / 5.7 | 10.1 / 5.9 | 9.9 / 5.8 |
| `peach` | `ink #4a2c22`, `soft #7a4332` | 12.1 / 7.6 | 12.5 / 7.8 | 10.0 / 6.3 | 10.9 / 6.8 | 9.2 / 5.7 | 9.4 / 5.9 | 10.5 / 6.6 | 10.1 / 6.3 |
| `mint` | `ink #1f3b34`, `soft #35564d` | 11.9 / 8.0 | 12.1 / 8.1 | 10.0 / 6.7 | 10.7 / 7.2 | 9.4 / 6.3 | 10.2 / 6.8 | 9.7 / 6.5 | 9.4 / 6.3 |

The three accent colours (`--mb-a1..a3`, the icons) are decoration, not text, and are not held to the ratio.

**Font sets** are `--mb-body`, `--mb-disp` and `--mb-cute` under `.mb-page[data-fonts="..."]`. `playful` is today's: Nunito, Baloo 2 (titles), Patrick Hand (handwriting). `classic` is Cormorant Garamond
(titles, and the handwriting lines in italic) over Be Vietnam Pro (body): families already shipped in `web/src/book/fonts/` with the whole Vietnamese alphabet (OFL, `README.md` there), so no new file was added.
A `@font-face` is fetched only when text uses it, so a book in one set never requests the other set's files (print check). The ready signal restarts when the font set changes
(`Book.tsx` key) and waits until `document.fonts` has stayed loaded. Be Vietnam Pro ships one weight in the book's own files (500), but the app's `@fontsource` faces of the same family (400, 500, 600 and 700, `web/src/styles/fonts.css`)
also apply to the book, so a bold label is drawn with the real Bold face, not a synthetic bold. In `classic` the Cormorant roles (`.disp`, `.cute`) use lining figures (`font-variant-numeric: lining-nums`), because its old-style 1 looks like a capital I. Print check: every theme x font set on A5 and A4 with the Vietnamese fixture (24 pages, none blank,
embedded fonts of that set only, long names and emoji everywhere not cut off, no request to another origin).

### Sections, texts and images of `memory-book` (T-143)

`BookPages` assembles the book from `design.sections` (`printSections` in `web/src/book/design.tsx`): the order of `sections.order` without the `hidden` ones, so `pageCount`, `profilePageCount`
and the ready signal count only what prints, and the photos of a hidden section are not fetched. No design, or an order that is not exactly this build's four sections (`cover`, `profile`, `notes`, `back`), prints
all four in the template's order. `ProfilePages` (the class book, T-130) follows the same function, so T-146 only has to hand it each student's design.
The eight fixed texts are the slots `Own` in `memory.tsx` (`useDesign().text(id, fallback)`; a web test pins them to the manifest). The template's own wording keeps its plain markup; a replacement is drawn in a one-line
box of the same size that shrinks it (`.mb-title-fit`, `.mb-sub-fit`, `.mb-belongs-fit`, `.mb-sticker-fit`, `.mb-pact-fit`) and is reported like other own boxes; the text is React text, never markup. A heading may take at most
the width left of the "No." tag and the sparkle (`.mb-title` max-width), so a long one shrinks instead of running under them. `cover.pattern`: `dots` is the dotted paper (`memory-dots.png`), `plain` draws the cover
without it; the other pages keep the paper, and a choice this build does not know draws `dots`. Print check: hidden profile, hidden back, both hidden, notes before profile (A5 and A4: page kinds in order, none blank, photos loaded at ready),
every text at its maximum length in both font sets, the plain pattern, and a class name with a 1 in `classic`.

### Themes, font sets, sections and texts of `navy-classic` (T-145)

The same runtime as `memory-book` (T-142, T-143), for `navy.tsx` and `navy.css`; `design.test.ts` pins the ids to `navy-classic.json`. Each page (`Page` in `navy.tsx`) carries `data-theme` and `data-fonts`.
Themes `navy` (today's colours, the default), `burgundy` and `forest`; font sets `classic` (today's) and `modern`; the four sections as in `memory-book` (`cover` fixed first, `profile` hideable and movable,
`notes` movable, `back` fixed last and hideable); five fixed texts and no picture slots (`images: []`: the design has none).

**Themes** are six custom properties under `.nc-page[data-theme="..."]`: `--nc-dark` (cover and back background, text on the cream pages), `--nc-light` (cream pages, text on the dark ones), `--nc-gold` (frames,
accents on the dark pages), `--nc-bronze` (small accents on the cream pages), `--nc-photo` (empty oval) and `--nc-rule` (the ruled lines). `navy.css` and the SVG pieces of `navy.tsx` hold no other colour (a unit test).
`navy` is exactly the canvas: a book with no design prints the same pixels as before (checked page by page against the previous build: 18 pages, A5, A4 and Letter, byte-identical PNGs at 110 dpi).
Contrast ratios of the four text pairs (the unit test fails below 4.5:1):

| Theme | light on dark | gold on dark | dark on light | bronze on light |
|---|---|---|---|---|
| `navy` | 12.8 | 6.3 | 12.8 | 5.6 |
| `burgundy` | 13.0 | 7.4 | 13.0 | 6.2 |
| `forest` | 11.5 | 6.0 | 11.5 | 6.2 |

**Font sets** are `--nc-serif`, `--nc-caps` and `--nc-italic` under `.nc-page[data-fonts="..."]`. `classic` is today's Cormorant Garamond (italic) over Be Vietnam Pro. `modern` is Nunito for both roles,
upright (Nunito has no italic, so `--nc-italic` is `normal`; no slanted copy is synthesised) and `font-size-adjust: 0.4`, which brings its taller, wider letters to Cormorant's x-height so the layout and the fit floors
hold. Nunito is already shipped with the whole Vietnamese alphabet (`web/src/book/fonts/README.md`); no file was added, and the two sets share no family, so a book requests only its own set's files (print check).
The default keeps Cormorant's old-style figures (the year on the cover is drawn with them); a replaced text does too.

**Texts** (`Own` in `navy.tsx`, same rules as `memory.tsx`): `profile.eyebrow` (40), `profile.heading` (24), `notes.eyebrow` (40), `notes.heading` (24), `back.thanks` (80). The farewell line ("Farewell, Class of 2026") takes the
year and is not replaceable. The template's wording keeps its plain markup; a replacement is drawn in a box of the same size that shrinks it (`.nc-eyebrow-fit`, `.nc-title-fit`, `.nc-thanks-fit`, floor 35 %).

**Customise screen.** It needed one generic fix: the colour chips and the font sample borrowed `memory.css`'s variables, so a template now declares `swatch` (`web/src/book/templates/index.ts`: its page class, the colour
variables to show and the class that sets a sample word); everything else is the template's `customise.tpl.navy-classic.*` locale keys.
Print check (`tools/print-check`, "navy-classic themes x font sets, sections and texts"): every theme x font set on A5 and A4 (9 pages, none blank, that set's fonts only, long names and emoji not cut off, signatures keep the page count
and never meet a text), hidden and reordered sections on A5 and A4, every text at its maximum length in both font sets, and an unknown design drawing the defaults.
