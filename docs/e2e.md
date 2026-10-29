# End-to-end test of the M1 journey

One Playwright test (`tools/e2e/journey.spec.mjs`, Chromium only) drives the whole M1 journey in a real browser against the
**built web app**, the **real API** and the real MySQL, Valkey and S3 store, so a regression anywhere in the chain (auth, yearbook,
media, link, public form, moderation, book data, renderer, PDF, text export and public photo links) turns CI red. The manual half of the M1 exit criteria
(Safari, phones, the print dialog) stays in `docs/spikes/print/OWNER-TESTS.md`.

## The journey (each is a `test.step`, so a failure names its step)

1. Register (the required Terms checkbox is ticked), verify with the fixed development code (`DEV-SHORTCUT(otp)`), see the `user-xxxxxxxx` chip on the yearbooks page.
   Then (1b) get-me is made to say the accepted Terms are old: the user is sent to `/accept-terms`, accepts (real `accept-terms` call) and continues to the yearbooks.
2. Create a yearbook (Vietnamese title, language `vi`, A5), fill the profile, upload a cover photo.
3. Pick the `memory-book` template and see the preview change.
4. Create a note link and read it once.
5. In a second, anonymous browser context open the link, draw a signature on the pad with the mouse (two strokes; the receipt shows it), send a note (diacritics, emoji, one photo).
6. The inbox shows 1 pending note (with its signature) and the sidebar badge; approve it.
7. Two more notes, approve them, move one up; the order survives a reload.
8. Print screen ready, `page.pdf()`: page count (`3 + ceil(notes / notes-per-page)`; memory-book has one friend per page, so 6),
   A5 size (420 x 595 pt), fonts embedded, title, owner and every note's author and message present, notes in book order, no text
   outside its page; the preview shows the signature of the first note (`svg.bk-sig`) and the PDF has ink inside its box. The PDF helpers are the ones of `tools/print-check`.
9. Export as text (T-094): Download text file, accept the public-link warning, and the downloaded file holds the title, the owner
   and the three note names and a photo link; that link opens from a fresh context with no cookies (200, `image/*`); Disable photo
   links, confirm, and the same link is now 404. The photo token is scrubbed from every error like the collection token.
10. Revoke the link: the friend's page is a dead end after exactly one lookup request; sign out and the protected pages redirect to sign-in.

## The class journey (T-131)

`tools/e2e/class.spec.mjs` runs in the same `make e2e` (about 8 s). It registers three accounts (an admin and students A and B, each with the fixed development code) and checks the privacy promise of
`docs/class-privacy-review.md` in a browser: (1) the admin creates a class, (2) makes an invite link and reads it once, (3-4) each student opens the link, signs up on the way, joins, creates the page and fills the profile,
(5) each student makes a note link, an anonymous friend sends two notes, and the student approves one and leaves one pending, (6) the admin adds an intro page, moves B before A and saves the order, (7) the print route holds the
intro page, both students and their approved notes in that order and **no pending note**, (8) `class-book/get` as the admin has the approved notes only (no pending note, token or email) and the admin gets 404 on the students' own yearbook,
note and book-data routes, (9) as student A: none of B's notes, name or email appears on any screen A can open (the class, the admin screens, B's book address), and direct API calls return 404 for B's yearbook, notes and book data and
403 for `class-book/get`, the roster and the class pages. The markers in the notes are unique per run, so a leak is found by a text search. A deliberate break (the class book query also returning pending notes) fails step 7.

Registration is limited to 5 per address and hour and the three specs together register exactly 5 accounts (M1: 1, the gallery check: 1, the class journey: 3), so a new account in any of them needs the limit raised for tests: `make e2e` resets the limits first; after a direct `scripts/e2e.sh` run
use `make reset-limits SMEM_ENV=test`. `class-book/get` is limited to 30 reads an hour per user once T-133 is in; the journey reads it about five times. The run deletes the class and the students' yearbooks at the end (the accounts stay).

## The gallery check over HTTP/2 (T-107)

`tools/e2e/h2-gallery.spec.mjs` runs in the same `make e2e`: it registers a second account, uploads one photo, starts a small TLS proxy (throwaway certificate from `openssl`, HTTP/2 and
HTTP/1.1, one HTTP/1.1 request per stream to the API like CloudFront/the ALB) and loads a page of 30 and of 100 `<img>` thumbnails in Chromium over each protocol. Every image must render with status 200 (a cap
of 8 downloads per account left 22 of 30 broken over HTTP/2). `scripts/e2e.sh` passes the API address as `E2E_API_URL`.

## Run it locally

```sh
make up              # MySQL, Valkey, S3 store (once)
make e2e             # about 20 s of set-up, then the journey in about 10 s
```

`make e2e` creates and uses its own database `smem_test_e2e` (`make e2e E2E_DB=smem_e2e_mine` for another), never the `smemories` dev database.

