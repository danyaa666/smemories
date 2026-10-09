# E04_T-043 — Note field catalogue (`internal/notefields`)

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

#### Description
Friends' notes become template-driven (D-21): a note is a set of answers keyed by field id, and the public form is generated from the fields of the yearbook's template. The fields come from a closed catalogue in code, like template slots,
so every id has known limits, text rules and EN/VI labels, and a template can only ask for fields we know how to validate, moderate and print. This task builds the catalogue and nothing else; T-034 stores answers and T-044 lets templates choose fields.

#### Scope
- In: package `internal/notefields` (pure Go: no HTTP, no database); the catalogue; validation of an answers map; the default field set; JSON shape for the API; docs.
- Out (do not do): endpoints, migrations, template changes (T-044), photos (they stay separate, up to three per note), rating or choice fields, and any personal-data field (phone, birthday, address, social handles) — contributors are collected minimally (E04 PRD).

#### Acceptance criteria
- [ ] AC1 — The catalogue holds exactly these fields (ids are permanent): `name` (short text, 1–60 characters), `nickname` (short text, up to 40), `relationship` (short text, up to 60), `message` (long text, 1–2000), `how_we_met`, `first_impression`, `best_memory`, `wish`, `advice` (long text, up to 500 each). "Short text" is a single line (no newline allowed); "long text" may contain `\n`.
- [ ] AC2 — Every field carries labels and hints in English and Vietnamese (`Label{En,Vi}`, `Hint{En,Vi}`; the hint is the placeholder text of the form). The Vietnamese text is drafted by the dev and listed in a table in the pull request description (id, English, Vietnamese); **the owner reviews that table before merge**.
- [ ] AC3 — `Validate(refs []FieldRef, answers map[string]string) (map[string]string, error)` where `FieldRef{ID string, Required bool}`: rejects an answer whose id is not among `refs` (`ErrUnknownField`), a missing or empty required answer (`ErrMissing`), and a value breaking its rules (`ErrInvalid`); the error type carries the field id and a stable code (`unknown_field`, `missing_answer`, `invalid_answer`). It returns the cleaned map without empty optional answers. Cleaning follows L-09 through the shared `internal/textx` helper: NFC, trimmed, `\r\n` to `\n`, control characters other than `\n` (long text only) rejected, format characters rejected except U+200D and variation selectors, length counted in characters after normalisation. Vietnamese diacritics, emoji and ZWJ sequences pass unchanged (tests).
- [ ] AC4 — `Default()` returns the default set: `name` (required), `relationship` (optional), `message` (required).
- [ ] AC5 — `Info(refs)` returns the API shape `[{"id","kind":"short_text|long_text","label":{"en","vi"},"hint":{"en","vi"},"required","max_length"}]` in the order of `refs`; unknown ids are an error. A test fixes the JSON.
- [ ] AC6 — Guard tests: the id list is asserted verbatim (a rename or removal breaks a test), every field has both labels and hints and a positive limit, and no id appears twice.
- [ ] AC7 — `docs/note-fields.md` lists the fields and the rule for adding one (new catalogue entry, limits, EN/VI text, test; never reuse or repurpose an id, because stored notes refer to ids).

#### Design
Files: `internal/notefields/{catalogue,validate,info}.go`, tests, `docs/note-fields.md`. Depends only on `internal/textx`.

```go
type Kind string // "short_text" | "long_text"
type FieldRef struct { ID string; Required bool }
type FieldError struct { ID, Code string }
```

#### Risk
`low`: a pure package with no input surface of its own; the callers (T-034) are the trust boundary.

#### Security & performance notes
Validation must be linear in the input size and bounded: reject an over-long value by length before normalising when the raw byte length already exceeds 4 × the character limit.

#### Test plan
- Dev: table tests for every field and character class (control, format, ZWJ, variation selector, combining marks, emoji, bidi override, NUL), boundaries at limit and limit + 1 in characters (not bytes), required/unknown/missing, JSON shape.
- QA should probe: a 10 MB string; strings with only whitespace; decomposed Vietnamese (`e` + combining marks) versus composed; a value with U+202E; the same id in refs twice.
