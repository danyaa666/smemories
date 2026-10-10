# E02_T-076 — Code screens polish (accessibility and small UX gaps)

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-049 (merged).

#### Description
QA of T-049 found five small issues in the email code screens. None blocks use; all are cheap and make the screens meet WCAG 2.1 AA.

#### Requirements (SHALL)
1. The "send a new code" button SHALL have a touch target of at least 24x24 CSS px at 375 px (WCAG 2.5.8), 44 px preferred like the other buttons.
2. The 60-second resend cooldown SHALL survive a page reload (keep the "next allowed at" time in `sessionStorage`, never the code or email; expired values are ignored).
3. A code with fewer than 6 digits SHALL show a "enter all 6 digits" message instead of "That code is wrong" (no request is sent).
4. After a `weak_password` error on the reset screen focus SHALL move to the password field (not stay on the submit button).
5. A leftover `?token=` in the address bar SHALL be removed with `history.replaceState` on load of `/verify-email` and `/reset-password` (nothing reads it; this only keeps old links out of history and referrers).

#### Acceptance criteria
- [ ] AC1 — One vitest case per requirement; EN and VI strings for requirement 3; key parity passes; `make lint build test` green.
- [ ] AC2 — 375 px check with a long email: no horizontal overflow.
