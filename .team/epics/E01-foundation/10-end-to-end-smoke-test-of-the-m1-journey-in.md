# E01_T-020 — End-to-end smoke test of the M1 journey in CI (Playwright)

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** infra
**Depends on:** T-017, T-018, T-061 (T-019, the old PDF export screen, was cancelled; the print path is T-058..T-061). **Read first:** `.github/workflows/ci.yml` (how CI starts MySQL, Valkey and MinIO today), `Makefile`, `docs/dev-shortcuts.md`, `tools/print-check/` (the PDF checks to reuse), `web/vite.config.ts`.

#### Description
One Playwright test drives the whole M1 journey against the real API, real MySQL/Valkey/MinIO and the built web app in CI, so a regression anywhere in the chain (auth, yearbook, media, link, public form, moderation, book data, renderer, PDF) turns CI red before the owner finds it by hand. This is the automated half of the M1 exit criteria; the manual half is `docs/spikes/print/OWNER-TESTS.md` (real Safari, phones, the print dialog).

#### The journey (one test, in this order, Chromium only)
1. Register a new user by email, verify with the fixed development code (the registered dev shortcut `SMEM_DEV_FIXED_OTP`; the API in this job runs with `SMEM_ENV=test`, which the shortcut guard allows; never in any other environment), land on the yearbooks page; the username chip shows `user-xxxxxxxx`.
2. Create a yearbook (Vietnamese title with diacritics, language `vi`, page size A5), fill the profile (long-ish quote), upload a cover photo.
3. Choose the `memory-book` template in the picker (T-061) and see the preview change.
4. Create a note link (T-017); read the shown link once (`/c/<token>`).
5. In a SECOND, anonymous browser context open that link, fill the form (Vietnamese name and message with diacritics and an emoji), add one photo, send; the thank-you screen appears.
6. Back in the owner context: the inbox shows 1 pending note and the sidebar badge; approve it; the Approved tab shows it.
7. Submit two more notes from the anonymous context (so the book has a page of notes), approve them, reorder two of them; the order persists after reload.
8. Open `/yearbooks/:id/book`, wait until photos are loaded (Print enabled), call `page.pdf()` for A5: assert page count `3 + ceil(n/4)`, page size 420x595 pt, and with `pdftotext` that the title, the owner name, each note author and message text (with diacritics) are in the PDF, fonts are embedded (`pdffonts`), and no text lies outside the page.
9. Negative paths: revoke the link in the owner context, reload the anonymous page and expect the dead-end screen after exactly one lookup request; sign out and expect the protected pages to redirect to sign-in.

#### Acceptance criteria
- [ ] AC1 — `make e2e` (or the documented command) runs the whole journey locally against the compose stack and a locally started API + web build; it creates its own data (unique emails), cleans up after itself and can be re-run without manual resets (it may call the dev `reset-limits` for its own Redis keys only).
- [ ] AC2 — A new CI job `e2e` (separate from `go`, `go-integration`, `web`, `security`) starts MySQL, Valkey and MinIO the same way `go-integration` does, applies migrations, builds the API and the web app, starts them with `SMEM_ENV=test` and the dev shortcut variable, runs the Playwright test, and on failure uploads the Playwright trace, a screenshot and the API log as an artifact. Pin all actions and images by version/digest like the other jobs.
- [ ] AC3 — The test is deterministic: no fixed sleeps (wait for UI states and network idle conditions), no dependence on wall-clock timing, unique data per run, a hard per-step timeout and a total budget under 5 minutes in CI. It ran green 10 times in a row on the PR (list the run links in the PR description) before the job is added to branch protection.
- [ ] AC4 — Failure messages say which step broke (use `test.step`); the PDF assertions reuse `tools/print-check` helpers where sensible instead of duplicating them.
- [ ] AC5 — No dev shortcut leaks: the e2e job is the only CI job that sets `SMEM_DEV_FIXED_OTP`; `scripts/check-no-dev-shortcuts.sh` (T-050) later keeps production clean; `docs/dev-shortcuts.md` is updated to mention this use.
- [ ] AC6 — Docs: README/`docs/` get a short "End-to-end test" section (how to run locally, how to read a CI failure, how to update the journey when a screen changes).

#### Out of scope
Firefox/Safari/mobile browsers, visual regression of pages, load testing, the Google sign-in flow (needs a provider), real email delivery.

#### Risk
`low`: tests and CI only; the one sensitive point is the dev shortcut, which stays inside the existing guard (SMEM_ENV=test).

#### Test plan
- Dev: run the journey 10 times locally and in CI; mutate the product (e.g. break the approve endpoint, drop a note from book-data, change the PDF page size) and show the test fails at the right step; say what you could not check.
- QA should probe: flakiness (run under CPU load), leftover data, secrets in logs/artifacts (the link token appears in the test: make sure it is not printed to CI logs), that no other job sets the shortcut variable, that the artifact upload works on a forced failure.
