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

**Step 7b (T-146)** is the design reaching the class book. Student A switches their book to `navy-classic` and saves a design through the API (burgundy, modern fonts, profile hidden, a replaced "Autographs" heading); the admin's `class-book/get` carries it as that member's `design`, the admin's class print draws A's pages in `burgundy/modern` without the profile page and with the text, B's pages and the class pages keep the default look, and **only the admin gets it: students A and B get 403** on `class-book/get` and nothing of the design text reaches them.

Registration is limited to 5 per address and hour, and the five specs together register 7 accounts (M1: 1, the gallery check: 1, the class journey: 3, the customisation journey: 1, the layout editor journey: 1). `scripts/e2e.sh` therefore starts the test API with `SMEM_RATE_REGISTER_PER_HOUR=20`, whatever `.env` says (`make` exports `.env`, whose 5 would otherwise win); to change it set `SMEM_E2E_RATE_REGISTER_PER_HOUR`, a variable `.env` never sets (test API only, the default stays 5), and `make e2e` still resets the limits first; after several direct `scripts/e2e.sh` runs
use `make reset-limits SMEM_ENV=test`. `class-book/get` is limited to 30 reads an hour per user once T-133 is in; the journey reads it about six times. The run deletes the class and the students' yearbooks at the end (the accounts stay).

## The customisation journey (T-147)

`tools/e2e/design.spec.mjs` runs in the same `make e2e` (about 8 s locally, well inside its 240 s limit; the specs together take about 35 s). One account, a Memory Book (A5, Vietnamese) with one approved friend note and a profile quote (printed on the profile page only):
(1) register, create the book, choose `memory-book`; (2) a friend sends a note and the student approves it; (3) with no design the print route draws `lavender/playful`, four sheets (cover, profile, notes, back) and `--mb-ink` `#3b2f4a`;
(4) the customise screen: Peach, the Classic font set, "All about me" switched off, the cover text "this book belongs to" replaced, Save design; (5) after a reload the screen shows the same choices and Save is disabled;
(6) the print route: `data-book-ready` and `page.pdf()` both say 3 pages, the profile sheet is absent (and its quote is not in the PDF), the cover holds the title and the new text, every page is `peach/classic`, `--mb-ink` is `#4a2c22`, all fonts embedded;
(7) Reset to template: the default look and four pages come back, the quote is printed again; (8) a design saved again, then the template picker on the customise screen switches to Navy Classic: the "design was reset" notice shows, `book-design/get` has `options: null`, the print route draws `navy/classic` and the note is still in the book.
The spec asserts the page, theme and text on the screen the student prints (DOM) and on the PDF (page count, fonts, text); pixel parity of the PDF with the preview is the print check's job (`docs/templates.md`, "What print-check covers"). A deliberate break (the print route ignoring the design) fails step 6.
The spec has no cover-title edit because the cover title is the book's own title (data, not a replaceable text); it edits `cover.belongs`, the cover's only replaceable heading.

## The layout editor journey (T-159)

