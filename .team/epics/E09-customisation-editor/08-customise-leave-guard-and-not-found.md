# E09_T-148 — Customise screen: guard browser Back, ask before a template change, fast NotFound

**Epic:** E09-customisation-editor · **Milestone:** M1b · **Risk:** low · **Priority:** P3 · **Type:** feature
**Depends on:** T-144 (merged). **Read first:** `web/src/components/useLeaveGuard.ts`, `web/src/pages/Customise.tsx`, `web/src/pages/YearbookEdit.tsx`, `web/src/App.tsx`, the T-144 spec (`04-web-customise-screen.md`, AC2) and the T-144 QA note on the board.

#### Why
T-144 QA passed the customise screen with three known gaps (all cosmetic data, none a blocker):
1. AC2 "leaving with unsaved changes asks first" holds for tab close, reload and in-app links, but **browser Back and the phone Back gesture silently drop the edits** (the app uses `BrowserRouter`, so `useBlocker` is not available).
2. Picking another template on the yearbook screen with unsaved design edits drops them without asking (the banner afterwards says the design was reset).
3. A non-owner sees "Loading the design..." for 8-10 s before NotFound, because the app shell retries the 404 of `book-design/get` three times.

#### Requirements (SHALL)
1. **Back.** Browser Back / phone Back with unsaved edits asks first, with the same dialog and wording as the in-app link guard; "Stay" keeps the edits and the URL; "Leave" goes back. Prefer the least invasive fix that works with the current router (for example a `popstate` handler that pushes the current URL back and shows the dialog); moving the app to a data router (`createBrowserRouter`) is allowed only if it is a small, fully tested change and does not alter any other route. Say in the PR which you chose and why.
2. **Template change.** Changing the template while the design has unsaved edits asks first and names what is lost; cancel keeps the template and the edits.
3. **NotFound speed.** A 404 (and a 403) from `book-design/get` must not be retried: a non-owner reaches the not-found screen in under 1.5 s on a normal connection. Keep the retry for network errors and 5xx. No change to any other request's retry policy.
4. vitest for each: Back with and without unsaved edits (stay and leave), template change (confirm and cancel), no retry on 404/403 and retry on 5xx; both locales, parity; no new dependency; no API change.

#### Acceptance criteria
- [ ] AC1 — With unsaved edits, browser Back and the phone Back gesture ask first; Stay keeps edits and URL; Leave leaves. Without edits, Back leaves at once.
- [ ] AC2 — A template change with unsaved edits asks first; cancel keeps both.
- [ ] AC3 — A non-owner reaches NotFound in under 1.5 s (measured in a browser, quote the time); other requests keep their retry policy.
- [ ] AC4 — Lint, typecheck, vitest, i18n parity, check:api, build; print-check unchanged.

#### Test plan
QA reproduces the Back case in Chromium (desktop and the 375 px viewport), checks the dialog in both locales, times the non-owner NotFound, and runs the web checks plus `make print-check`.
