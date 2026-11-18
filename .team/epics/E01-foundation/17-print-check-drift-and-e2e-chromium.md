# E01_T-185 — Print check: notes drift 3 px on A5, the step hangs to its timeout, and the develop e2e job cannot install Chromium

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** bug
**Depends on:** none.

#### Description
Print check is paused in CI since D-37, so nobody saw it drift. The first manual full CI run in weeks (run 44293267820, T-181 branch,
2026-11-18) failed it: `A5 notes item note01: y0 1516 on screen against 1513 in print (192 dpi)`, and then the step ran until the
15-minute job timeout instead of ending. T-181 cannot cause it (its CSS is all under `.ed`, nothing outside the editor imports its files),
so the drift is on `develop`. Two manual runs of the full CI on `develop` (44298102605, attempts 1 and 2) failed earlier, at
"Install Chromium" (`playwright install --with-deps chromium failed or timed out`), so `develop` has no baseline yet.

#### Requirements (SHALL)
1. Get a green manual CI run (`gh workflow run ci.yml --ref <branch>`) of the e2e job on `develop` head; if "Install Chromium" keeps
   failing, find out why (cache key, mirror, apt step) and fix it.
2. Find which change moved the A5 notes item by 3 px between screen and print and fix the cause (a real screen/print mismatch is a
   product bug). Do not loosen the tolerance unless you show the 3 px is rendering noise, with numbers from more than one run.
3. Print check SHALL end with a clear failure instead of hanging to the 15-minute timeout (find what it waits on after an assertion).
4. Report the run ids and results in the PR.

#### Test plan
- Manual CI run on the PR branch: e2e job green, print check included.
- Locally on the Windows PC if it works there (poppler and zip are installed since 2026-11-18); say if it does not.
