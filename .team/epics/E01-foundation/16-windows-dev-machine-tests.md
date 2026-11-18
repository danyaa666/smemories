# E01_T-184 — Unit tests pass on a Windows dev machine (make test with -race)

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P1 · **Type:** tech-debt
**Depends on:** none.

#### Description
Since D-46 (2026-11-18) dev and QA run on the owner's Windows 10 PC (Go 1.27.4 with cgo via WinLibs GCC 16.2, Git Bash, Docker Desktop on WSL2).
`make test` (`go test -race ./...`) on a clean `develop` (3c7ec4c) fails in four tests there, while CI on Linux is green. Every task's
"make test green" check is therefore impossible on this machine until they are fixed. All four are test assumptions, not product bugs:

| Package | Test | Failure on Windows | Likely cause |
|---|---|---|---|
| `internal/booklayout` | `TestHostileDocumentsFailFast` (case "43 000 tiny items") | took 269 ms, ceiling `maxTime = 200 * time.Millisecond` | wall-clock ceiling too tight under `-race` on a slower box |
| `internal/httpx` | `TestServeStartsAndShutsDownOnCancel` | `Serve returned context deadline exceeded, want nil` after 5 s | shutdown/listener timing differs on Windows |
| `internal/notes` | `TestSweepSpool` | `os.Symlink`: "A required privilege is not held by the client" | symlinks need admin or Developer Mode on Windows |
| `internal/notes` | `TestCheckSpool` (read-only dir case) | "a read-only directory must fail" | `os.Chmod(dir, 0o500)` does not make a directory read-only on Windows |

#### Requirements (SHALL)
1. `make test` SHALL pass on the Windows dev machine and stay green on Linux CI. Do not change product code unless a test shows a real Windows bug in it (then say so in the PR).
2. `booklayout`: keep the test's purpose (hostile input fails fast and bounded). Prefer the allocation ceiling as the hard gate; scale the time ceiling for `-race`
   (a `race`/`!race` build-tagged constant, as the Go project does) or loosen it with a one-line reason. Do not delete the case.
3. `httpx`: find why `Serve` does not return nil on cancel on Windows. If it is test timing, fix the test; if `Serve` really misbehaves on Windows, fix `Serve` and keep the test.
4. `notes`: when `os.Symlink` fails with a privilege error, `t.Skip` that sub-case with the reason (do not skip on Linux). For the read-only case, skip on
   `runtime.GOOS == "windows"` with a one-line reason, or make the directory unwritable in a way Windows honours.
5. Every skip SHALL be limited to Windows and say why in its message.

#### Test plan
- On the Windows PC: `make test` green (paste the summary line and the head SHA in the PR).
- CI `go` job green on the PR (Linux, `-race`).

#### Out of scope
`make test-integration` and e2e on Windows: dev reports any Windows-only failures there as new tasks, not in this PR.
