# E09_T-194 — Stage 2 tests: A4 editor-stage parity flake, and the drag-fps test on slow machines

**Epic:** E09-customisation-editor · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** test
**Depends on:** T-183 (merged first). Findings from QA on T-183 (2026-11-19).

#### Description
1. Print-check "A4 editor stage vs book screen" failed once with the cover 30.2 % different and passed on rerun (0.130 %): a flake in an older test.
2. The drag-fps budget test asserts >= 30 fps at 4x throttle, so `make e2e` always fails on the 2012 i7 Windows PC (16.6 to 18.2 fps; 59.7 on the T-160
   machine). The budget itself (S2-02) stays as it is.
3. editor-text printed-view differs 0.62 to 0.68 % against a 0.6 % limit on Windows only.

#### Requirements (SHALL)
1. Find why the A4 editor-stage picture is sometimes 30 % different (timing, image load, font) and fix the test's wait, not the threshold; 10 container
   runs of that test pass.
2. The drag-fps test SHALL keep asserting the budget in CI (Linux runner) and report, not fail, on a machine below a stated CPU baseline, with the measured fps
   printed; document the rule in docs/e2e.md. No other budget changes.
3. Explain the Windows 0.62 % editor-text difference (fonts, hinting) with evidence; Linux CI limits stay 0.6 %; a Windows-only allowance needs numbers from
   three runs.

#### Test plan
- 10x container runs of the flaky test; make e2e on the Windows PC; manual CI run.
