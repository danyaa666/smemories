# End-to-end test of the M1 journey

One Playwright test (`tools/e2e/journey.spec.mjs`, Chromium only) drives the whole M1 journey in a real browser against the
**built web app**, the **real API** and the real MySQL, Valkey and MinIO, so a regression anywhere in the chain (auth, yearbook,
media, link, public form, moderation, book data, renderer, PDF) turns CI red. The manual half of the M1 exit criteria
(Safari, phones, the print dialog) stays in `docs/spikes/print/OWNER-TESTS.md`.

## The journey (each is a `test.step`, so a failure names its step)

1. Register, verify with the fixed development code (`DEV-SHORTCUT(otp)`), see the `user-xxxxxxxx` chip on the yearbooks page.
2. Create a yearbook (Vietnamese title, language `vi`, A5), fill the profile, upload a cover photo.
3. Pick the `memory-book` template and see the preview change.
4. Create a note link and read it once.
5. In a second, anonymous browser context open the link, send a note (diacritics, emoji, one photo).
6. The inbox shows 1 pending note and the sidebar badge; approve it.
7. Two more notes, approve them, move one up; the order survives a reload.
8. Print screen ready, `page.pdf()`: page count (`3 + ceil(notes / notes-per-page)`; memory-book has one friend per page, so 6),
   A5 size (420 x 595 pt), fonts embedded, title, owner and every note's author and message present, notes in book order, no text
   outside its page. The PDF helpers are the ones of `tools/print-check`.
9. Revoke the link: the friend's page is a dead end after exactly one lookup request; sign out and the protected pages redirect to sign-in.

## Run it locally

```sh
make up              # MySQL, Valkey, MinIO (once)
make e2e             # about 20 s of set-up, then the journey in about 10 s
```

Needs Go, Node 22, Docker for the stack, and poppler (`pdfinfo`, `pdffonts`, `pdftotext`, `pdfimages`; `brew install poppler`),
like `make print-check`. `scripts/e2e.sh` builds and starts the API on `127.0.0.1:18080` and the built web app on `127.0.0.1:14173`
(`E2E_API_PORT`, `E2E_WEB_PORT`), so a dev server on :5173 and an API on :8080 keep running, and stops only what it started.
The API runs with `SMEM_ENV=test` (Redis keys `smem:test:*`, which `make e2e` resets because registration is limited to 5 per hour per
address). The run creates its own data (unique email `e2e-<run>@example.com`, unique title) and deletes its yearbooks, notes and photos at
the end, also after a failure; the account itself stays (there is no endpoint to delete it). It can be re-run at once.

`tools/e2e/out/` (git-ignored) has the API log, the web log and, after a failure, the Playwright trace and screenshot:
`cd tools/e2e && npx playwright show-trace out/results/*/trace.zip`.

## Reading a CI failure

The `e2e` job in `.github/workflows/ci.yml` starts MySQL, MinIO and Valkey like `go-integration`, then runs `scripts/e2e.sh`. The
error starts with `STEP FAILED: <n. title>`, then Playwright's own message and code frame. On failure the artifact `e2e-failure`
(7 days) holds the trace (open it with `npx playwright show-trace`, or at trace.playwright.dev), the screenshot and `api.log`.
The trace contains the link token of the run: it belongs to a throwaway database that is gone with the job, and it is never
printed to the job log (every error leaving a step is scrubbed of it).

## Changing the journey when a screen changes

The test finds things by role and visible English text (the UI language is fixed to `en-US` in `playwright.config.mjs`), the way a
person sees them, so a renamed button or changed copy is a one-line fix in `journey.spec.mjs`. Keep a new step a `step(...)` with
a hard limit, wait for a screen state or a network response (never `waitForTimeout`), and give new data a per-run unique value.
If the notes-per-page of the template changes, update `NOTES_PER_PAGE`.

## Dev shortcut

The journey needs the registered fixed email code (`DEV-SHORTCUT(otp)`, `docs/dev-shortcuts.md`). `scripts/e2e.sh` hands it only to the
API it starts, with `SMEM_ENV=test`; the `e2e` job is the only CI job that sets `SMEM_DEV_FIXED_OTP`. T-050 removes the shortcut and
this test then needs real codes (a test mailbox) before the shortcut can go.
