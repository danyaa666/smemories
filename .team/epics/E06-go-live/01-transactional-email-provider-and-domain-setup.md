# T-021 — Transactional email provider and domain setup

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Replace LogMailer with a real provider (SES is the natural fit on AWS) including SPF/DKIM/DMARC runbook and bounce/complaint handling. Paid service: raise an owner question before specifying.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._
