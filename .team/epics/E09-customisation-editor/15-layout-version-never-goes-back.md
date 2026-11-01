# E09_T-156 — Layout version must never go back (reset / template change)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low (touches `internal/booklayout` and possibly a migration: QA runs the full integration, class-matrix and newman lines per CLAUDE.md "CI scope") · **Priority:** P2 · **Type:** fix
**Depends on:** T-153 (merged). **Read first:** `.team/epics/E09-customisation-editor/12-layout-document-model-and-api.md`, `internal/booklayout/`, QA's T-153 comment on the board (`board.py get T-153`, observation 1).

#### Why
QA reproduced an ABA hole on T-153 (2026-11-01). `book-layout/reset` deletes the row, so `version` restarts at 1. Editor X is open at version 3. The owner resets, then saves 3 times in another tab, so the stored version is 3 again; X saves with `base_version` 3 and gets 200, silently overwriting the newer work. The editor (T-154 onward) autosaves with `base_version`, so this becomes a data-loss path as soon as it ships. The same hole exists after a template change, which also deletes the layout.

#### Requirements (SHALL)
1. The version of a yearbook's layout is **monotonic for the life of the yearbook**: reset, template change and any other delete of the layout never make a later stored version equal to a version an older editor may hold. Keep the counter on a row that is not deleted by reset/template change (a counter column on the yearbook row, or blank the layout row and bump its version instead of deleting it). Dev chooses and records the choice in the PR; the yearbook-delete path still removes everything.
2. After a reset, `book-layout/get` returns `layout: null` with a `version` greater than any version issued before; a save with an older `base_version` gets 409 with `layout: null` and the current version, as today.
3. A save made after a template change with a `base_version` from before the change gets 409 (not 200).
4. Migration (if any) goose up and down, tested with existing rows (a yearbook with a layout, one without); existing stored versions are preserved.
5. Docs: `docs/api-contract.md` states that `version` never decreases; `book-data/get` keeps returning `layout_version`.
6. Nothing under `.team/`; no web code except a regenerated API schema if the contract changes.

#### Acceptance criteria
- [ ] AC1 — Test reproducing QA's scenario (X at v3, reset, three saves elsewhere, X saves with base 3) now returns 409.
- [ ] AC2 — Same scenario with a template change instead of a reset returns 409.
- [ ] AC3 — Version sequence across save, reset, save, template change, save is strictly increasing; a fresh yearbook still starts at 1.
- [ ] AC4 — Migration up/down tested with existing rows; yearbook delete still leaves no layout data.
- [ ] AC5 — `go test -race`, `make test-integration` for touched packages, lint, `check:api`, `scripts/check-class-matrix.sh`, the book-layout newman folder pass.

#### Test plan
Dev: unit and integration tests for AC1..AC4. QA: repeats the reproduction over HTTP on a live stack and runs the full lines.
