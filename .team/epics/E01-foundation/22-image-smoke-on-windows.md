# E01_T-192 — make image-smoke fails on the Windows dev PC (curl file upload path)

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** chore
**Depends on:** none. Found by dev on T-191 (2026-11-19).

#### Description
`scripts/image-smoke.sh` uploads a photo with `curl -F file=@/tmp/...`. Under Git Bash on Windows, native curl.exe cannot read the MSYS path `/tmp/...`, so the
smoke test fails at the upload step on the Windows PC (D-46), although the image is fine. CLAUDE.md (D-38) asks dev to run `make image-smoke` for image changes.

#### Requirements (SHALL)
1. `make image-smoke` passes on the Windows PC (Git Bash) and on Linux/CI without a wrapper outside the repo; e.g. a temp file under the repo's ignored
   tmp dir or a path converted with `cygpath -w` when present. No change to what the smoke test checks.
2. A manual CI run with `image` green (run id in the PR).
