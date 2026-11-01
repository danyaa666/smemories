# E09_T-155 — Fix three stale Postman assertions in the customisation-editor collection

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** test
**Depends on:** none. **Read first:** `postman/customisation-editor.postman_collection.json` (old book-design folder, steps 17 and 18), `docs/api-contract.md`.

#### Why
QA found on T-152 (2026-10-31) that three assertions in the old book-design folder still assume `navy-classic` is not customisable. That stopped being true in T-145. They fail the same way on `develop`, so the whole-file newman run is red for a reason unrelated to any open task.

#### Requirements (SHALL)
1. Update steps 17 and 18 (and the third failing assertion) so they match today's behaviour: `navy-classic` accepts a design; use a template or case that is still really rejected (an unknown theme id, an unknown template id) for the "refused" checks, so the negative coverage stays.
2. No change to production code, to other folders, or under `.team/`.

#### Acceptance criteria
- [ ] AC1 — `newman run postman/customisation-editor.postman_collection.json` (same command and fixtures as `docs/` / CI use) passes with 0 failed assertions on a fresh stack.
- [ ] AC2 — The negative checks still assert a refusal (not deleted), with the correct error code.

#### Test plan
Dev runs the whole collection twice on its own compose project; QA repeats once.
