# E04_T-074 — Sweep orphaned upload spool files at start-up

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

#### Description
T-034 spools every public submission to a `0600` temp file (`smem-upload-*`) and deletes it on success, error, disconnect and panic. QA found one gap: a graceful shutdown (or a crash or kill) in the middle of an upload leaves the file in the spool directory, and nothing removes it later. Up to 48 files of up to 32 MiB can be left per restart, and the files contain friends' private text and photos.

#### Scope
- In: `internal/notes` (spool directory handling), `cmd/smemories-api/main.go` wiring, docs/media.md note.
- Out (do not do): changing the spool design, caps or pacing of T-034.

#### Requirements (SHALL)
1. At start-up, before the listener accepts traffic, the API SHALL delete every file named `smem-upload-*` in the spool directory that is older than the longest possible upload (route timeout plus 1 minute).
2. The sweep SHALL touch only regular files with that prefix directly in the spool directory (no recursion, no symlink following) and log one INFO line with the count removed (never file names or contents).
3. A failure to list or delete SHALL be logged at WARN and SHALL NOT stop start-up.

#### Acceptance criteria
- [ ] AC1 — Unit test: a directory with an old `smem-upload-x`, a fresh `smem-upload-y`, an unrelated file, a sub-directory and a symlink named `smem-upload-z`: only `x` is removed.
- [ ] AC2 — Integration test: kill-style shutdown mid-upload (cancel the server context while a body is half sent), start a new `Handler` on the same directory with an aged file: it is removed.
- [ ] AC3 — `docs/media.md` says where the files are, the sweep rule and the disk sizing (48 x 32 MiB).

#### Test plan
- QA should probe: spool dir missing or unreadable, a file being written by a live upload at the moment of the sweep (must not be removed: age check), very many files.
