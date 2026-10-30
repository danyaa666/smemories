# E09_T-146 — Class book applies each student's design

**Epic:** E09-customisation-editor (touches E07) · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-145. **Read first:** docs/class-permissions.md and the class privacy review (what an admin may read), `internal/class` (`class-book/get`, `Encode`, the 8 MiB cap), `web/src/book/classBook.ts` and `ClassBookPages.tsx`, the T-141 resolved-design shape, T-143's section/text/image handling.

#### Requirements (SHALL)
1. **API.** `class-book/get` returns, per member, the member's **resolved design** (same shape as in `book-data/get`, T-141) next to the profile and approved notes. A design holds no personal data beyond what the student typed into the text slots (titles), which the student chose to put in their book; document this in docs/class-permissions.md and in the privacy review (section 1: add "design choices and fixed-text edits"). The read goes through the existing admin check; a student/non-member still gets 403/404; an unlinked member has none. The byte cap counts the design.
2. **Web.** The class print route applies each student's design to that student's pages only (theme, fonts, section visibility and order, texts, images); the class cover and class pages keep the class look (no design). The paper size stays one per class book. Fonts of every font set in use load before ready (only the sets that are used).
3. print-check: a class fixture of 6 students with mixed themes/font sets/hidden sections on A5 and A4; page counts, order and no blank pages asserted; photos eager and at most 16 in flight; the existing 30-student fixture stays green.
4. Matrix and privacy: add a cell proving `class-book/get` returns the design to the admin only; extend `docs/class-privacy-review.md` and the endpoint list; `scripts/check-class-matrix.sh` stays green.

#### Acceptance criteria
- [ ] AC1 — The admin's class book shows each student's chosen look; a student without a design prints the template default.
- [ ] AC2 — Students and strangers still cannot call `class-book/get`; no personal field other than design texts is added.
- [ ] AC3 — The cap and `truncated` behaviour are unchanged and tested with designs present.
- [ ] AC4 — Lint, vet incl. `-tags integration`, `go test -race ./...`, web checks, print-check; D-37: manual full CI with run id.

#### Test plan
QA prints a 6-student class with different designs to PDF in Chrome and compares each student's pages with their own customise preview.
