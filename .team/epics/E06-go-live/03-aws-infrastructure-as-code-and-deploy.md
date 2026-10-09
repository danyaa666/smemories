# E06_T-023 — AWS infrastructure as code and deploy pipeline

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Fargate service, RDS MySQL (private subnets, encrypted, backups), S3 (private, encrypted, lifecycle), CloudFront with /api origin and prefix strip, ACM cert, secrets in Secrets Manager, deploy workflow with OIDC to AWS (no long-lived keys). Blocked on owner decision Q-001 (IaC tool) and the owner's AWS account.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader note from decision D-22 (2026-10-08)
The deploy pipeline must run `scripts/check-no-dev-shortcuts.sh` (T-050) before it builds or pushes an image, and must refuse to deploy when it fails. T-050 is a prerequisite of this task.

#### Leader note from decision D-23 (2026-10-08)
Provision ElastiCache (Valkey or Redis OSS, decide by price) in the private subnets: encryption in transit and at rest, an auth token in Secrets Manager, Multi-AZ with automatic failover once there is real traffic, `maxmemory-policy noeviction`, a parameter group in code. **Bring the monthly cost estimate (node size, one versus two nodes) to the owner before provisioning** (E06 PRD: cost agreed first). A single small node is acceptable for the first users; losing it only logs everybody out.