Needs Go, Node 22, Docker for the stack, and poppler (`pdfinfo`, `pdffonts`, `pdftotext`, `pdfimages`; `brew install poppler`),
like `make print-check`. `scripts/e2e.sh` builds and starts the API on `127.0.0.1:18080` and the built web app on `127.0.0.1:14173`
(`E2E_API_PORT`, `E2E_WEB_PORT`), so a dev server on :5173 and an API on :8080 keep running, and stops only what it started.
The API runs with `SMEM_ENV=test` (Redis keys `smem:test:*`, which `make e2e` resets because registration is limited to 5 per hour per
address). The run creates its own data (unique email `e2e-<run>@example.com`, unique title) and deletes its yearbooks, notes and photos at
the end, also after a failure; the account itself stays (there is no endpoint to delete it). It can be re-run at once.

## Guards (T-087)

The journey writes real data, so it refuses to start unless the target is a throwaway local one. The checks live in `tools/e2e/guard.mjs`;
`scripts/e2e.sh` runs them before it builds or starts anything (exit 2 and a message) and `playwright.config.mjs` runs them again for a
direct `npx playwright test`:

- `E2E_BASE_URL` must have the host `localhost`, `127.0.0.1` or `::1`. Override on purpose with `E2E_ALLOW_REMOTE=1` (this check only).
- `SMEM_DB_DSN` must name a database starting with `smem_e2e` or `smem_test` (CI uses `smem_e2e`). There is no override.
- The host in `SMEM_DB_DSN` must be loopback (`127.0.0.0/8`, `localhost`, `::1`), a private address (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`) or a unix socket; a DNS name is refused because it can point anywhere. There is no override (T-106).

`cd tools/e2e && npm run guard-test` proves the refusals (it runs in `make e2e` and in the `e2e` job).

**Registration is limited to 5 per hour per address.** Run directly (`scripts/e2e.sh`, not `make e2e`, which resets the limits) after five
runs, step 1 fails with `registration is rate limited ...`: run `make reset-limits SMEM_ENV=test`, or point `SMEM_REDIS_URL` at a fresh
Redis db, and run again.

`tools/e2e/out/` (git-ignored) has the API log, the web log and, after a failure, the Playwright trace and screenshot:
`cd tools/e2e && npx playwright show-trace out/results/*/trace.zip`.

## Reading a CI failure

**The `e2e` job is paused (D-37, T-132; `image` and `go-integration` are paused too, see [ci.md](ci.md)):** it does not run on pushes and pull requests (it shows as skipped) and runs only by hand with `gh workflow run ci.yml --ref <branch>`. QA runs the same commands locally (`make e2e` and the print-check, see CLAUDE.md "CI scope"). To switch it back on, delete the `if:` line above the job.

The `e2e` job in `.github/workflows/ci.yml` starts MySQL, the S3 store and Valkey like `go-integration`, then runs `scripts/e2e.sh`. The
error starts with `STEP FAILED: <n. title>`, then Playwright's own message and code frame. On failure the artifact `e2e-failure`
(kept for 3 days) holds the trace (open it with `npx playwright show-trace`, or at trace.playwright.dev), the screenshot and `api.log`.
A trace holds every request, response body and page snapshot, so it would contain the note-link token, the photo-link tokens and the session cookie.
`scripts/e2e.sh` therefore runs `tools/e2e/scrub-trace.sh` on a failed run, before anything uploads the folder: it unpacks each `trace.zip`, replaces
`token=...`, `/c/<token>`, `/join/<token>`, `"token":"..."` and the `__Host-smem_session` cookie by `REDACTED`, and packs it again without directory entries (Playwright's reader fails on them; needs `unzip`, `zip` and `perl`; the
runner and macOS have them). It fails closed: a `trace.zip` that cannot be scrubbed and checked is deleted and the reason printed, so it is never uploaded; the run's exit code is kept. `npm run guard-test` in `tools/e2e` tests the script. The trace stays useful (steps, screenshots, request URLs and statuses) and the artifact expires after 3 days. The token
is also never printed to the job log (every error leaving a step is scrubbed of it). To check it:
`unzip -q out/results/*/trace.zip -d /tmp/t && grep -rE 'get-content\?token=[^R]' /tmp/t` finds nothing. A new bearer secret in the app needs a rule in the script. The job has a 10 minute limit (the journey itself 240 s) and caches the Chromium download per
Playwright version.

## Changing the journey when a screen changes

The test finds things by role and visible English text (the UI language is fixed to `en-US` in `playwright.config.mjs`), the way a
person sees them, so a renamed button or changed copy is a one-line fix in `journey.spec.mjs`. Keep a new step a `step(...)` with
a hard limit, wait for a screen state or a network response (never `waitForTimeout`), and give new data a per-run unique value.
If the notes-per-page of the template changes, update `NOTES_PER_PAGE`.

## Dev shortcut

The journey needs the registered fixed email code (`DEV-SHORTCUT(otp)`, `docs/dev-shortcuts.md`). `scripts/e2e.sh` hands it only to the
API it starts, with `SMEM_ENV=test`; the `e2e` job is the only CI job that sets `SMEM_DEV_FIXED_OTP`. T-050 removes the shortcut and
this test then needs real codes (a test mailbox) before the shortcut can go.