`tools/e2e/editor-shell.spec.mjs` runs in the same `make e2e` (about 15 s). One account, a Memory Book (A5, Vietnamese) with no notes:
(1) register, create the book, choose `memory-book`; (2) the customise screen's "Free layout" link opens `/yearbooks/:id/editor`: the page strip has three pages (cover, profile, back), the cover is current, the safe margin guide shows;
(3) select the cover title, read its width in the property panel (it matches what the page draws), set it to 40 %, `book-layout/save` answers and the title on the stage is 40 % of the page wide;
(4) set the rotation to 4, undo with Ctrl/Cmd+Z, and that undo is saved too (width 40, rotation 0, Redo enabled); (5) after a reload the saved layout is drawn (width 40, rotation 0, Undo disabled because history is not saved), and "Reset this element" makes the title wide again;
(6) an item flush to the right and bottom edge (saved through the API) on A5, then A4, then Letter (the book's paper is changed through `yearbook/update`): the south-east handle sticks out of the page and `elementFromPoint` at its outer corner is the handle, so the page's clip does not hide it;
(5b) with every save delayed 1.5 s, the student sets the width three times and clicks "Preview as PDF" at once: the new tab, opened by the click, goes to `/book` only after the save in flight and the queued one are done and shows the new width each time; an out-of-range number in step 3 shows "Between 1 and 200" and restores the old value; (7) at 700 px wide the editor does not mount and the message with the link back shows. The test uses the role `spinbutton` for the panel numbers because a resize handle is also named "Resize from left".

## The editor gestures (T-160)

`tools/e2e/editor-gestures.spec.mjs` runs in the same `make e2e` (about 25 s; it reuses one account and one book and runs in order). Chromium only.
(1) mouse: on A5, A4 and Letter at 100 % and 125 % a drag (70 right, 45 down), a corner resize (40, 30) and a rotation (30 degrees) are measured against the pointer, with Alt held so nothing snaps; the error is at most 1 screen px (measured 0.03 to 0.55, rotation 0), one Ctrl/Cmd+Z undoes each gesture whole, and the four corners of a 44 x 44 px square around the handle all hit the handle;
(2) Shift keeps the ratio and snaps the angle to 15; without Alt the item snaps onto the page centre line and a guide is drawn, with Alt it does not; Escape during a drag puts the item back, records no step and sends no `book-layout/save` (it waits 2 s, longer than the 1.5 s debounce);
(3) keyboard only: Tab to the item, arrows (1 and 10 design px), Alt+Shift+arrows (resize), `.` `,` `Shift+.` (rotate), `?` (help list with 14 rows), Delete and undo, the live region text;
(4) an item flush to the right and bottom edge can be shrunk from its south-east handle that lies outside the page, on all three papers;
(5) a template element dragged over another one (the photo moved first, then the title over it): a click on the overlap reaches the title, the one drawn above, and Tab visits the photo before the title;
(6) touch at 768 px with CDP touch events: a swipe over an unselected item pans the stage and moves nothing, a tap selects, then a finger drag moves the item (within the 6 px snap pull);
(7) one item dragged among 100 (the document is served by the test through `page.route`, because the server allows 40 per page) under `Emulation.setCPUThrottlingRate` 4: at least 30 frames a second (measured 59.7, p95 frame 16.8 ms).
The numbers print as `T160-NUMBERS {...}` lines.

The editor keeps the whole box inside the page while it is dragged, resized, rotated or moved by keys (ADR 0004 line 30). That is stricter than the saved-layout bound (a box may hang out by half its size), which the server still accepts. A rotated item stops cleanly in a corner; nothing is cut by print. Pointer error is the quantisation of the stored thousandths: at most 0.55 px from 50 to 125 % zoom, about 1.05 px at 150 % and 1.03 px at 200 % (QA, Letter, Alt held).

Not proven by the test, to check by hand (the leader lists these for the owner): the shortcut set on **Windows and Linux** (Alt+Shift+arrow must not switch the input language or move a window; the `,` and `.` keys on a Vietnamese keyboard with Telex on; Alt+Left must still go back), **macOS Safari and Firefox** (Option+Shift+arrow, Option while dragging), **touch in Firefox and WebKit** and on a real tablet, **screen readers** (the live region texts), and a **keyboard layout where `,` or `.` needs AltGr or Shift** (the handler reads the typed character, so `<` and `>` also count).

A template element moved to a box equal to its own is a print-check test too (T-159 rework 1): "an override equal to the natural box leaves every editable element where it was", A5/A4/Letter, all eight editable ids, rect change below 0.15 % of the page width (measured 0.04 to 0.07 %; before the fix up to 12 %).

The stage against the book screen (T-159 AC3) is a print-check test: `make print-check`, "the editor stage draws the cover and a notes page exactly as the book screen does", on A5, A4 and Letter at zoom 100 %. Both are on-screen renderings, so the budget is 0.5 % of the pixels (measured on the fixture book: cover 0.00 to 0.08 %, notes page 0.00 to 0.34 %; the rest is glyph and gradient edges on a fractional CSS zoom, and Letter with its exact zoom is 0.000 %).

## Text boxes in the editor (T-161)

`tools/e2e/editor-text.spec.mjs` runs in the same `make e2e` (about 20 s). Chromium only. One account and book, in order:
(1) "Add text" gives a box in the default style, selected and open with its text selected; "Việt", "Nam" and "đường" are typed through the browser's real composition path (CDP `Input.imeSetComposition`, then `Input.insertText`) and none of the letters is lost (3 `compositionstart`, 18 `compositionupdate`, 3 `compositionend`); Escape keeps the text, one undo takes the whole typing back, redo brings it back, it is saved;
(2) at 499 code points a word started with 3 more runes finishes (502 during the composition) and is cut to 500 at `compositionend`; (3) paste: HTML on the clipboard inserts no markup, an HTML-only clipboard inserts nothing, 600 pasted emoji give 500 code points, Ctrl/Cmd+Z in the field is the field's own undo; Enter is a line break;
(4) the 41st box on a page is refused with a message; (5) the clip warning shows for 499 runes in a 220 x 70 box, stays while the field is open, and goes away with a larger box or a shorter text; (6) font, colour, alignment, bold and size reach the page and the saved layout, a size of 97 is refused ("Between 8 and 96"); (7) a text with only white space is removed on Escape and one undo brings it back; (8) with a touch context (`pointer: coarse`) a text of size 10 gets a field of 16 px scaled by a transform; (9) the **edit view against the printed view**: short, medium, long, 500-rune and clipped text, rotated 0 and -4 degrees, on A5, A4 and Letter at zoom 100 % (30 pairs of screenshots of the box, compared in the browser): the field takes the same font size as the print in all 30, no pixel differs by more than 48/255, and at most 0.6 % of the box differs at all (16 of 30 are exactly 0; the rest is glyph anti-aliasing of long or rotated text). The numbers print as `T161-NUMBERS {...}` lines.
Run it without the other specs with `cd tools/e2e && npx playwright test editor-text` against a running API and web server (`scripts/e2e.sh` starts them but has no filter). **`make e2e` inside a task worktree can pick up the worktree's `.env` and point at the main stack**: set the compose project, ports and `SMEM_*` variables by hand and call `scripts/e2e.sh` directly (docs/editor-text.md lists what is not proven by a machine).

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

In a git worktree (an agent's task checkout) `make up` copies `.env.example` to a `.env` of its own with the default ports, so `make e2e` there talks to the main stack (database `smem_test_e2e`, Redis keys `smem:test:*`). To use a private stack, run `scripts/e2e.sh` directly with `COMPOSE_PROJECT_NAME`, your own ports and `SMEM_TEST_S3_ENDPOINT` set, and say so in the PR.

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

The print-check suite has 181 tests (run `make print-check` for the current number) and takes about 5-6 min locally (about 319 s); the job's "Print check" step is limited to 15 min and the whole job to 25 min (T-149), and the step prints `print-check: N s`.

The `e2e` job in `.github/workflows/ci.yml` starts MySQL, the S3 store and Valkey like `go-integration`, then runs `scripts/e2e.sh`. The
error starts with `STEP FAILED: <n. title>`, then Playwright's own message and code frame. On failure the artifact `e2e-failure`
(kept for 3 days) holds the trace (open it with `npx playwright show-trace`, or at trace.playwright.dev), the screenshot and `api.log`.
A trace holds every request, response body and page snapshot, so it would contain the note-link token, the photo-link tokens and the session cookie.
`scripts/e2e.sh` therefore runs `tools/e2e/scrub-trace.sh` on a failed run, before anything uploads the folder: it unpacks each `trace.zip`, replaces
`token=...`, `/c/<token>`, `/join/<token>`, `"token":"..."` and the `__Host-smem_session` cookie by `REDACTED`, and packs it again without directory entries (Playwright's reader fails on them; needs `unzip`, `zip` and `perl`; the
runner and macOS have them). It fails closed: a `trace.zip` that cannot be scrubbed and checked is deleted and the reason printed, so it is never uploaded; the run's exit code is kept. `npm run guard-test` in `tools/e2e` tests the script. The trace stays useful (steps, screenshots, request URLs and statuses) and the artifact expires after 3 days. The token
is also never printed to the job log (every error leaving a step is scrubbed of it). To check it:
`unzip -q out/results/*/trace.zip -d /tmp/t && grep -rE 'get-content\?token=[^R]' /tmp/t` finds nothing. A new bearer secret in the app needs a rule in the script. The job has a 25 minute limit (the journey itself 240 s per spec, the print check 15 min) and caches the Chromium download per
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
