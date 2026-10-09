# E01_T-029 — Replace the frozen MinIO dev image with a maintained S3-compatible store

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
MinIO no longer publishes container images (Docker Hub and Quay pulls fail), so the local stack and CI (T-009) use the frozen bitnamilegacy/minio:2025.4.22-debian-12-r2, which receives no security patches. It is dev/CI-only, bound to loopback, with no real data, but should not stay forever. Evaluate maintained S3-compatible stores (for example SeaweedFS or Garage, or building MinIO from source) against what the app needs: the S3 API through aws-sdk-go-v2 with path-style addressing, bucket auto-creation, and a console or CLI to inspect objects. Swap the compose service and CI service container; no application code should change.

_BACKLOG: needs a full spec before it moves to TODO. Do it before M2 go-live, or earlier if the frozen image causes trouble._
