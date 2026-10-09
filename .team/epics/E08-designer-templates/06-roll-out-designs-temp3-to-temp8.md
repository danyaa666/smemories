# E08_T-042 — Roll out designs temp3 to temp8

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** feature

#### Description
Placeholder for the rollout after the owner has reviewed the two pilots (D-15). When T-040 and T-041 are done, the leader replaces this task with one task per design (retro scrapbook temp3, magazine temp4, groovy temp5, movie poster temp6, botanical temp7, passport temp8), each with its page mapping, font choices and lessons from the pilots, and cancels this one.

#### Scope
- In: planning only: a mapping table per design, the font lookalike list with Vietnamese coverage, a list of renderer gaps found in the pilots (for example more rotation, polygon shapes, vector decoration).
- Out (do not do): any implementation under this task id.

#### Acceptance criteria
- [ ] AC1 — The leader writes six task specs in this epic folder and updates the PRD task index; nothing else.

#### Design
Per design the canvas CSS gives its fonts; temp4 is the only one whose fonts all carry a Vietnamese subset in the canvas. Known gaps to check: temp3 uses 59 rotated elements and 43 shadows, temp7 38 rotations and 18 SVG illustrations, temp8 stamps and a ticket layout.

#### Risk
`low`.

#### Security & performance notes
None.

#### Test plan
None (planning task).
