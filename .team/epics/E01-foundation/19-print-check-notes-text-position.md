# E01_T-189 — Print check: the notes-page item check cannot see a moved note text

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** test
**Depends on:** T-186 (merged first). Found by QA on T-186 (2026-11-19).

#### Description
The per-item screen-vs-print check in `tools/print-check/check.test.mjs` takes the first ink of a note box as its edge. On notes pages that ink is the
box's ruled lines, so a note **text** shifted 5 px (A4) or even 20 px (A5) in print still passes. The blind spot predates T-186. The A4 notes-doodle
Linux budget also has only 0.39 pp headroom (profile-tilted 0.18 pp).

#### Requirements (SHALL)
1. The notes-page check SHALL measure the note text position (for example the text's own ink box inside the note, or pdftotext `-bbox` words against the
   DOM text rects), with a tolerance of at most 2 px at 192 dpi (Linux: at most the T-186 measured noise, with numbers).
2. A deliberate 5 px shift of a note text in print CSS SHALL fail on A5, A4 and Letter; prove it in the PR and revert.
3. Keep every existing check and budget; full print-check 298/298 (or more) in the Linux container and in a manual CI run (run id in the PR).

#### Test plan
- Container run before/after; deliberate-shift run (fails), reverted run (passes); manual CI run.
