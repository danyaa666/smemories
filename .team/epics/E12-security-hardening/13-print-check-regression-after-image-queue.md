# E12_T-121 — Fix the print-check regression from the image queue (T-116) and run print-check in CI

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** low · **Priority:** P1 · **Type:** bug
**Depends on:** none. **Read first:** the T-116 diff (PR #113: `web/src/components/imageQueue.ts`, `QueuedImg.tsx`, `web/src/book/env.tsx`, `ready.ts`, `usePhotoPreload`), `tools/print-check/` (how it decides a page is ready and how it checks photos), the T-097 QA note on the board.

#### Why
QA on T-097 found that `make print-check` fails 4 of 55 on develop: the navy-classic photo tests. They pass at 5258ccb (just before T-116 merged) and fail from fa4a97c on. print-check is not in CI, so nobody saw it. If the cause is in the app (the print book's ready signal firing before queued photos have loaded, or IntersectionObserver never firing for off-screen print pages), **real printed books can miss photos**. That makes this P1.

#### Requirements (SHALL)
1. **Root cause first.** Bisect to the exact change and say in the PR whether printed output in a real browser was affected or only the harness. Print to PDF a navy-classic book with photos on pages never scrolled into view, before and after the fix, and compare photo counts.
2. **Fix it where all print paths route.** In print mode (the print route, `beforeprint`, and print-check's renderer), every photo must load without waiting for intersection. The ready signal must not fire until every photo has settled (loaded or failed after its retry). Keep the 16-in-flight cap: queue the loads, never skip them. The owner-gallery behaviour from T-116 stays.
3. **print-check in CI.** Add `make print-check` (or its npm script) to the CI e2e job, or a new job reusing the Playwright cache from T-120, so a print regression fails a PR. Keep the added CI time under 3 minutes; if the full run is slower, run the fast subset and say which.
4. A regression test: one that fails on fa4a97c and passes after the fix, either a vitest case for the ready logic or the print-check case itself in CI.

#### Acceptance criteria
- AC1: `make print-check` passes 55/55 on the PR head. CI runs it and it is green.
- AC2: On the PR head, a PDF of a 40-note navy-classic book with photos has every photo. Show the photo count against the number of notes with photos.
- AC3: The T-116 checks still hold: at most 16 in flight on a 100-photo gallery, and print waits for all photos.
