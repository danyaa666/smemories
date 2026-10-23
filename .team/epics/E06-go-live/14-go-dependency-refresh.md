# E06_T-122 — One-time Go dependency refresh (Dependabot never ran)

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P2 · **Type:** deps
**Depends on:** none. **Read first:** `go.mod`, docs/go-conventions.md, the T-113 spec (WebP decode memory measurements depend on `golang.org/x/image`).

#### Why
The 2026-10-23 health sweep found that Dependabot has never opened a PR. It reads `.github/dependabot.yml` from the default branch (`main`, last updated 2026-10-06), and `main` does not have that file yet. Until the owner promotes develop to main, nothing updates automatically. Direct Go modules behind on that date:
- the aws-sdk-go-v2 family (patches);
- `pressly/goose/v3` 3.28.0 → 3.29.0;
- `prometheus/client_golang` 1.23.2 → 1.25.0;
- `golang.org/x/crypto` 0.57 → 0.58, `x/image` 0.46 → 0.47, `x/text` 0.42 → 0.43.

#### Requirements (SHALL)
1. Update the direct Go modules to their latest patch or minor versions (`go get -u=patch` first, then minors one family at a time), then run `go mod tidy`. No major version bumps.
2. Read the release notes of goose, client_golang and x/image. Name in the PR anything that changes behaviour this codebase relies on: migration locking (T-108), metric names, WebP decoding.
3. Re-run T-113's measurement test (`TestDecodedBytesBoundsALossyWebPWithAlpha`) and the media integration tests after the x/image bump. The memory bound must still hold.
4. Install and run `govulncheck ./...` (pinned version) and paste the result. Add it to the CI security job if it is not there yet.
5. No application code changes, unless a release note forces one; then stop and report.

#### Acceptance criteria
- AC1: `go list -m -u` shows no outstanding patch or minor updates for direct modules.
- AC2: `make lint test test-integration image-smoke` and all CI jobs are green.
- AC3: govulncheck reports no reachable vulnerabilities and runs in CI.
