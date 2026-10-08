# T-019 — Web: template picker, PDF preview (pdf.js) and export/download

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Choose a template (thumbnails from sample renders), trigger export, show progress, preview the real PDF with pdf.js (works on phones), download. Show renderer warnings (low resolution, truncated text, missing glyph) in plain language.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the designer-templates decisions (2026-10-08)
The picker lists only the templates that support the book's page size (`templates.ForPageSize`, T-037): an A5/A4 book sees `classic` and `modern`, a Letter book sees the designer templates (T-040, T-041 and later). Show each template's name in the UI language
(`name.en`/`name.vi`). The page-size selector of the yearbook form (T-016) offers A5, A4 and Letter. The preview must handle Letter pages (aspect ratio 0.773).
