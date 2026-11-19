# E09_T-193 — Print time of a book with a maximum layout is +37 % (budget 10 %)

**Epic:** E09-customisation-editor · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** medium · **Priority:** P2 · **Type:** perf
**Depends on:** T-183 (merged first; its budget spec and docs/stage2-exit-report.md are the baseline). Finding **S2-01** of the stage 2 exit report.

#### Description
PRD budget: the print time of a book with a layout is within 10 % of the same book without one. Measured on the Windows PC (dev and QA independently):
typical layout +8 to +9.4 % (met), **maximum layout +34 to +37 % (missed)**. Server p95 and heap are within budget.

#### Requirements (SHALL)
1. Profile the print path with the maximum layout (Go pprof on the API, Chromium trace of the print page) and name where the extra time goes, with numbers.
2. Fix the cause in the product (render path, image decoding, font loading, layout resolution...) so the maximum layout is within 10 %, or, if it cannot be
   without a design change, stop and write the options with numbers in the PR for the leader (no budget change in this task).
3. The stage2-budgets measurement shows before/after on the same machine (three runs each); print-check stays 298/298 (Linux container); no parity loss.

#### Test plan
- stage2-budgets spec before/after, pprof/trace summary in the PR, full print-check in the container, manual CI run.
