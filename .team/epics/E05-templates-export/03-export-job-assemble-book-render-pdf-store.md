# E05_T-014 — Export job: assemble book, render PDF, store, download

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Asynchronous export: POST creates a job (one active export per book), a bounded worker maps DB rows to the pdf.Book value, renders with T-010, writes the PDF to storage, and exposes status (queued, running, done, failed with a safe reason) and an authorised download. Budget: 24 pages / 30 photos within 60 s and 512 MB. Survives restart (job rows in MySQL, stale running jobs requeued).

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the designer-templates decisions (2026-10-08)
A yearbook can now have `page_size` `Letter` (T-037, D-16). The export must refuse (`400 template_page_size_mismatch`, clear message) a template whose `page_sizes` does not include the book's page size, using the helper T-037 adds
(`templates.ForPageSize`). The export budget (24 pages, 30 photos, 60 s, 512 MB) must hold with full-page background images (T-038); measure with a Letter book that uses a designer template once T-040 exists.


#### Leader notes from decision D-21 (2026-10-08)
Pass each approved note to `pdf.Render` as `Note{ID, Answers, Photos}` (T-044 changes the renderer input from fixed author/relationship/message fields to answers). Notes are printed with the fields the book's template declares; answers for fields it does not declare stay stored but unprinted.
