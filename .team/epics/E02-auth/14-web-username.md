# E02_T-082 — Web: show and change the username on the account page

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-081. Decision **D-28**.

#### Description
Add the username to the existing account page (`web/src/pages/Account.tsx`): the user sees their handle (the generated `user-xxxxxxxx` at first), can change it, and sees when the next change is allowed.

#### Requirements (SHALL)
1. Account page shows the current username and a "Change username" form (one text input, Save). The input is labelled, has `autocomplete="username"`-free behaviour (it is not a login field: `autocomplete="off"`), and shows the rules as help text (3-30 characters; letters, digits, dot, underscore, hyphen; lower case).
2. Client-side checks mirror the server rules (length, characters) only to give an instant hint; the server is the authority. Input is lower-cased as typed or on submit.
3. Error codes map to localised messages in `en.json` and `vi.json`: `ERROR_INVALID_USERNAME`, `ERROR_USERNAME_RESERVED`, `ERROR_USERNAME_TAKEN`, `ERROR_USERNAME_CHANGE_TOO_SOON` (shows the date from `username_next_change_at`, localised), `ERROR_RATE_LIMITED`. Messages sit under the field (`aria-describedby`, `role="alert"` on error), never only as colour.
4. While `username_next_change_at` is in the future the form is disabled with the date shown; after a successful save the page shows the new name and the new next-change date without a reload.
5. Mobile first: works at 375 px, targets at least 44 px, keyboard and screen-reader usable (WCAG 2.1 AA).

#### Acceptance criteria
- [ ] AC1 — Vitest: render with a default name; successful save updates the view; each error code shows its message; cooldown state disables the form and shows the date.
- [ ] AC2 — Both locale files have every new key (the i18n check passes).
- [ ] AC3 — `npm run typecheck lint test check:api` and `make lint build test` green; no `.team/` files in the PR.

#### Test plan
- QA should probe: very long and whitespace input, pasted upper case, double submit, losing the session mid-save (401 handling), Vietnamese locale text overflow at 375 px.
