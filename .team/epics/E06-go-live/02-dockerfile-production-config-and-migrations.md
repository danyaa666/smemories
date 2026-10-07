# T-022 — Dockerfile, production config and migrations as a one-off task

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Multi-stage Dockerfile (distroless or alpine, non-root), production config validation, graceful shutdown, migrate-as-task entrypoint, image scan in CI.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-009 review (2026-10-08)
Image processing is memory hungry (see T-036): size the container for it (2 GiB minimum unless T-036 documents otherwise), set `SMEM_MEMORY_LIMIT_MIB` to about 80% of the container memory,
and add the `SMEM_S3_*` and `SMEM_MEDIA_*` variables to the production configuration; storage credentials come from the task role, not static keys (the S3 client currently supports static keys only: extend it to the default credential chain when SMEM_S3_ACCESS_KEY is empty).
