# E06_T-118 — Re-pin the Go build image to 1.26.10 (CVE-2026-111943)

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P1 · **Type:** security
**Depends on:** none. **Read first:** `Dockerfile` (header comments on pinning), `.github/dependabot.yml` (docker ecosystem), the T-089 spec `09-image-scan-secret-hygiene.md`.

#### Why
The CI `image` job is red on develop (since at least c244f77) and on every PR. Trivy finds Go stdlib v1.26.9 in the API binary: CVE-2026-111943, HIGH, crypto/x509, fixed in 1.26.10. The Dockerfile pins `golang:1.26-bookworm` by digest to the 1.26.9 image, and Dependabot's weekly docker run has not proposed a new digest yet. The CI Go jobs already use the newest patch (`setup-go` with `check-latest` and `go-newest-patch`), so only the image is behind.

#### Requirements (SHALL)
1. Re-pin the build stage `FROM golang:1.26-bookworm@sha256:...` to the current multi-arch index digest of an image that ships Go **1.26.10 or newer 1.26.x**. Keep the digest pin and `--platform=$BUILDPLATFORM`, and update the "(1.26.9 when pinned)" comment.
2. Check the runtime stage's base too. If Trivy reports a fixed HIGH or CRITICAL in it, re-pin it the same way. Otherwise leave it.
3. Do not change the minor version (stay on 1.26), go.mod, the CI workflow or the Trivy policy. No `.trivyignore` entry.
4. Say in the PR how you got the digest: `docker buildx imagetools inspect golang:1.26-bookworm`, plus a check of `go version` inside that image.

#### Acceptance criteria
- AC1: `make image` and `make image-smoke` pass locally, and `docker run --rm --entrypoint "" <build image> go version` (or the equivalent) prints go1.26.10 or newer.
- AC2: The PR's CI `image` job (Trivy) is green, and the other jobs stay green.
- AC3: The diff covers only the Dockerfile (and docs, if a doc names the Go patch version).

#### Test plan
Run `make image-smoke` locally and let the CI `image` job check it as a regression test. If a scan fails later, Dependabot's weekly docker update keeps the pin moving.
