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
| `how_we_met` | long text | 500 | How we met | Chúng ta quen nhau thế nào |
| `first_impression` | long text | 500 | First impression | Ấn tượng đầu tiên |
| `best_memory` | long text | 500 | Best memory together | Kỷ niệm đẹp nhất của chúng ta |
| `wish` | long text | 500 | My wish for you | Điều mình ước cho bạn |
| `advice` | long text | 500 | My advice | Lời khuyên |

Short text is one line; long text may contain `\n`. The default set (`Default()`) is `name` (required),
`relationship` (optional), `message` (required). Photos are separate (up to three per note), and no
personal-data field (phone, birthday, address, social handles) belongs in the catalogue.

## Adding a field

1. Add an entry at the end of `fields` in `catalogue.go` with kind, limit, and label and hint in English and Vietnamese.
2. Add the id and limit to `TestCatalogueGuard` (the id list is asserted verbatim) and a case to `TestValidate` if the field has a new rule.
3. Have the Vietnamese text reviewed by the owner.
4. Never rename, remove, reuse or repurpose an id: stored notes refer to ids. Retire a field by no longer offering it in templates.
