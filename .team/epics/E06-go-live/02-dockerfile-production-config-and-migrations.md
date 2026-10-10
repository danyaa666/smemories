# E06_T-022 — Dockerfile, production config and migrations as a one-off task

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Multi-stage Dockerfile (distroless or alpine, non-root), production config validation, graceful shutdown, migrate-as-task entrypoint, image scan in CI.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-009 review (2026-10-08)
Image processing is memory hungry (see T-036): size the container for it (2 GiB minimum unless T-036 documents otherwise), set `SMEM_MEMORY_LIMIT_MIB` to about 80% of the container memory,
and add the `SMEM_S3_*` and `SMEM_MEDIA_*` variables to the production configuration; storage credentials come from the task role, not static keys (the S3 client currently supports static keys only: extend it to the default credential chain when SMEM_S3_ACCESS_KEY is empty).

#### Leader note from decision D-23 (2026-10-08)
Production configuration also needs `SMEM_REDIS_URL` (a `rediss://` URL with the auth token from Secrets Manager), `SMEM_OTP_KEY` and `SMEM_OIDC_COOKIE_KEY`; the API must not start in prod without them. `/readyz` includes Redis, so the container health check and the load balancer target group must tolerate a Redis outage only as "not ready", not as a crash loop.

#### Leader note from T-075 (2026-10-10)
The one-off task for `smemories-media-backfill` needs only the database and object-store settings (it uses `config.LoadMigrate`); the exact variable list is in `docs/media.md` (section on the backfill). Run it with `--dry-run` first after the first deploy, then without; it is idempotent. The migrate task definition takes the same variables.
