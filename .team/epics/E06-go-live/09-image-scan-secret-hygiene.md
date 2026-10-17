# E06_T-089 — Image scan (Trivy + SBOM) and secret hygiene (T-022 slice 4)

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra
**Depends on:** T-086 (slice 2a merged); independent of T-088. **Source of truth for the text:** `02-dockerfile-production-config-and-migrations.md`, requirements 7 (scan part) and 9, ACs 6 (scan part) and 7, and `07-production-config-contract-s3-chain-migrate-lock-image-scan.md` (AC-B and the items added at its bottom).

#### In scope
Trivy scan (pinned version, DB cached) in the CI `image` job failing on fixable HIGH/CRITICAL vulnerabilities; SBOM artifact; `make image-scan`; the image secret-hygiene check (final filesystem and `docker history` contain no `.env`, `*.pem`, `AKIA` or the dev OTP value; proven to fail on a planted `.env`); proof that the scan fails on a deliberately old base image (once, in the PR description). Plus the robustness items from the T-022 QA: pin the Dockerfile `# syntax=` frontend and the smoke test MinIO image by digest, log the `/readyz` drain WARN once, document that an upload longer than the drain window is cut on deploy, and make the smoke test's `migrate status` check capture its output and retry before grepping.

#### Acceptance criteria
- [ ] AC1 - CI `image` job: Trivy fails on a fixable HIGH/CRITICAL finding and passes on the real image; the SBOM is uploaded as an artifact; `make image-scan` works locally.
- [ ] AC2 - The secret-hygiene check passes on the real image and fails on a planted `.env`.
- [ ] AC3 - The scan failing on an old base image is demonstrated once (link or transcript in the PR).
- [ ] AC4 - The listed pins, the once-only drain WARN, the drain/upload note and the smoke-test robustness change are done, each with a test or a visible check.
- [ ] AC5 - Everything pinned by version/digest; no secrets in logs or artifacts.

#### Added from the T-086 slice (a) QA (non-blocking, do here)
- N1: the opt-out WARN (SMEM_DB_ALLOW_INSECURE and the other escape hatches) is logged by the API only; `smemories-migrate` and `smemories-media-backfill` honour them silently. Log it loudly in all three binaries at start-up.
- N2: the errors for SMEM_PUBLIC_BASE_URL and SMEM_ALLOWED_ORIGINS echo the offending URL even when it contains `user:pass@`. Never echo a URL userinfo (redact to `***`), and refuse a base URL or origin that carries userinfo.
- N3: prod accepts an `http://` SMEM_S3_ENDPOINT, and a static SMEM_S3_ACCESS_KEY silently overrides the task role. In prod: require https for a custom S3 endpoint (AWS S3 needs none) and log a WARN naming the override when static keys are set (or refuse them in prod unless an escape hatch is set; document the choice).
