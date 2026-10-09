# E05_T-035 — T-010 follow-ups: template tests iterate templates.List()

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
From the T-010 QA notes: adding a template exactly as docs/templates.md describes fails TestListHasBuiltIns and needs a Go edit to TestSamples. Make both tests iterate templates.List() (and assert the built-in ids only as a minimum), so a new template needs no Go change, as the docs promise.

_BACKLOG: tiny; needs a one-paragraph spec before TODO._
