# E07_T-135 — Web: text fields count characters, and the join hint matches the build

**Epic:** E07-class-yearbook · **Milestone:** M3 · **Risk:** low · **Priority:** P3 · **Type:** bug
**Depends on:** none (T-134 merged). **Read first:** the shared `TextArea` and text inputs in `web/src/ui/`, `NoteForm`, the class page editor and class form, the T-134 QA comment on the board, `web/src/pages/Login.tsx` and `Register.tsx` (the Google hint).

#### Why
1. The server counts runes (Unicode characters) for every text limit, the visible counter now does too, but the native `maxLength` attribute counts UTF-16 units. Text with emoji stops at about half the limit (2000 of 4000) while the counter reads 2000/4000.
2. On `/login` and `/register` reached from `/join/`, the hint "Signed in with Google? Sign in first, then open your invite link again." also shows in builds without Google sign-in (`VITE_GOOGLE_SIGNIN` unset), where it mentions a button that does not exist.

#### Requirements (SHALL)
1. Remove the native `maxLength` from the shared text components that have a rune counter and enforce the limit in the change handler by rune (truncate pasted text at the limit, never split a surrogate pair). The server stays the authority. One shared helper, used by every field that shows the counter (note body, class page body and title, class form, any other).
2. Show the Google hint only when Google sign-in is enabled in the build; keep the rest of its behaviour (only from `/join/`, token never stored).
3. Tests: 4000 emoji accepted, the 4001st rune refused, a paste of 5000 emoji cut at exactly 4000, Vietnamese text unchanged; the hint present with the flag on and absent with it off. Both locales, i18n parity, no new dependency, no API change.

#### Acceptance criteria
- [ ] AC1 — An emoji-only body can be 4000 characters long and the counter agrees.
- [ ] AC2 — The hint is absent when Google sign-in is off and present when on, only from `/join/`.
- [ ] AC3 — Lint, typecheck, build, vitest, i18n parity, check:api; CI green (jobs that run).

#### Test plan
QA types and pastes emoji and Vietnamese text into each counted field and checks both build modes.
