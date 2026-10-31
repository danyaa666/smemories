# Epic: e09-customisation-editor

PRD: [PRD.md](PRD.md) · Board index: [../../README.md](../../README.md)

Task blocks for this epic. The leader writes them with `board.py add-task --epic e09-customisation-editor`; status, branch,
PR and comments change only through `board.py`.

<!-- tasks:start -->

### T-148 — [E09] Customise screen: guard browser Back, ask before a template change, fast NotFound
- **Status:** DONE
- **Priority:** P3
- **Type:** feature
- **Milestone:** M1b
- **Depends-on:** —
- **Risk:** low
- **Rework:** 2
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-148-e09-customise-screen-guard-browser-back-
- **PR:** https://github.com/danyaa666/smemories/pull/144
- **Updated:** 2026-10-31 10:01Z by human
- **Comments-seen:** 9

Spec: .team/epics/E09-customisation-editor/08-customise-leave-guard-and-not-found.md (read it from the main checkout). Follow-up to T-144 QA: browser Back drops unsaved edits, template change drops them silently, non-owner waits 8-10 s for NotFound.

#### Comments
- 2026-10-30 08:40Z · dev · PR #144 head 788e0b9. Back: sentinel history entry while dirty (no data router). Template pick asks via TemplatePicker ask prop. NotFound slowness was owner-check yearbook/get retried 3x (not book-design/get): retryUnlessDenied on the customise observer only. vitest 668 pass, lint/typecheck/i18n/check:api/build green. QA: time non-owner NotFound in browser, Back at 375px, both locales. Known: multi-entry history jump skips dialog; extra Back press after confirmed template change. Print untouched. #143 not merged yet.
- 2026-10-30 08:58Z · qa · 1. [blocker] PR #144 (head 788e0b9) conflicts with develop (c0e3b68, T-145 merged after you branched). gh pr view 144: mergeable=CONFLICTING, mergeStateStatus=DIRTY. Repro: git merge-tree --write-tree 788e0b9 origin/develop -> CONFLICT (content) in web/src/pages/customise.test.tsx (locales en.json/vi.json auto-merge). Fix: merge/rebase origin/develop into the task branch, resolve customise.test.tsx keeping both T-145 and T-148 tests, re-run web checks (vitest, lint, typecheck, i18n, check:api, build), push, set READY_FOR_QA. CI on 788e0b9 was green (go, web, security pass; e2e/go-integration/image skipped by D-37/D-38). Not yet tested by QA (a head that cannot merge is not worth testing, the re-merge changes the code under test): browser Back, template-change dialog, NotFound timing (AC3 under 1.5 s), 375 px both locales, print-check. All will be run on the new head.
- 2026-10-30 12:05Z · dev · Merged origin/develop (T-145, c0e3b68) into branch, head ed7891b; only conflict was imports in customise.test.tsx (kept T-145 readFileSync/resolve + T-148 act). Both test sets kept. vitest 680 pass, lint/prettier, typecheck, lint:i18n, check:api, build green. Nothing else changed.
- 2026-10-30 12:33Z · qa · QA on head ed7891b (PR #144 MERGEABLE/CLEAN, CI go+web+security pass, e2e/go-integration/image skipped by D-37/D-38). 1. [blocker, AC3] Non-owner NotFound is NOT fast in a real browser. Repro: built web from ed7891b (npm run build; vite preview with SMEM_API_PROXY to the API built from the same head, SMEM_ENV=test), register user B, open /yearbooks/<A's book id>/customise as B with a fresh context (Chromium via Playwright). Expected < 1.5 s, one yearbook/get. Actual: 'This yearbook does not exist.' after 7366 ms (desktop) and 7365 ms (375 px), and yearbook/get is called 4 times: 404@54ms, 404@1064, 404@3082, 404@7096. Same 4 calls and 7.3 s on /yearbooks/<id>/book and /notes, so retryUnlessDenied on the Customise observer has no effect: the shell YearbookLayout (components/YearbookLayout.tsx:73) owns the same ['yearbook', id] query with no retry option and starts the fetch with the app default retry 3 (main.tsx makeQueryClient(3)); it only shows BookNotFound after those retries end. Suspected fix: put retry: retryUnlessDenied on the YearbookLayout query (or queryClient.setQueryDefaults(['yearbook'], {retry: retryUnlessDenied})); keep 5xx/network retries. If you widen it to every /yearbooks/:id screen, say so in the PR (spec said no other request's policy changes, but the 404 owner check is this one request). 2. [major, test gap] The vitest 'shows not-found at once for a 404 and a 403' passes while the real build takes 7.4 s, so it does not guard AC3. Make it reproduce the browser (e.g. count yearbook/get with the shell mounted and the real default retry of makeQueryClient(3), fake timers or a wait past the 1 s retry; check it fails without your fix) and re-time in a browser. What PASSED (real Chromium, API+MySQL, head ed7891b): AC1 no edits: Back leaves at once, no dialog, no extra history entry. With edits: history 3->4 (sentinel), Back shows 'Leave without saving?' (same dialog/wording as the link guard), URL unchanged; Stay keeps edits and URL (history stays 4); a second Back asks again; Escape = Stay; Leave goes to /book; Forward returns to customise. After Save, and after undoing the edit, Back leaves at once. In-app link guard still works (Stay/Leave); reload with edits still triggers beforeunload. AC2 template change with edits: dialog 'Change the template?' (names the loss); Keep editing keeps template and edits and sends 0 yearbook/update; Change the template saves the new template, drops the edits (clean), dialog closes. A pick with no edits asks nothing. 375 px (isMobile, touch) en and vi plus desktop vi: dialogs fit (x=16, w=343, no horizontal scroll, buttons 44 px high), both locales render the right strings. Known limits confirmed: history.go(-2) with edits skips the dialog and lands on /book; after a confirmed template change Back #1 stays on customise (stale sentinel, Editor is keyed by template), Back #2 reaches the book. Both acceptable as documented, non-blocking. Non-blocking: leaving via link and pressing Back within milliseconds shows the still-mounted editor with the old edits (lazy chunk Suspense), unrealistic. AC4 on ed7891b: lint+prettier, typecheck, lint:i18n 'i18n check ok', check:api 'api schema ok', vitest 'Test Files 35 passed, Tests 680 passed', build ok; make print-check: 'tests 161, pass 161, fail 0'. Not run: make test-integration and make e2e (web-only diff, no Go/migration/auth change). Re-test needs only the AC3 timing again (I will re-run the same script) plus the existing checks.
- 2026-10-30 12:33Z · leader · Leader, for the rework: root cause is the shell query in web/src/components/YearbookLayout.tsx:73, which owns the same ["yearbook", id] query and retries with the app default of 3; the retryUnlessDenied on the Customise observer is ignored. Fix it where all callers route through: apply retryUnlessDenied (no retry on 404/403, keep retry for network/5xx) to that shell query, or to the ["yearbook"] query defaults in the QueryClient, and check /book and /notes still behave (they share the query). Replace the vitest 'shows not-found at once' with a test that uses the app's real QueryClient defaults (not retry:false) and asserts exactly ONE yearbook/get request for a 404 and for a 403; then time the non-owner NotFound in a real browser and quote it (AC3 < 1.5 s). Also merge origin/develop first (T-146 merged as 5334c1c).
- 2026-10-30 12:58Z · dev · Head 317a1a3 (origin/develop merged, T-146/T-149 in). QA #1 (AC3): fix at the shared place, not the screen: makeQueryClient now sets setQueryDefaults(['yearbook'], retry: no retry on 404/403, up to the app's N for network/5xx), so the shell YearbookLayout, Customise and YearbookEdit all stop retrying; Customise's own retry option and retryUnlessDenied removed (isDenied helper in auth/errors.ts). Widens to every /yearbooks/:id screen (same request). QA #2: replaced the test with 'makes exactly one yearbook/get for a 404 and a 403' on /customise and /yearbooks/:id using the real makeQueryClient(3); it fails without the queryClient change (checked) and passes with it; 5xx retry test kept. Real Chromium (built web + API from this head, SMEM_ENV=test), non-owner user, fresh context: NotFound after 39-51 ms with exactly 1 yearbook/get (still 1 after 1.5 s more) on /customise, /book, /notes at 1280px and 375px (was 7366 ms / 4 calls). Checks: vitest 684 pass, lint+prettier, typecheck, lint:i18n, check:api, build green. Owner paths covered by the existing vitest suite, not re-driven in browser.
- 2026-10-30 13:31Z · qa · QA on head 317a1a3 (PR #144 MERGEABLE/CLEAN, merge-tree with fresh origin/develop clean; PR checks go, web, security pass; e2e/go-integration/image skipped by D-37/D-38). Rework fix verified. Browser: Chromium/Playwright against the built web + API from this head (SMEM_ENV=test, fresh DB).
  AC3 non-owner NotFound (was 7.37 s, 4 calls): /customise 76 ms desktop, 68 ms 375px; /book 82/69 ms; /notes 62/68 ms; exactly 1 yearbook/get (404) each, none after 1.5 s more. 
  Owner path (new): /customise, /book, /notes load at both widths, 1 yearbook/get (customise shows 2 identical 200s: shell + screen refetch on mount, not retries). Mocked 500 on yearbook/get: 4 requests at 52/1060/3067/7073 ms on /customise and /book (retry kept); aborted network: 4 requests, same schedule. Mocked 404: NotFound in 60 ms, 1 request. Mocked 403: 1 failed request per observer (2 total, no retries), screen shows the generic 'Something went wrong / Try again' (not NotFound; pre-existing: isBookGone covers 404 only; the real API answers 404 for foreign books, so minor). Only key ['yearbook', id] is affected (grep: Layout, Customise, YearbookEdit).
  AC1 Back: no edits -> leaves at once; edits -> dialog 'Leave without saving?', URL unchanged, Stay keeps edits/URL (second Back asks again), Escape = stay, Leave goes to /book, Forward returns; after Save or undone edit Back leaves with no dialog; link guard still works; reload gives beforeunload. Dialog fits at 375 px (343 px wide, no horizontal scroll) and desktop; en and vi.
  AC2 template change with edits: dialog names the loss; cancel ('Keep editing'/'Tiếp tục chỉnh') keeps template, edits, 0 update calls; confirm changes template and clears dirty; en/vi x desktop/375 all OK.
  AC4 web checks in worktree: lint+prettier, typecheck, vitest 684/684 (35 files), lint:i18n, check:api, build all exit 0. make print-check: 163 tests, 163 pass, 0 fail. make e2e (with COMPOSE_PROJECT_NAME=awesomeproject1): 6 passed (journey, class, h2 gallery x4) on head 317a1a3. No Go change, so go-integration not run.
  Non-blocking: (a) known limits from dev still hold: history.go(-2) with edits skips the dialog; after a confirmed template change one extra Back stays on /customise before reaching /book. (b) 403 shows the generic error, see above. (c) leaving via link then pressing Back within ~0.5 s (before the lazy Book chunk renders) shows the old draft; harmless.
- 2026-10-30 13:32Z · leader · merged f4b0d88. Review: customise leave guard (history entry kept while edits are unsaved), template-change confirm dialog, and the no-retry-on-404/403 rule moved into makeQueryClient for the [yearbook] query (affects every /yearbooks/:id screen, intended). QA: non-owner NotFound 62-82 ms with one yearbook/get on /customise, /book, /notes at desktop and 375 px (was 7.37 s); 500 and network errors still retry 4x; Back and template dialogs ok in en+vi; vitest 684, print-check 163/163, e2e 6/6; go/web/security green on 317a1a3. Known limits (non-blocking): history.go(-2) with edits skips the dialog; one extra Back press after a confirmed template change; a mocked 403 shows the generic error instead of NotFound (the real API answers 404). For owner to check: leave /customise with Back on a phone.
- 2026-10-31 10:01Z · human · accepted by owner (2026-10-31, block restored after an old board.py dropped it)

### T-150 — [E09] Spike: layout editor architecture and ADR 0004
- **Status:** IN_PROGRESS
- **Priority:** P2
- **Type:** spike
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** dev
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-31 10:00Z by dev
- **Comments-seen:** 1

Spec: .team/epics/E09-customisation-editor/09-editor-architecture-spike.md (read it from the main checkout). Answers units/page sizes, drag-resize-rotate library, in-place Vietnamese text editing, undo technique and print parity with a prototype under docs/spikes/editor/.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.

### T-151 — [E09] Asset pack and registry for stickers, shapes and frames
- **Status:** TODO
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-31 10:00Z by leader
- **Comments-seen:** 1

Spec: .team/epics/E09-customisation-editor/10-sticker-shape-frame-asset-pack.md (read it from the main checkout). 48+ CC0/OFL/authored SVG assets, registry (web + Go), SVG safety check script, docs/assets.md.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.

### T-152 — [E09] Backend: student-uploaded design images (backgrounds and decorations)
- **Status:** TODO
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-31 10:00Z by leader
- **Comments-seen:** 1

Spec: .team/epics/E09-customisation-editor/11-design-image-uploads.md (read it from the main checkout). Media purpose=design, isolation from photos, quotas, class admin read path, privacy review. High risk: owner approves before merge.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.

<!-- tasks:end -->
