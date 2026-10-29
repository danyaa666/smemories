# E06_T-023 — AWS infrastructure as code and deploy pipeline

> **From the security audit (2026-10-20, [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md)), checklist for this task:**
> 1. **Edge headers on the web origin:** `Content-Security-Policy` (`default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'; base-uri 'none'`; fonts are self-hosted, L-12), `Strict-Transport-Security`, `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`. Test the CSP against the real build and the print screen.
> 2. **Log hygiene (L5):** the public link token is in the query string of `GET /api/public-collect/get` and `POST /api/public-collect/submit-note`. ALB access logs and CloudFront standard logs would record it: leave them off, or drop the query string, and say which in `docs/deploy.md`.
> 3. **Client address behind CloudFront:** `SMEM_TRUST_PROXY` takes the last `X-Forwarded-For` hop. With CloudFront in front of the ALB that hop is the CloudFront edge address, not the visitor, and every per-client limit then keys on edge addresses. Decide the topology (ALB only, or CloudFront with the viewer address passed in a header) and make `ResolveClientIP` match it; T-098 masks IPv6 to /64 after that choice.
> 4. **`/readyz` and `/healthz`** reachable by the ALB health check only, not through the public path (T-101 caches the result, this removes the exposure).
> 5. **Database users:** one for `smemories-migrate` (DDL), one for the API (DML only on the app tables), no `smem_test_%` grant in production.
> 6. **Repo settings (owner questions on the board):** Dependabot alerts and security updates on; agent identity and `enforce_admins`.

> **From audit run 1 (2026-10-20, `docs/security-audit-skill-2026-10-20/`, PR #86), also for this task:**
> 3. **Log hygiene, more paths:** the link token is also in the SPA path `/c/<token>` and the photo-link token in `GET /api/public-media/get-content?token=` (T-092). Edge access logs must not record the path or query of `/c/*`, `/api/public-collect/*` and `/api/public-media/*` (or those logs stay off).
> 4. **Upload spool sizing (lead 8):** the API can hold `SMEM_PUBLIC_UPLOAD_MAX_CONNS` x 32 MiB of spooled bodies (1.5 GiB by default) in `SMEM_UPLOAD_TMP_DIR`. The task definition gives that directory an ephemeral volume of at least that size plus 25 %, or the config lowers the connection cap; state which in the plan, and alarm on spool write errors.
> 5. **Shared Redis (lead 9):** limiter keys share the ElastiCache node with sessions and codes under `noeviction`. Size the node for the worst case after /64 keying (T-098) and add a memory alarm at 70 %.
> 6. **`/readyz`** reachable only from the ALB health check, not through CloudFront (T-101 caches it, this keeps it private).

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Fargate service, RDS MySQL (private subnets, encrypted, backups), S3 (private, encrypted, lifecycle), CloudFront with /api origin and prefix strip, ACM cert, secrets in Secrets Manager, deploy workflow with OIDC to AWS (no long-lived keys). Blocked on owner decision Q-001 (IaC tool) and the owner's AWS account.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader note from decision D-22 (2026-10-08)
The deploy pipeline must run `scripts/check-no-dev-shortcuts.sh` (T-050) before it builds or pushes an image, and must refuse to deploy when it fails. T-050 is a prerequisite of this task.

#### Leader note from decision D-23 (2026-10-08)
Provision ElastiCache (Valkey or Redis OSS, decide by price) in the private subnets: encryption in transit and at rest, an auth token in Secrets Manager, Multi-AZ with automatic failover once there is real traffic, `maxmemory-policy noeviction`, a parameter group in code. **Bring the monthly cost estimate (node size, one versus two nodes) to the owner before provisioning** (E06 PRD: cost agreed first). A single small node is acceptable for the first users; losing it only logs everybody out.

#### Leader note from T-125 (2026-10-26): invite links are secrets in the path
The class invite link is `/join/<token>`; the token is a bearer secret. The CDN, load balancer and web-server access logs must not record the path of `/join/*` (log it as `/join/[redacted]` or drop the path), and no analytics or error-reporting tool may capture it. `docs/class-permissions.md` states the rule; `tools/e2e/scrub-trace.sh` already scrubs it from traces. Add a check to the deploy verification (a request to `/join/TESTTOKEN` must not appear verbatim in any log group after the smoke test).

#### Leader note from T-131 (2026-10-29): class yearbook go-live check
Add to the go-live review list: "Class yearbook: the owner has signed `docs/class-privacy-review.md` section 8, the legal texts (version 2026-10-29) are finalised, `scripts/check-class-matrix.sh` is green, and no invite or class-book path appears in the edge logs."

#### Leader note from D-38 (2026-10-29)
The `image` CI job (build, smoke test, Trivy scan, SBOM) is paused on pushes and pull requests. The deploy pipeline must therefore run `make image-smoke` and `make image-scan` (or the same steps) on the exact image it is about to deploy, fail on Trivy HIGH/CRITICAL with a fix available, and keep the SBOM as a build artifact.
