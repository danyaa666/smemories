# E01_T-186 — Print check on Linux: six screen-vs-print parity failures

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** bug
**Depends on:** T-185 (PR #182: Chromium install, step limits, journey race; must be merged first).

#### Description
The layout parity tests of print-check (T-154) had only ever run on macOS, where they passed 298/298 (QA, 2026-11-06, develop cc3d8c6).
On the Linux CI runner (manual runs 44334658321, 44340594952 on the T-185 branch) print-check gives **292/298**. All six failures compare the
book on screen with the printed PDF:
- A5 `note01`: the note text is 3 px lower on screen than in print (y0 1516 vs 1513 at 192 dpi).
- A4 notes doodle, stickers and tape: 2.16 % pixel difference against a 0.55 % baseline.
- A4 profile, tilted.
- Letter `thin01`.
`--font-render-hinting=none` made it worse (10 failures) and was reverted. T-181 is not the cause (it only touches editor files).
Print-check cannot run natively on the Windows PC (spawn npx ENOENT, Windows paths, pdftotext differences); Docker Desktop is up there.

#### Requirements (SHALL)
1. Reproduce the six failures locally in a Linux container matching the CI runner (ubuntu-24.04, same Chromium/Playwright and fonts as
   `ci.yml`), so each iteration takes minutes, not a 20-minute CI run. Write the exact command in `docs/e2e.md`.
2. For each failure, decide with evidence: a **real screen/print mismatch** (fix the product CSS or print path; it would show in a real
   student's PDF) or **renderer noise on Linux** (then set a per-platform baseline or tolerance, with the measured numbers from at least
   three runs in the PR). Do not loosen a tolerance without those numbers.
3. A manual CI run on the PR branch passes print-check 298/298 (run id in the PR).
4. Do not make print-check run natively on Windows in this task (separate decision).

#### Test plan
- Local Linux container run before and after (counts in the PR).
- Manual CI run `gh workflow run ci.yml --ref <branch>`: e2e job green including print-check.
