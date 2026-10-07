# T-014 — Export job: assemble book, render PDF, store, download

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Asynchronous export: POST creates a job (one active export per book), a bounded worker maps DB rows to the pdf.Book value, renders with T-010, writes the PDF to storage, and exposes status (queued, running, done, failed with a safe reason) and an authorised download. Budget: 24 pages / 30 photos within 60 s and 512 MB. Survives restart (job rows in MySQL, stale running jobs requeued).

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._
