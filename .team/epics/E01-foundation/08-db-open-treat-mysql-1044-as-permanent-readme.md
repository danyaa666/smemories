# E01_T-032 — db.Open: treat MySQL 1044 as permanent; README warning about bare docker compose

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
From the T-030 QA notes. (1) For an unprivileged account an unknown database (or a missing grant) answers MySQL error 1044, not 1049, so db.Open still retries for the full 10 s: add 1044 to permanent() with a test. (2) A bare 'docker compose' outside make still targets the old shared 'smemories' project and a bare 'down -v' would wipe its volumes: add a short warning to the README's compose section.

_BACKLOG: needs a full spec before it moves to TODO; tiny, can be bundled into the next db task._
