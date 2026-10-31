# E09_T-152 — Backend: student-uploaded design images (backgrounds and decorations)

**Epic:** E09-customisation-editor (stage 2) · **Milestone:** M4 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** E09 PRD "Stage 2 plan", `docs/media.md` (routes, quotas, memory budget), `internal/media/`, `docs/class-permissions.md`, `docs/class-privacy-review.md`, `docs/api-contract.md`, `docs/db-conventions.md`, owner decisions D-36 and D-43(c).

#### Why
Students may upload their own background and decoration images for the layout editor (D-43). They go through the same photo pipeline as yearbook photos (decode, strip metadata, re-encode, limits), but they are not photos of friends and must not appear in the photo gallery or the print of photos. They are also new personal data a class admin may read in a class book, so the privacy review and the class matrix must cover them. High risk: migration, public API, privacy, quotas.

#### Requirements (SHALL)
1. **Purpose.** Media rows get a `purpose` (`photo`, default for all existing rows, or `design`); migration is additive with a default, applied by `make migrate`, and reversible per the migration conventions. `POST /api/media/create` accepts an optional `purpose` field in the multipart form (`design`); anything else is `ERROR_INVALID_PARAM`. Photos behave exactly as before.
2. **Isolation.** `media/get-list` returns `photo` rows by default and only `design` rows when `purpose=design` is passed; gallery, photo counts, the profile/book photo pickers and the collection (public) routes never show or accept a `design` image; a `design` image id cannot be used as a profile or note photo, and vice versa (tests for both directions).
3. **Limits.** Same file types and size limits as photos. Design images have their own cap of 30 per yearbook; bytes count in the owner's 500 MiB per-user quota together with photos (`ERROR_QUOTA_EXCEEDED` 409 for either). The per-user and per-client upload concurrency limits and the memory budget apply unchanged. Variants: `display` (longest side 2000 px) and `thumb`; no `print` variant is needed unless the layout needs it: decide from `docs/media.md` and say which in the PR (a full-bleed background printed at 300 dpi on A4 needs about 2480 x 3508 px: if `display` is smaller, add the print variant and say why).
4. **Delete.** The owner can delete a design image; deleting a yearbook deletes its design images (same path as photos). A layout that references a missing image keeps working: the reader ignores the missing id (that is the layout task's job; here, only the deletion and a test that deletion succeeds while a reference exists).
5. **Class book.** In a class book (D-36), a class admin can read the design images of members' books through the existing admin read path (`media.Store.readable`), and only for members whose book is in the class; a non-admin member, another class's admin and an anonymous caller get 404 (extend `TestClassBookMatrix` or the class matrix script with the new operation, and `scripts/check-class-matrix.sh`). `docs/class-privacy-review.md` lists the new data and the new read path (section 1 and the endpoint list; the section 8 sign-off stays the owner's).
6. **Privacy and data.** Update `docs/media.md`, `docs/api-contract.md` (the field and the filter), `docs/class-permissions.md`, and the data export and deletion docs/tests (a deleted account deletes its design images; the export lists them). `api/openapi.yaml` and the generated web schema updated; `check:api` clean.
7. **Observability.** The existing media metrics get a `purpose` label (low cardinality: two values) on upload and refusal counters; no image content or file names in logs.
8. Run `make test-integration` for the touched packages and the class matrix script; the e2e and print-check are unchanged (report).

#### Acceptance criteria
- [ ] AC1 — A `design` image uploads, lists only with `purpose=design`, serves in `display` and `thumb`, and never shows in the photo gallery or counts toward the 200-photo cap; its bytes count toward the 500 MiB user quota; the 31st design image of a yearbook gets 409.
- [ ] AC2 — Cross-use is refused in both directions (design id as a photo, photo id as a design image).
- [ ] AC3 — The class matrix shows admin 200 and every other caller 404 for a member's design image; the privacy review lists the new data.
- [ ] AC4 — Account and yearbook deletion remove the design images; the export lists them.
- [ ] AC5 — The migration applies and rolls back on a database with existing photos; existing photo behaviour and tests are unchanged.
- [ ] AC6 — `go test ./...`, `make test-integration` (touched packages), lint, `check:api`, `scripts/check-class-matrix.sh` pass.

#### Test plan
QA uploads valid, oversized, wrong-type and truncated design images, exercises the quota and the 30 cap, walks the class matrix as admin, member, other-class admin and anonymous, checks deletion and export, and runs the migration up and down on a database with photos. High risk: the owner approves before merge.
