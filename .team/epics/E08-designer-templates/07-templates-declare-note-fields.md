# T-044 — Templates declare note fields (format v2.1)

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
With template-driven notes (D-21) a template says which fields its friend pages ask for and where each answer is printed. This task adds that to the template format and the renderer and exposes the field list of a template to the API (used by T-034 for the public form).
It builds on the field catalogue (T-043) and the v2 format (T-038).

#### Scope
- In: `note_fields` in the template file; the slot `note_field`; renderer input for notes as answers; the helper `templates.NoteFields(id)`; docs; the two built-in templates declare the default set.
- Out (do not do): the public form (T-018), storing answers (T-034), new catalogue fields (T-043), rating or choice fields.

#### Acceptance criteria
- [ ] AC1 — A template may declare `"note_fields":[{"id":"name","required":true},{"id":"how_we_met"},{"id":"message","required":true}]` (1 to 9 entries, ids from the catalogue, no duplicates). Without it the default set of T-043 applies. Validation errors name the template and the entry.
- [ ] AC2 — In a notes page's `flow.elements`, `{"type":"text","slot":"note_field","field":"how_we_met", …}` prints that answer; `field` must be one of the template's `note_fields`. The old slots `note_author`, `note_relationship` and `note_message` keep working as aliases of the fields `name`, `relationship` and `message` (so `classic` and `modern` need no change). A text element whose field has no answer draws nothing (no label either), as for any slot.
- [ ] AC3 — The renderer input for a note is `Note{ID, Answers map[string]string, Photos []Photo}`; the previous author/relationship/message fields are replaced by answers (update `internal/pdf` tests and the sample test). Output for `classic` and `modern` is unchanged.
- [ ] AC4 — `templates.NoteFields(templateID string) ([]notefields.FieldRef, bool)` returns the declared set (or the default set when none is declared; `false` for an unknown template id). A test covers both built-in templates and a fixture template with custom fields.
- [ ] AC5 — A text box for a long-text field is validated for height like other text boxes; a field declared but never placed on the notes page is allowed (the answer is collected but not printed) and reported by a validator warning in the test output, not an error.
- [ ] AC6 — `docs/templates.md` documents `note_fields`, the `note_field` slot, the aliases and the rule that changing a template's fields later never loses collected answers (answers are stored by id; unused ones are simply not printed).

#### Design
Files: `internal/templates/{spec,validate,load}.go`, `internal/pdf/{render,text}.go`, `internal/notefields` (read-only), `docs/templates.md`, tests; `internal/templates/embed/{classic,modern}.json` only if needed.

#### Risk
`low`: no endpoint or migration; extends embedded-file validation.

#### Security & performance notes
Answers are already cleaned by T-043 when stored; the renderer still treats them as hostile (the existing normalisation and length limits apply).

#### Test plan
- Dev: validator table tests (unknown id, duplicate, field not declared, too many); renderer test with answers for each kind including Vietnamese and an emoji ZWJ sequence; alias equivalence test (old slot output equals new slot output byte for byte for the same data).
- QA should probe: a notes page where one note has all answers empty except `name`; the longest allowed answers in every box (truncation with an ellipsis and a warning); switching a book between two templates with different fields (nothing lost, nothing crashes).

#### Leader note from D-24 (2026-10-09)
Templates are now HTML components in the web app (T-059, T-060), but the API still needs each template's id, names, supported page sizes and note fields (the public form, `GET /v1/templates`, validation of `template_id`). Do the manifest part of this task (`note_fields`, `templates.NoteFields`) and allow a manifest that has `"renderer":"html"` and **no pages**; skip the Go renderer changes (`note_field` slot, `pdf.Render` input) unless a Go template needs them.
