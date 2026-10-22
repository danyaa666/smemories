# Note fields

A friend's note is a map of answers keyed by field id. The public form is generated from the fields a
yearbook's template asks for, and every field comes from the closed catalogue in
[`internal/notefields/catalogue.go`](../internal/notefields/catalogue.go). Text rules are the shared ones
(`internal/textx`, decision L-09): NFC, trimmed, `\r\n` to `\n`, no control characters (except `\n` in long
text), no format characters (except U+200D and variation selectors), length counted in characters.

| Id | Kind | Limit (characters) | English label | Vietnamese label |
|---|---|---|---|---|
| `name` | short text | 60 | Your name | Tên của bạn |
| `nickname` | short text | 40 | Nickname | Biệt danh |
| `relationship` | short text | 60 | Your relationship | Mối quan hệ |
| `message` | long text | 2000 | Your message | Lời nhắn |
| `how_we_met` | long text | 500 | How we met | Chúng mình quen nhau thế nào |
| `first_impression` | long text | 500 | First impression | Ấn tượng đầu tiên |
| `best_memory` | long text | 500 | Best memory together | Kỷ niệm đẹp nhất của chúng ta |
| `wish` | long text | 500 | My wish for you | Điều mình ước cho bạn |
| `advice` | long text | 500 | My advice | Lời khuyên |

Short text is one line; long text may contain `\n`. The default set (`Default()`) is `name` (required),
`relationship` (optional), `message` (required). Photos are separate (up to three per note), and no
personal-data field (phone, birthday, address, social handles) belongs in the catalogue.

The form is read with `GET /api/public-collect/get?token=` (`data.fields`) and answered with `POST /api/public-collect/submit-note?token=` (multipart
fields `answers` as a JSON object of field id to text, up to three `photos`). A rejected answer is `ERROR_MISSING_ANSWER` or
`ERROR_INVALID_ANSWER` (the message names the field id, never the value); an id that is not in the form is `ERROR_PARAM`.

## Signature

A note can carry one optional handwritten signature (epic E11, decision L-14), stored as **vector strokes**, never as an image. It is not a
catalogue field: it is a separate multipart part `signature` of `POST /api/public-collect/submit-note`, and only a template whose manifest says
`"signature": true` ([templates.md](templates.md)) shows the pad (`data.signature.enabled` of `GET /api/public-collect/get`) and accepts one; for any
other book the part is `ERROR_PARAM`.

Format v1, canonical JSON: `{"v":1,"w":1000,"h":400,"s":[[x0,y0,x1,y1,...],...]}`.

| Rule | Limit |
|---|---|
| numbers | integers only (no floats, exponents, strings or negatives); `0 <= x <= 1000`, `0 <= y <= 400` |
| keys | exactly `v` (= 1), `w` (= 1000), `h` (= 400), `s`; no extra, duplicate or differently cased key |
| strokes | 1 to 12; each at least 2 points (4 numbers, an even count) |
| points | at most 600 in all strokes together |
| size | at most 3072 bytes after the server re-serialised it (the byte limit usually binds before 600 points: three-digit coordinates give about 400) |
| extent | the bounding box is at least 5 % of `w` wide or 5 % of `h` high (no empty speck) |
| input | at most 4 KiB read from the client; one `signature` part; an empty part is invalid |

- **Re-serialised.** The server parses into a struct (`internal/signature`) and stores `json.Marshal` of it in `note_tab.signature_json` (`VARCHAR(3072)`, NULL =
  none); the client's string, its spacing and key order are never stored. Reads (`note/get-list`, `book-data/get`) parse the stored value again and answer
  `null` for anything that is not valid, so the API can only ever return integers.
- **Errors.** Any breach is `ERROR_INVALID_SIGNATURE` (400); the message names the rule, never the content. It is checked right after the text and before any
  photo is processed, so a bad signature stores nothing and costs no image work. A request that fails here is not counted against the submission limits.
- **No biometrics, no logging.** Only positions are kept: no timing, pressure, speed, pointer type or device data. The content is never logged, put in an
  error or used as a metric label.
- **Moderation.** The signature is part of the note row: it follows the note's status (pending, approved, hidden), is returned to the owner by `note/get-list`
  in every status and by `book-data/get` for approved notes only, and is deleted with the note, with the yearbook, and is covered by any account deletion or
  data export (E06 T-025), which work on the note row.
- **Drawing it.** Clients build an SVG path from the numbers (T-096, T-097); never as user-supplied markup or an uploaded file.

## Adding a field

1. Add an entry at the end of `fields` in `catalogue.go` with kind, limit, and label and hint in English and Vietnamese.
2. Add the id and limit to `TestCatalogueGuard` (the id list is asserted verbatim) and a case to `TestValidate` if the field has a new rule.
3. Have the Vietnamese text reviewed by the owner.
4. Never rename, remove, reuse or repurpose an id: stored notes refer to ids. Retire a field by no longer offering it in templates.
