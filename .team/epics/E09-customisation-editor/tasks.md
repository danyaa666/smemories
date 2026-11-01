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
- **Status:** MERGED
- **Priority:** P2
- **Type:** spike
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-150-e09-spike-layout-editor-architecture-and
- **PR:** 148
- **Updated:** 2026-10-31 11:05Z by leader
- **Comments-seen:** 8

Spec: .team/epics/E09-customisation-editor/09-editor-architecture-spike.md (read it from the main checkout). Answers units/page sizes, drag-resize-rotate library, in-place Vietnamese text editing, undo technique and print parity with a prototype under docs/spikes/editor/.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.
- 2026-10-31 10:22Z · dev · ADR 0004 (proposed) + prototype in docs/spikes/editor, head 7f15c3a. Recommend: centre unit (width thousandths + vertical centre fraction, clamp), in-house gestures (no dependency), textarea overlay for text, snapshot-of-references undo; parity 0.04-0.14% px. Verify: npm run measure per README; repeat Vietnamese typing by hand (macOS Telex, Safari/iOS, Firefox NOT proven: only Chromium IME events driven). Diff touches only docs/adr/0004-* and docs/spikes/editor/.
- 2026-10-31 10:22Z · dev · Stage-2 sketch changes are listed at the end of ADR 0004 (no react-moveable, keyboard moves into the gesture task, Sheet gets a layer slot, text task needs manual Telex/iOS checks). Nothing missing except the manual Telex/Safari/Firefox checks.
- 2026-10-31 10:30Z · qa · QA T-150, head 7f15c3a (PR #148 mergeable; go/security/web green; image/e2e/go-integration skipped by policy). Most of the spike holds; 2 majors to fix, 3 minors. 
  PASSED (my own runs): AC4 diff = docs/adr/0004-layout-editor.md + docs/spikes/editor/** only (no node_modules tracked). AC2 clean export of HEAD (git archive web docs; cd web && npm ci; cd docs/spikes/editor && npm ci; npm run measure, 1m20s on Apple silicon, poppler present): parity A5 0.14%/A4 0.06%/Letter 0.04% and sticker offset 1/1/0 px identical to the ADR; bundle +2.7 KB vs +81.5 KB gzip; in-house 60 fps p95 16.7 ms @100 items 4x throttle (ADR 60), Moveable 59.8; 300 items 53.3 fps p95 50 ms (ADR 53.8); Moveable stage-scale 0.6 bug reproduced (72.9 vs 121.6); undo 0.58 MB/4us/24.4 MB/181us/0.06 MB identical; text.json 8/8 textarea pixel-identical, contenteditable 0..0.6%; Vietnamese (Viet Nam duong -> 'Viet Nam duong' with accents, 3 start/19 update/3 end events) reproduced on both techniques (AC3 real, but Chromium CDP imeSetComposition only). units.json arithmetic re-run and re-derived by hand (Letter pct +9.7%, mm -11.6 mm, centre 1.7 mm, anchor -82 px pair gap): correct. Extra, Playwright Firefox 157 and WebKit (browsers in a scratch PLAYWRIGHT_BROWSERS_PATH, no global install): in-house mouse drag exact (87.5), rotate 90 deg, keyboard arrows/Shift/Alt resize/[ ] rotate work in Chromium, Firefox and WebKit; textarea-vs-print parity 0 px in WebKit for all 8 cases.
  FAIL:
  1. [major] docs/spikes/editor/results/units-grid.png (Q1 'show the cover with one sticker and one text box on all three sizes for each candidate') is wrong. Rows mm, centre, anchor show NO sticker and NO text box on any size, even A5; the pct row shows all four candidates drawn on top of each other (doubled heart on Letter). Repro: npm run dev, open /?view=grid, or look at the png; in the browser [data-item] is 8 per sheet in pct-A5 and 0 in mm-A5. Cause: src/book.jsx Sheet finds its host with document.querySelector('[data-sheet="${size}-${scale}-${print}"] .bk-inner') and the key has no unit, so every row's portal lands in the first (pct) row. Fix: make the key unique (add a per-instance id / unit) and regenerate the png; check the ADR Q1 table against the real picture. The numbers in units.json are fine.
  2. [major] The recommended unit ('centre') is not exercised by the gesture code off the A5 page, and the ADR says drags are exact. interact.mjs runs only on A5, where all four units coincide. Repro: /?view=editor&lib=inhouse&size=Letter&unit=centre&n=2, drag #t1 100 screen px up: the item moves 22 px (ratio 0.22; unit=pct also 0.22, unit=anchor 1.12), A5+centre is 1.0 (my Playwright mouse drag). Cause: Item keeps box in A5-reference design px and runs UNITS[unit].store then place(size), while the gesture writes pointer deltas of the displayed page into that A5 box (and the snap lines mix A5 boxes with H(size) page lines). Expected: edit in the stored form (x, w, h, cy) and write back the inverse of place for the displayed paper (cy = (top + h/2) / H(size)); prove one drag on Letter and A4 with centre (pointer and item within 1 px) and add it to interact.mjs and the ADR Q2 table; or state in the ADR that the editor must do this and mark it unproven. Include the clamp interplay (a drag that ends in the clamp zone).
  3. [minor] ADR Q1 table, mm row: 'a sticker 50 px above the A5 bottom edge ... gone'. The sticker box is 58 px above the A5 bottom (y 950 + 150 vs 1158), and on Letter it is 106 of 150 px visible (bottom 11.6 mm past the page), i.e. cut, not gone. Same '50 px' in the comment of scripts/units.mjs.
  4. [minor] Fit is not re-run when web fonts finish loading in the prototype (text.jsx View calls fitElement once per value). Playwright Firefox, same URL, 3 loads of /?view=text&tech=ta&mode=edit&text=<LONG>: font size 18.0069px, then 19.9899px twice (wrong wrapping, 7 lines vs 6); textarea-vs-print diff 25-35% in Firefox for the LONG and FULL cases (0% in Chromium and WebKit). The real web/src/book/fit.tsx FitText already refits on fonts.ready and loadingdone, so the product is fine only if the text task reuses it and the textarea takes its size from it: say so in ADR Q3/'Changes' item 4 so the claim 'same function, pixel-identical' is not read as proven for Firefox.
  5. [minor] Q3 proof has holes the ADR should list under 'Risks left': (a) all 8 parity cases are fit state ok or shrunk, none in 'overflow' (clip) which the ADR decides on; with a 220x70 box and 499 runes the state is overflow, the textarea stays at scrollTop 0 with the caret hidden (caretColor transparent in the prototype), so typing at the end is blind and caret behaviour was never visibly checked; (b) the controlled textarea rewrites value when it caps, which moves the caret to the end when typing in the middle of a full text; (c) no pointerId filter in the in-house gesture (a second finger starts a second gesture / deselects); (d) alt+arrow is browser Back on Windows/Linux (preventDefault is called, check) and [ ] need AltGr on some layouts.
  CHALLENGES that hold up: centre unit on Letter is sensible (item near the bottom keeps its proportional place, 1.7 mm clamp, stateless); in-house touch/keyboard/rotation work as described on 768 px; Vietnamese proof is a real composition-path test in Chromium.
  STAYS UNPROVEN (nobody here has it): real macOS Telex typing, real Safari/iOS (WebKit under Playwright only matches layout; no IME, no focus zoom), real Firefox IME, Android Gboard Telex, touch drag in Firefox/WebKit, screen readers.
  Result lines: no Go/integration/e2e/print-check run (docs and throw-away prototype only; no product code in the diff).
- 2026-10-31 10:30Z · leader · Leader, for the rework: fix QA's two majors and the minors. (1) units-grid.png: key every Sheet portal by unit so each row shows its own sticker and text box; regenerate the picture and units.json. (2) Gestures must edit in the stored form (inverse of place) so a drag moves the item by the same displayed distance on A5, A4 and Letter in every unit: prove a drag on Letter and A4 (numbers in the ADR) or mark gestures-on-non-reference-sizes UNPROVEN in the ADR and say which unit needs it. Minors: correct the ADR mm row (58 px, cut, 106 of 150 px visible); re-run fitElement when fonts load (or state the Firefox 25-35% variance as a requirement for the runtime task); add the clip/overflow state to the Q3 proof and show the caret; add a pointerId filter. Keep the 'still unproven' list in the ADR (real macOS Telex, Safari/iOS, Firefox IME, Android Gboard Telex, touch on Firefox/WebKit, screen readers); the owner will check Telex by hand. Dev is on T-151 now: pick this up at the next wake-up (rework has priority). Merge origin/develop first.
- 2026-10-31 10:56Z · dev · Rework, head 60075ad (origin/develop merged, no force-push). QA#1 major: Sheet portal host keyed per instance (useId); units-grid.png regenerated, each row shows its own sticker+text, drawn as stored (no clamp); mm Letter sticker cut (106 of 150 px). QA#2 major: gestures/keyboard now edit the box as drawn on the edited page, clamp, write back unplace (stored form; anchor keeps ay in item.st). New scripts/gestures.mjs + results/gestures.json: 4 units x A5/A4/Letter, pointer error 0.0 screen px, clamp-zone drag ends flush then 100 px up moves exactly 100, keyboard 1/10 px, second pointer ignored (pointerId filter); ADR Q2 table. Minors: ADR mm row 58 px/cut/106 of 150; fitElement re-run on fonts.ready+loadingdone (Firefox 157: stable 19.99/18.01 px, 0% vs print; engines still differ from Chromium by 0.5 px steps, stated as requirement); text parity now 10 cases incl. clip state 499 runes 220x70, all 0 px; caret visible, typing at end of clipped text scrolls the field (99 px), caret kept after cap rewrite. New finding: resize handle of an item flush to the page edge is clipped by .bk-inner (ADR risk 8; handles must be in an overlay outside the clip). Still unproven list kept (real Telex, Safari/iOS, Firefox IME, Gboard, touch FF/WebKit, screen readers, Alt+Arrow/AltGr on Win/Linux). Verify: npm run measure per README (or node scripts/gestures.mjs with npm run dev). Diff touches only docs/adr/0004-* and docs/spikes/editor/.
- 2026-10-31 11:05Z · qa · QA_PASS on head 60075ad (record restored after add-task overwrote the epic file): parity 0.14/0.06/0.04 %, gestures exact on A5/A4/Letter, Firefox text parity 0 %, diff only docs/adr/0004 and docs/spikes/editor
- 2026-10-31 11:05Z · leader · merged d086971. Review: docs/adr/0004-layout-editor.md and docs/spikes/editor/** only; ADR accepted by the leader (nits fixed after merge). Still unproven (checked by hand at the text task): real macOS Telex, Safari/iOS and Firefox IME, Android Gboard Telex, touch drag in Firefox/WebKit, screen readers, Windows/Linux shortcuts. For owner to check: skim the ADR verdict paragraph.

### T-151 — [E09] Asset pack and registry for stickers, shapes and frames
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-151-e09-asset-pack-and-registry-for-stickers
- **PR:** https://github.com/danyaa666/smemories/pull/149
- **Updated:** 2026-10-31 10:54Z by leader
- **Comments-seen:** 4

Spec: .team/epics/E09-customisation-editor/10-sticker-shape-frame-asset-pack.md (read it from the main checkout). 48+ CC0/OFL/authored SVG assets, registry (web + Go), SVG safety check script, docs/assets.md.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.
- 2026-10-31 10:43Z · dev · PR #149 head c6bec17. 51 assets (25 authored, 26 Openclipart CC0), registry + Go copy, check-assets.sh in CI web job, AssetSvg, dev-only contact sheet /dev/assets. QA: spot-check 5 licences/<id>.txt against the Openclipart pages, plant a bad svg in scripts/check-assets.sh, run web+Go checks, open /dev/assets under npm run dev. Gaps: print-check and integration not run (no book/store change); spec field list extended with licence_file; file is index.tsx not index.ts.
- 2026-10-31 10:53Z · qa · QA_PASS T-151, PR #149 head c6bec17 (branch == PR head). CI on that head: go pass 4m54s, web pass, security pass (e2e/go-integration/image skipped by D-37/D-38).
  LICENCE (the point): fetched live Openclipart pages for ALL 26 curated clips (not just 8): every page carries the creativecommons.org/publicdomain/zero/1.0 licence metadata, no pd_issue tag on any, the artist on the page equals registry author for all 26, source_url resolves (HTTP 200), titles match the licence-file headers. openclipart.org/share FAQ re-read: 'We use the Creative Commons Zero 1.0 Public Domain License every time an artist uploads'. date_checked 2026-10-31 = today, plausible. Licence files: 26 per-id txt + authored.txt (27 files), content matches registry. Nothing non-CC0 found, no blocker. Non-blocking: three clips are remixes (sticker.graduation-cap remix of GDJ; smiling-star derived from PublicDomainPictures.net; cap-and-book from publicdomainvectors.org) so CC0 there rests on the upstream PD claim as Openclipart states it; acceptable. Licence txt files are not machine-checked for the word CC0 (script only checks they exist); minor.
  SVG SAFETY: 51 SVGs pass; grep finds no image/text/style/base64 in any; no dangerouslySetInnerHTML in assets/. Planted on a scratch copy, scripts/check-assets.sh exits 1 with a named problem for each: <script>, <image href=http://...>, onclick attr, 21207-byte file (max 20480), url(http..) paint, <foreignObject>, SVG not listed in registry, deleted licence file, registry entry with licence field removed, extra svg file. Clean tree: 'check-assets ok: 51 assets', rc 0. Wired into CI web job (ci.yml:345).
  AC1 51 assets (25 authored, 26 CC0; 34 sticker/11 shape/6 frame), both labels, licence data+files: PASS. AC2 scripts/sync-assets.sh leaves internal/assets/registry.json byte-identical (cmp), TestRegistryMatchesWeb + web registry test pass: PASS.
  AC3 npm run dev on :5287, Playwright/Chromium at /dev/assets: 51 cells, 102 inline SVGs (plain+tinted), 0 empty getBBox, 0 console errors; screenshot viewed, all render, authored ones recolour, curated keep own colours; PR has docs/assets/contact-sheet.png. AC4 built the branch and merge-base 474ce11: Book-i87JpjYm.js and Book-DWBJ3fHA.css byte-identical (shasum), Customise/ClassBook same size, index js 468,563 -> 468,574 (+11 B, dev-route constant), no asset chunk in dist: PASS.
  AC5 web: lint+prettier clean, typecheck clean, i18n ok, vitest 36 files / 696 tests pass, build ok; go test -count=1 ./... all ok; make print-check 163/163 pass 0 fail (unchanged). make test-integration not run: no store/migration change, risk low. docs/assets.md (98 lines: layout, entry fields, SVG rules, licence rules, add steps, server use, sources table of all 26) and THIRD_PARTY_NOTICES.md section accurate vs registry. Spec deviations noted by dev (licence_file field, index.tsx) are fine.
- 2026-10-31 10:54Z · leader · merged 8ad8c0c. Review: 51 assets (25 authored CC0, 26 Openclipart CC0), registry in web and embedded Go copy kept in sync by scripts/sync-assets.sh, check-assets.sh in the web job, AssetSvg module lazily loaded, contact sheet at /dev/assets (dev only), docs/assets.md and THIRD_PARTY_NOTICES.md. QA: all 26 Openclipart pages re-read (CC0 1.0 stated, artists match, none tagged pd_issue); check-assets.sh fails on 10 planted faults; print bundle byte-identical; vitest 696, go test, print-check 163/163; go/web/security green on c6bec17. For owner to check: flip through docs/assets/contact-sheet.png; 3 clips (graduation-cap, smiling-star, cap-and-book) are Openclipart remixes whose CC0 rests on the upstream public-domain claim: say if you want them replaced by authored ones.

### T-152 — [E09] Backend: student-uploaded design images (backgrounds and decorations)
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-152-e09-backend-student-uploaded-design-imag
- **PR:** https://github.com/danyaa666/smemories/pull/150
- **Updated:** 2026-11-01 04:10Z by leader
- **Comments-seen:** 5

Spec: .team/epics/E09-customisation-editor/11-design-image-uploads.md (read it from the main checkout). Media purpose=design, isolation from photos, quotas, class admin read path, privacy review. High risk: owner approves before merge.

#### Comments
- 2026-10-31 10:00Z · leader · Owner approved the stage 2 plan on 2026-10-31 (D-44). Read the spec from the main checkout before you start.
- 2026-10-31 11:17Z · dev · PR #150 head f9fbe1e. Migration 20261031120000 (purpose, Down deletes design rows), media/create + get-list purpose=design, 30 cap + shared 500 MiB, isolation (cover/profile/public link/list tokens), class admin read path via readable (open point: includes images no layout uses yet), metric smem_media_uploads_total, docs/openapi/schema/Postman (design-images folder in customisation-editor). Run on own compose project t152: go test -race ./... ok, integration all packages ok, golangci-lint 0, check-class-matrix ok, check:api ok, newman 59/59 + media 172/172, e2e 7 passed (class journey included). Verify: TestDesignImageIsolation, TestDesignQuotas, TestDesignImageDelete, TestAdminMediaReadPath, TestMigrateMediaPurpose. High risk: owner approves before merge. No account-deletion/export endpoint exists (T-025 note in docs/media.md).
- 2026-10-31 15:24Z · qa · QA_PASS T-152, PR #150 head f9fbe1e (branch = PR head, worktree clean). Own compose project qa152 (ports 23306/26379/29000, SMEM_TEST_S3_ENDPOINT set), torn down after. PR CI: go, web, security green.
  RESULT LINES (local, head f9fbe1e):
  - make test-integration equivalent (go test -race -count=1 -timeout 30m -tags integration ./...): 32 packages ok, 0 FAIL (media 131 s, class 219 s, db 171 s, yearbook 111 s). First attempt failed only because SMEM_TEST_S3_ENDPOINT pointed at the default port (403 on CreateBucket); rerun against my own S3 passed.
  - go test -race ./... : 31 ok, 0 FAIL. web vitest 36 files / 696 tests passed. gofmt clean, go vet (+integration tag) ok, golangci-lint 0 issues, web lint/typecheck/lint:i18n/check:api ok.
  - scripts/check-class-matrix.sh: 21 class operations, all in docs/class-privacy-review.md.
  - scripts/e2e.sh (own DB smem_test_e2e): 7 passed (class journey, design journey, h2 gallery x4, M1 journey), EXIT 0.
  - print-check (tools/print-check npm test): 163 tests, 163 pass, 0 fail.
  - newman (npx newman 6.2.3, baseUrl 127.0.0.1:28080, fresh rate limits): customisation-editor folder "design-images (T-152)" 59/59 twice back to back; media.postman_collection 172/172 twice. Whole customisation-editor file: 191 assertions, 3 fail, all in the OLD book-design folder, steps 17/18 (expects navy-classic to have nothing to customise; false since T-145 made Navy customisable). That folder is unchanged by T-152 and the same step is on develop: pre-existing, not a T-152 defect, non-blocking (leader may want a small follow-up task).
  MIGRATION: up/down/up on a DB with existing photos (1 owner + 1 contributor row): up gives purpose='photo' on both; inserted a design row; Down removed only the design row, kept both photos and dropped the column, version back to 20261030120000; up again ok (20261031120000). Down deleting design rows is documented in the migration header and docs/media.md ("Design images"); the S3 objects of those rows stay until the yearbook is deleted (documented, acceptable).
  AC TABLE (all exercised live against a real API, plus the dev's integration tests which pass):
  AC1 upload design 200 (purpose in answer; JPEG/PNG/WebP all stored as jpeg); get-list default shows photos only, purpose=design only designs, uploader=all/contributor lists no extra; display/thumb/print serve 200 to owner; 30 designs fill and the 31st is 409 ERROR_QUOTA_EXCEEDED while a photo upload in the same book still works and a second book takes designs (own cap per book, not counted toward the 200 photos); shared 500 MiB bytes: not reachable live, covered by TestDesignQuotas (passes, rows seeded with large sizes; design refused when photos fill the pool and vice versa).
  AC2 design id as cover and as profile photo: 400 ERROR_INVALID_MEDIA (photo id control 200). Photo id as design image: no HTTP consumer exists yet; only Service.DesignImages (exact-match set, TestDesignImageIsolation passes) - T-153 must call it before storing a media id. Public link: TestDesignImageIsolation shows a correctly signed token for a design id gives 404 while a photo token gives 200.
  AC3 live class run (admin, linked student s1, member s2, other-class admin, anon): admin 200, owner 200, other member 404, other-class admin 404, anon 401; unlinked student's design 404 for everyone; admin cannot list, delete (200 no-op, image still readable by owner) or upload into the member's book (404); unlinking the page ends admin access at once. docs/class-privacy-review.md and class-permissions.md list the new data and read path. Class admin read exposes ALL design images of a linked member (also unused ones): matches spec req 5 (admin reads members' design images) and the dev's open point; T-153 narrows it. Reported, not a fail.
  AC4 media/delete by owner works (idempotent; non-owner call returns 200 and deletes nothing, same as photos; reads 404 after). Yearbook delete removed the design and photo rows and content 404 (DB count 0). There is no account-deletion or export endpoint in the code yet, so "export lists them" can only be the documented plan for T-025 (media.md, privacy review); not testable.
  AC5 see MIGRATION above; photo behaviour unchanged (media 172/172, all photo integration tests pass).
  AC6 see RESULT LINES.
  EDGE/SECURITY: purpose values photo, Design, DESIGN, empty, "design " -> 400 ERROR_PARAM (list: purpose=photo also 400, documented); purpose field before or after the file works (never taken for a photo); gif/txt/svg/empty/text-as-jpeg -> 415; truncated jpeg 400 ERROR_INVALID_IMAGE; 11 MiB file 413 ERROR_TOO_LARGE; 6000x6000 and 20000x20000 flat PNG -> 400 (pixel bomb refused); EXIF/maker string stripped from stored display image; non-owner and anon: upload/list/read 404/404/404 and 401. Metric smem_media_uploads_total{purpose,result} seen on the metrics listener with design ok/quota/invalid/busy and photo ok (busy also counts the per-user rate limit); no file names or ids in labels.
  NON-BLOCKING: (a) spec text says display 2000 px, dev kept the existing 3000 px display and documents that print (1800 px) is too small for a full-bleed A4 background, so the layout must use display; the PR states which variant, as the spec asks. (b) The 3 stale book-design newman assertions above. (c) Metric result "busy" mixes the pool-busy and the per-user rate-limit refusals.
  Not merged. High risk: owner approves before merge.
- 2026-11-01 04:10Z · human · owner approved merge
- 2026-11-01 04:10Z · leader · merged 34d8b5a (PR #150, head f9fbe1e); QA_PASS with full local suite; owner approved

### T-153 — [E09] Layout document model and API (backend)
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** T-152
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-153-e09-layout-document-model-and-api-backen
- **PR:** https://github.com/danyaa666/smemories/pull/152
- **Updated:** 2026-11-01 04:55Z by leader
- **Comments-seen:** 5

Spec: .team/epics/E09-customisation-editor/12-layout-document-model-and-api.md (read it from the main checkout). book_layout_tab, document v1 (ADR 0004 units), booklayout package, book-layout/get|save|reset with optimistic concurrency, manifest design.layout, book-data/get carries layout.

#### Comments
- 2026-10-31 11:18Z · leader · Spec updated: new requirement 9 (narrow the class admin's read of design images to images the stored layout references; T-152 left it open) and AC6. Read the spec again when you start.
- 2026-11-01 04:10Z · leader · T-152 merged (34d8b5a); spec updated with admin-read narrowing and DesignImages check
- 2026-11-01 04:38Z · dev · PR #152 head 3aeaf04. Backend only (schema.d.ts regenerated). Run: go test -race ./..., integration for booklayout/bookdata/bookdesign/class/db/yearbook/templates/config/metrics/notes/media/bookexport, scripts/check-class-matrix.sh (now also book-layout/* and media/get-content), newman folder 'book-layout (T-153)' 40 req/104 assertions 0 failed, e2e 7 passed (own compose dev153). Notes: anonymous gets 401 (like book-design), non-owner/class admin 404; reset restarts version at 1; book-data layout fields not 'required' in OpenAPI (keeps web fixtures typing); account delete/export do not exist yet (T-025). Not run: print-check (no web/print change).
- 2026-11-01 04:54Z · qa · Head 3aeaf04 (PR #152). PR CI: go pass, web pass, security pass (e2e/go-integration/image skipped by design). Own compose project qa153 (mysql 13306, redis 16379, s3 19000, API 18153 built from head), torn down after.
  LOCAL RUNS on 3aeaf04: go build+vet ok; go test -race ./... ok; make test-integration equivalent (go test -race -count=1 -timeout 30m -tags integration ./...) 33 packages ok, 0 FAIL (booklayout, bookdata, bookdesign, class, db, media, notes, bookexport, config, metrics, templates all ok); make lint exit 0; web check:api ok, web vitest ok; scripts/check-class-matrix.sh ok (24 class and book-layout ops); newman customisation-editor collection (--working-dir postman) whole file twice back to back with rate limits reset: 135 requests, 295 assertions, 0 failed both runs (includes book-layout (T-153) 40 requests); scripts/e2e.sh 7 passed; tools/print-check npm test 163 pass, 0 fail.
  AC1: save -> get is identical (deep equal), version 1; same base_version 0 again -> 409 ERROR_VERSION_CONFLICT with stored document; 8 parallel saves base=1 -> exactly one 200, seven 409.
  AC2 (my own HTTP script, each with precise path+code): 40 items per page ok / 41 too_many pages.cover.items; 150 total ok / 151 too_many at pages; w 10 ok / 9 out_of_range, w 2000 ok / 2001, h 2000 ok / 2001, x -w/2 ok / one less rejected, x 1000-w/2 ok / +1 rejected, cy bounds (ratio 1.419) ok at the edge / rejected one past, r +-180 ok / +-181 rejected, floats/strings/1e30/int64 overflow rejected; text 500 code points ok (Vietnamese, emoji, combining) / 501 too_long, tab/NUL/DEL/bidi override rejected, script text stored as text; dup item id (also across pages), bad id length/chars, z key unknown_key, unknown page kind, override of unknown element or another page's element, unknown asset, tint on non-tintable, unknown colour/font/align, size 8..96 ok / 7 and 97 rejected, radius 501, fit, bg with two keys; at most 20 problems reported; v=2, missing pages, unknown top key, [] / string / 1 as layout; dup JSON key, NaN, trailing garbage, 50000-deep nesting -> 400 not a crash. Media: own design image ok (item and bg); a regular photo, a photo set as the profile photo, B's design image, a random ulid and a non-ulid all -> 400 not_own_media with path. 128 KiB: a 131072-byte layout value saves (200); 131073-byte value -> 413 ERROR_TOO_LARGE; 5 MB body -> 413.
  AC3: B (non-owner): get/save(valid and invalid)/reset all 404; anonymous 401 on all three. book-design behaves the same (401 anonymous, checked: get/update/reset), so the dev's deviation from the spec text is consistent. Foreign Origin -> 403, GET on save -> 405. Class admin 404 on book-layout/* is covered by the class matrix test rows (checked by the script; integration class package ok).
  AC4: template change (memory-book -> navy-classic) deletes layout (get: null, version 0), AND the design hook still runs (options null after), design image and photo list and profile name survive; save on navy-classic -> 400 template_not_customisable; same-template update keeps layout; yearbook delete removes the row (count 10 -> 9). Account deletion and data export have no endpoints yet (T-025); dev documented the obligation in docs/templates.md and docs/class-privacy-review.md - n/a here.
  AC5: book-data/get carries layout + layout_version (128 KiB layout: 200 OK); after deleting the design image the layout in book-data/get drops the bg and the image item and keeps the text, while book-layout/get still returns the stored document; migration: down with 9 rows in the table (29 books untouched) then up -> table recreated empty, schema as in the file.
  AC6: integration TestAdminMediaReadPath (passed): admin reads a design image referenced as image item or bg (200), not an unreferenced one, not one named only inside a text imitating "media":"<id>", not other-class/unlinked/removed-member images; access ends at once when the layout drops the image or is reset, starts at once when it references it. Other callers 404.
  Rate limits: 20 saves ok then 429 with Retry-After 10 and ERROR_RATE_LIMITED (burst); invalid saves also spend burst; with 1 save per 0.6 s the 61st in the minute got 429 (Retry-After 24). Counter smem_layout_saves_total{ok,conflict,invalid,too_large,rate_limited} all present and increasing; text with a marker sent in 24 saves does not appear in the API log (0 hits).
  NON-BLOCKING observations: (1) Reset restarts the version at 1, so a stale editor can win silently (ABA): with editor X open at version 3, a reset then 3 new saves in another tab bring the stored version to 3 again and X's save with base_version 3 returned 200 and overwrote them (reproduced). It needs reset plus exactly the same number of saves; a stale editor with a different version gets 409 with layout null. Suggest a version that never goes back (for example keep the counter on the yearbook row, or do not delete the row on reset but blank it). Low risk now; matters once the editor autosaves. (2) The stored document is compact, but HTTP JSON responses escape < > & as <, so a 128 KiB layout full of such characters becomes a ~470 KB book-data/get or book-layout/get response (reproduced: 97 KB stored, 472 KB response). Not a limit violation; mention for the print route size expectations. (3) Text is trimmed/normalised on save (leading/trailing spaces removed), so "reads back identical" holds for canonical input only. (4) A trailing whitespace outside the layout value is not counted in the 128 KiB (Go RawMessage); inner whitespace is counted. Harmless.
- 2026-11-01 04:55Z · leader · merged d917d92 (PR #152, head 3aeaf04); QA_PASS: integration 33 pkgs 0 FAIL, newman 295/0, e2e 7/7, print-check 163/0; version-reset ABA follow-up is T-156

### T-154 — [E09] Web layout runtime: the book draws a layout (Memory Book)
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** T-153
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-154-e09-web-layout-runtime-the-book-draws-a-
- **PR:** https://github.com/danyaa666/smemories/pull/153
- **Updated:** 2026-11-01 06:43Z by leader
- **Comments-seen:** 5

Spec: .team/epics/E09-customisation-editor/13-web-layout-runtime.md (read it from the main checkout). place/unplace geometry, Sheet layer slot, bg/overrides/items rendering with FitText, notes pages, parity test on A5/A4/Letter, class book.

#### Comments
- 2026-11-01 04:55Z · leader · T-153 merged (d917d92)
- 2026-11-01 05:28Z · dev · PR #153 head 615d38a. Layout runtime in web/src/book/layout (geometry, Sheet layer/chrome slots, bg/overrides/items, FitText, notes repeat), manifest editable ids corrected, print-check 170/170 pass (7 new: parity A5/A4/Letter, no-layout identical, theme restyle, perf +1.8% for 40 items, class book). GAPS: (1) class-book/get does not carry layout; web reads member.layout when present; server side needs a task and a decision (8 MiB cap, privacy). (2) make e2e NOT run by dev (permission denied for the shared stack); QA to run make e2e + class journey. (3) print-check mock was broken since the YearbookLayout change (CI paused); repaired in tools/print-check/lib.mjs. (4) notes-page parity: whole page 0.16-0.72% (template ruled-line noise 0.31-0.56% without layout), layout adds <0.5%; text size drawn in design px. Verify: make print-check, make e2e, web lint/tsc/vitest.
- 2026-11-01 05:29Z · leader · Leader note for QA: AC4 is narrowed in the spec (web side only, fixture-tested); the server does not send class-book layouts yet, a later task. Dev could not run make e2e (permission denied on the shared stack): QA must run make e2e and the class journey on your own compose project, plus make print-check (dev: 170 pass/0 fail, repaired tools/print-check/lib.mjs mock). Also review that repair and the manifest editable-id corrections (cover.belongs, back.subtitle removed).
- 2026-11-01 06:39Z · qa · QA_PASS on PR #153 head 615d38a (PR checks go/web/security green; QA then pushed d3db7a1 = tests only, +10 print-check tests in tools/print-check/check.test.mjs). Own compose project t-154 on ports 13306/16379/19000, own print-check port 5299; main stack untouched.
  RESULTS (all on 615d38a): web eslint+prettier clean, tsc clean, lint:i18n ok, check:api ok, vitest 40 files/732 tests pass, vite build ok. go test ./... all ok incl internal/templates and internal/booklayout. make test-integration: 33/33 packages ok (needed SMEM_TEST_S3_ENDPOINT=http://127.0.0.1:19000: the tests default to 127.0.0.1:9000 = main stack). make print-check: tests 170, pass 170, fail 0 (7 new layout tests included; with my 10 extra tests the suite is 180, all pass). make e2e: 7 passed (25.6 s): class journey, customisation journey, M1 journey, h2 gallery x4.
  AC1 PASS: print-check parity on A5/A4/Letter: whole-page difference A5 cover 0.149% (0.167% without layout), notes 0.716%/0.289% (0.559/0.307 baseline), A4 0.054/0.163/0.351, Letter 0.047/0.178/0.172; each item corner within 2 px; page sizes exact; fonts embedded; missing asset ghost01 and gone image ghost02 draw nothing. I looked at the Letter PDF cover by eye: items where placed, clamped inside the page, nothing cut. Hostile layouts (my tests, A5 and Letter): HTML/script text printed as literal text and no script runs (window.__pwned stays unset); 20000-rune text in 100x40 and 1000-tall boxes; 40 overlapping 300x300 rotated hearts + 40 rotated texts at the page edge; degenerate (w=0, bad ids, unknown font/colour, size 5000, boxes bigger than the page, bg of a gone image / unknown asset / unknown colour token, overrides for unknown ids, unknown page kind): all give the same page count as the book without a layout, no page error, no sheet scrolls (layer clipped), sizes exact, fonts embedded. Ready waits for images: with a 1200 ms media delay every cover img is loaded at data-book-ready.
  AC2 PASS: I printed the same fixture book (3 notes) on develop 15e75de and on PR head, memory-book, navy-classic and basic x A5/A4/Letter = 9 PDFs, rasterised every page at 96 dpi and compared the bytes: 0 differing pages of 42. Existing print-check cases and vitest DOM snapshots unchanged. (Editable adds no wrapper without a layout.)
  AC3 PASS: FitText with the book fit rule (fits.clipped=overflow, longtx not overflow); theme+font set switch tested on lavender/playful (Baloo 2, rgb 59,47,74), peach/classic (Cormorant Garamond, 74,44,34), mint/classic (31,59,52); tinted asset takes accent of the theme; size is design px (fontSize item.size in the 816 px box); bg uses media variant display (print-check asserts DESIGN_IMAGE:display for the bg and :print for the image item).
  AC4 PASS (narrowed): print-check class book test: 6 students, layout on student 0 appears on exactly 1 PDF page, that student's own profile page; student 3 on navy-classic (no layout block) draws none; class book without layouts is unchanged (existing class cases + class e2e journey green); vitest classBook.test.tsx green. Server does not send member.layout yet (known gap).
  AC5 PASS: see RESULTS. Manifest ids reviewed: cover.belongs and back.subtitle removed (they are text ids inside the cover header / the Title block, not positionable elements); remaining 8 ids each wrapped in Editable (data-layout-id) in memory.tsx and pinned by tests; go templates tests ok. Mock repair in tools/print-check/lib.mjs reviewed: adds yearbook/get and note/get-list for the YearbookLayout shell and the display variant for the design image only; sound. Perf: 40 extra items 276 ms vs 281 ms (+1.8%).
  NON-BLOCKING: (1) tools/print-check startServer accepts ANY server already on the port: a stale vite from the T-146 worktree (pid 2259, started Oct 30, not mine, left running) sits on :5199 and made my first default run test T-146 code (5 of 7 layout tests failed). Fix idea: startServer should refuse an occupied port or check the server's root; for now run with PRINT_CHECK_PORT=<free port> and kill that vite. (2) Parity is baseline-relative (diff minus the same page without layout < 0.5%); the whole-page number on A5 notes is 0.716% because the template's ruled lines are 0.559% on their own. I agree with the method, the spec's wording is looser. (3) docs/e2e.md and templates.md say 170 print-check tests; with the 10 hostile tests I added it is 180. (4) the notes-kind items repeat with the same fit key layout:notes:<id> on every notes page, so the fit report holds one entry for them (report only). (5) Integration tests default the S3 endpoint to 127.0.0.1:9000, i.e. whatever runs there.
- 2026-11-01 06:43Z · leader · merged 78f730b (PR #153, head d3db7a1); QA_PASS: print-check 180/0, e2e 7/7, integration 33 pkgs, vitest 732; AC4 web-side only

### T-155 — [E09] Fix three stale Postman assertions in the customisation-editor collection
- **Status:** MERGED
- **Priority:** P3
- **Type:** test
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-155-e09-fix-three-stale-postman-assertions-i
- **PR:** https://github.com/danyaa666/smemories/pull/151
- **Updated:** 2026-10-31 15:37Z by leader
- **Comments-seen:** 3

Spec: .team/epics/E09-customisation-editor/14-fix-stale-book-design-newman-steps.md (read it from the main checkout). Steps 17 and 18 assume navy-classic is not customisable (false since T-145)

#### Comments
- 2026-10-31 15:32Z · dev · Postman-only: steps 17 (own defaults: options null, design.themes[0]=navy) and 18 (lavender on navy-classic, still 400 ERROR_INVALID_DESIGN). Whole collection run twice on own compose project t155dev: 132 assertions, 0 failed (3 failed before). Run make reset-limits between runs (register rate limit). Head 1a78b64.
- 2026-10-31 15:35Z · qa · Head 1a78b64 (PR #151). AC1: diff vs origin/develop touches only postman/customisation-editor.postman_collection.json (4 ins, 4 del; steps 17 name+assertion, 18 name+body). Fresh own compose project t155qa (mysql 13307, redis 16380, s3 19001, API 18080 built from head, SMEM_ENV=dev, migrations applied, users verified), make-reset-limits equivalent (rl keys deleted) before each run: newman run postman/customisation-editor.postman_collection.json twice back to back -> run1 requests 62/0 failed, test-scripts 62/0 failed, assertions 132 executed/0 failed; run2 identical 132/0. (A first run before verifying the two newman users showed ERROR_EMAIL_NOT_VERIFIED in steps 5/8; that is the documented setup step, not a defect.) AC2: step 18 still POSTs options.theme=lavender on navy-classic and asserts status 400 + ERROR_INVALID_DESIGN (both passed); lavender is a memory-book-only theme (embed/memory-book.json), so the refusal is real; step 17 now asserts template_id navy-classic, options null, design.themes[0]=navy (passed). Nothing deleted or weakened; no other assertion changed. Only PR CI pending for go/web at check time, security pass; e2e/integration/image skipped by design; no code/Go touched so make test-integration not applicable (Risk low, postman-only). Stack torn down (own project only).
- 2026-10-31 15:37Z · leader · merged b2bc66e (PR #151, head 1a78b64); QA_PASS: collection 132 assertions, 0 failed, twice

### T-156 — [E09] Layout version must never go back (reset / template change)
- **Status:** MERGED
- **Priority:** P2
- **Type:** fix
- **Milestone:** M4
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-156-e09-layout-version-must-never-go-back-re
- **PR:** https://github.com/danyaa666/smemories/pull/154
- **Updated:** 2026-11-01 06:55Z by leader
- **Comments-seen:** 4

Spec: .team/epics/E09-customisation-editor/15-layout-version-never-goes-back.md (read it from the main checkout). QA ABA finding on T-153: reset restarts the version, so a stale editor can overwrite newer work

#### Comments
- 2026-11-01 06:24Z · dev · PR #154 head 8b3c25d. Counter on yearbook_tab.layout_version (migration 20261101130000, backfilled from layout rows); save compares base to it under the row lock, reset and template change delete the row and +1. After reset get -> layout null, version N+1; 409 carries current version. Tests: TestVersionNeverGoesBack (AC1-3), TestMigrateYearbookLayoutVersion (AC4), existing reset/template tests updated. Own compose t156: go test -race -tags integration ./... all ok, make lint 0, check:api ok, class-matrix ok, newman customisation-editor x2: 135 req/295 assert/0 failed. Not run: e2e/print-check (no web/print change). Note: reset on a book with no layout also bumps the version (harmless). Deploy migrate+code together.
- 2026-11-01 06:25Z · leader · Leader note for QA: dev did not re-run the full integration suite after fixing TestMigrateBookLayoutTable (only TestMigrate*), and did not repeat the HTTP reproduction on a live stack. Run the full make test-integration equivalent, the live ABA reproduction (reset and template-change variants), migration up/down with existing rows (versions backfilled from layout rows), class matrix and newman, and quote result lines with the head SHA.
- 2026-11-01 06:54Z · qa · QA_PASS on PR #154 head 8b3c25d (CI go/web/security pass; mergeable). Own compose project qa156 on ports 23306/26379/29000 (SMEM_TEST_S3_ENDPOINT set), torn down after.
  RESULT LINES (head 8b3c25d): go test -race -count=1 -tags integration ./... = 33 pkgs ok, 0 FAIL (booklayout 73s, bookdata 46s, db 195s); go test -race ./... = 32 ok; make lint = 0 (gofmt, vet x2, golangci, eslint, tsc, i18n, check:api 'api schema ok', check-assets); scripts/check-class-matrix.sh ok (24 ops); newman customisation-editor x2 (limits reset between, --working-dir postman) = 135 req / 295 assert / 0 failed, both runs; e2e 7/7 passed; print-check 163/163, 0 fail.
  AC EVIDENCE (live API on qa156, my own script, SMEM_ENV=dev): AC1 X at v3, reset -> get layout null version 4, 3 saves elsewhere -> 7, X save base 3 -> 409 {layout, version:7}, stored untouched. AC2 template variant (memory-book is the only customisable template, so memory-book -> navy-classic -> memory-book): get version 4 then 5 layout null; 3 saves -> 8; X base 3 and base 4 -> 409 carrying version 8; base 8 -> 200 (9). AC3 fresh book get = version 0/no layout, first save 1; sequence save x3 / reset / save x3 / reset / save / template change = 1,2,3,4,5,6,7,8,9,10 strictly increasing. Concurrency: 8 parallel saves with the same base + a reset: exactly one winner at most, never a double 200; no row has book_layout_tab.version != yearbook_tab.layout_version. AC4 migration on a real DB: up -> column BIGINT NOT NULL DEFAULT 0; down -> column gone, layout rows and yearbooks kept, version back to 20261101120000; with rows (A layout v7, B no layout, C layout v1) up backfilled A=7, B=0, C=1; down then up again re-backfills the same. Yearbook delete: layout get 404, 0 orphan book_layout_tab rows. Reset on a book with no layout: 200, version 0 -> 1, layout null, harmless. 409 carries current version (7, 8). AC5 all lines above.
  MIGRATION SAFETY: expand-only (one NOT NULL DEFAULT 0 column + backfill UPDATE; old code ignores it, old inserts get 0), so it can run before the code. Caveat (non-blocking): old code running between migrate and deploy keeps bumping only book_layout_tab.version and its reset restarts at 1, so a book saved or reset in that window can have a stale counter; the new code then just uses the counter (self-consistent, the ABA protection starts from the counter). Nothing is shipped to users yet (editor T-154 unreleased), so migrate+code together as dev says is fine.
  NON-BLOCKING OBSERVATIONS: (1) A reset of a book with no layout moves version 0 -> 1, so an editor that read version 0 and then saves base 0 gets one harmless 409 and must reload (spec allows). (2) book-data/get layout_version stays 0 when layout is null (documented print payload), while book-layout/get returns the real counter; T-154 must take base_version from book-layout/get, not book-data/get. (3) After a template change to a non-customisable template save returns 400 template_not_customisable before any version compare (fine). (4) 429 from layout save limit (60/min) seen only under my 9-thread burst.
- 2026-11-01 06:55Z · leader · merged 7adc7af (PR #154, head 8b3c25d); QA_PASS: integration 33 pkgs 0 FAIL, newman 295/0 x2, e2e 7/7, print-check 163/163, live ABA repro 409 both variants

### T-157 — [E09] print-check: refuse a foreign server on its port; refresh test counts in docs
- **Status:** MERGED
- **Priority:** P3
- **Type:** test
- **Milestone:** M4
- **Depends-on:** T-154
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-157-e09-print-check-refuse-a-foreign-server-
- **PR:** 155
- **Updated:** 2026-11-01 07:04Z by leader
- **Comments-seen:** 4

Spec: .team/epics/E09-customisation-editor/16-print-check-start-server-strict.md (read it from the main checkout). QA found print-check silently tested a stale vite on :5199

#### Comments
- 2026-11-01 06:43Z · leader · T-154 merged
- 2026-11-01 06:55Z · dev · Head 2646dc6. print-check picks a free port itself; foreign server on PRINT_CHECK_PORT fails fast (new server.test.mjs, plus real stale vite on 5199 tried, left alive). make print-check 181/181 pass; web lint/tsc/vitest green. QA: run make print-check with a dummy server on 5199 and with PRINT_CHECK_PORT=5199.
- 2026-11-01 07:03Z · qa · Head 2646dc6. AC1: PRINT_CHECK_PORT=5199 (stale vite pid 2259, IPv6-only, left alive) -> exit 1 'port 5199 is held by another process (print-check never tests a server it did not start)' in 0.05 s; same with python dummy on 5288, dummy still 200 after. Default port is now OS-picked free port, so a foreign server on 5199 is avoided (run used e.g. 50669). Nothing killed (pids 2238/2259 still alive). AC2: make print-check -> tests 181, pass 181, fail 0 (430 s), includes new server.test.mjs (ok 56). stop(): started+stopped own vite on port 50699 -> down after stop; after full run only the foreign 2238/2259 vite remain, no orphan. AC3: docs/e2e.md says 181 (+ 'run make print-check'), docs/templates.md 'about 180' (real 181, fine). Diff: only docs/e2e.md, docs/templates.md, tools/print-check/{fidelity.mjs,lib.mjs,package.json,server.test.mjs}; nothing under .team/, web/ or product code, so web lint/tsc/vitest not applicable. CI on PR: go, web, security pass; others skipped. Non-blocking: message names the port but not the holding process (spec said 'port and the process' only for the reuse option); a foreign server that accepts TCP but never answers would make the pre-check fetch wait on Node's default timeout.
- 2026-11-01 07:04Z · leader · merged 4c9b0f6 (PR #155, head 2646dc6); QA_PASS: print-check 181/181, foreign server on port refused, no orphan vite

### T-158 — [E09] Editor state: document store, undo/redo, autosave and conflict handling (web)
- **Status:** QA_FAIL
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** T-156
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-158-e09-editor-state-document-store-undo-red
- **PR:** 156
- **Updated:** 2026-11-01 07:11Z by qa
- **Comments-seen:** 4

Spec: .team/epics/E09-customisation-editor/17-editor-state-undo-autosave.md (read it from the main checkout). Pure operations, snapshot history depth 100, debounced autosave with base_version, 409 choices

#### Comments
- 2026-11-01 06:55Z · leader · T-156 merged 7adc7af; spec notes base_version source
- 2026-11-01 07:02Z · dev · PR #156 head 6525f4a. web/src/book/editor/ (document, history, store, useLayoutEditor, SaveStatus). Verify: states per req 4-6 in store.ts, base_version only from book-layout/get, tests (vitest src/book/editor, 69). Changed shared api/client.ts: ApiError.data (409/400 bodies). Live check (2 tabs, offline) is QA's per spec; no UI route exists yet. AC4 = server boundary table replayed (no shared fixture file exists).
- 2026-11-01 07:11Z · qa · QA_FAIL T-158 (PR #156, head tested 6525f4a; QA pushed test-only commit 2b0dfe2 on top). One blocker-class AC4 issue, one minor. Rest verified, see below.
  
  ISSUES
  1. [major, AC4] The client mirror accepts text the server refuses. Repro (live, my own stack): addItem/setText("cover", ...) with "a​b" (also U+200E, U+FEFF, U+2028, U+2029, U+FFFD) returns a doc with no refusal; autosave then gets 400 ERROR_INVALID_LAYOUT, problems [{"path":"pages.cover.items[0].text","code":"bad_text"}], editor goes to `invalid` and stays there until the user undoes. Zero-width/LRM chars are common in pasted text. Cause: document.ts textRefusal() only rejects C0/C1 controls; internal/textx CleanMultiline rejects Cc (except \n), Cf (except ZWJ U+200D), Zl, Zp and U+FFFD. Also the server counts code points AFTER NFC + TrimSpace; the client counts raw: 300 decomposed Vietnamese chars (600 code points, 300 after NFC) and 500 letters + 3 trailing spaces are refused client-side though the server accepts (conservative, but a Vietnamese paste of NFD text is refused wrongly). Fix: in textRefusal use /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}�]/u (allow \n and ‍; keep \r\n -> \n), count [...text.normalize("NFC").trim()]. Test: I added web/src/book/editor/document.mirror.test.ts (8 cases, 7 fail now; they must pass; the dev replay of validate_test did not cover Unicode categories, which is why this slipped). Also fine to leave size/font/color/align/asset/media to the server (they need the manifest; live: bad font gave 400 with path pages.cover.items[0].font unknown_id, state invalid, correct).
  2. [minor, req 6] A commit while a 429 (or backoff) wait is pending triggers save() through the 1.5 s debounce, which clears the retry timer and sends at once, before Retry-After. Live: 429 after 21 rapid saves; edit during the wait -> one extra request 1.5 s later (429 again), then recovery at ~9.5 s. Same for the `online` event (fine) and for offline backoff (a commit shortens the wait). Suggest: remember `notBefore = now + wait` and have schedule()/save() defer to it (still allow flush() on hide/online/stop).
  3. [minor, non-blocking] keepMine()/useTheirs() with no conflict body call getBookLayout without a catch (unhandled rejection if offline). Unit tests do not cover it.
  
  VERIFIED (head 6525f4a, worktree T-158/web)
  - eslint 0 problems; prettier --check clean; tsc --noEmit clean; vitest 44 files, 802 passed (802); lint:i18n "i18n check ok"; check:api "api schema ok"; npm run build ok. CI on 6525f4a (run 41283165763): go, web, security success; e2e/image/go-integration skipped (paused, D-37/D-38).
  - Diff vs develop: 15 files, web only (api/bookLayout.ts, api/client.ts + test, book/editor/*, locales en/vi). No server, no .team, no new dependency. go test not needed: no Go change. client.ts: ApiError gains `data` (body.data); the only reader is the editor store; grep shows no caller logs, renders or JSON.stringify's an ApiError, no console/Sentry in web/src, so no leak or behaviour change for other callers (constructor arg is last and optional).
  - Code review of store.ts vs req 3-6: one save in flight + one queued (inFlight/queued), conflict/gone/invalid never saved (dirtyAndSavable + schedule), base_version only from getBookLayout and own save answers, flush on visibilitychange hidden / online / stop, leave guard via useLeaveGuard(dirty) (beforeunload + in-app), reset bumps epoch.
  - Live check (own compose project qa158, ports 13307/16380/19010, API :18158 SMEM_ENV=test, real MySQL/Redis/S3, real fetch; store driven from a throw-away vitest script outside the repo; two stores = two tabs, same user): (a) load v1, edit, autosave -> saved v2, undo/redo each saved, second client loads identical doc and version PASS. (b) conflict from second client, useTheirs: A kept its local doc while in conflict, edits+flush in conflict sent no save (save count +1 only), useTheirs -> B's doc, history cleared, version == server, no further saves PASS. (c) same, keepMine: server ends with A's two items at A's version, then stale B gets 409 in turn (no silent overwrite) PASS. (d) offline -> save attempts at +1502, +2503, +4503, +8504 ms (1.5 s debounce then 1, 2, 4 s... spaced) with doc dirty; edit while offline; reconnect -> saved, a fresh client sees both items PASS. (e) 400: problem path in state, no re-send until next commit, undo recovers PASS (except issue 1). (f) 404 after yearbook/delete: gone, no retry (1 request), further edit sends nothing PASS. (g) real 429 (20 saves/10 s limit hit on the 21st): state stays saving, recovers after Retry-After (~9.5 s) PASS, early extra request = issue 2.
  - AC1 pass (suite + checks above). AC2 pass (store.test "30 edits, 10 undos, 5 redos and a conflict" present and green; also live conflict). AC3 pass (unit "never two saves in flight", live: no save in conflict/invalid). AC4 FAIL as described in 1; box limits (x/cy/w/h/r, ratio 1419, 40/150 items, id 6..26 alnum, 500 cap) match validate.go constants, checked line by line. A shared fixture does not exist; a replay of the Go table is acceptable for boxes, not for text categories.
  - Not applicable: Postman (no endpoint change); make test-integration / e2e / print-check not run: web-only change with no server, print or auth flow touched (risk low).
  Re-test after fix: run document.mirror.test.ts + the suite; I will re-run live scenarios (e) and (g).
- 2026-11-01 07:11Z · leader · Leader to dev (rework 1): fix all three QA findings, not only the blocker. (1) textRefusal in document.ts must match the server's text validation exactly: read internal/booklayout validate.go for the refused categories (zero-width/format characters, U+2028/2029, U+FFFD, control chars) and the order (NFC and trim first, then count code points); make QA's document.mirror.test.ts (commit 2b0dfe2, pushed on the PR branch; merge it in, no force-push) pass 8 of 8, and add a comment in document.ts naming validate.go as the source of truth. (2) A commit during 429/backoff must not schedule a save before Retry-After/backoff has passed (the debounce must wait for max(1.5 s, remaining wait)); add a fake-timer test. (3) keepMine and useTheirs must catch a refetch that returns no conflict body (move to a clear error/offline state, no unhandled rejection); add a test. Re-run web lint/prettier/tsc/vitest/lint:i18n/check:api/build.

### T-159 — [E09] Editor route and shell: page strip, zoom, selection overlay, property panel (web)
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** T-158
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-11-01 08:00Z by leader
- **Comments-seen:** 0

Spec: .team/epics/E09-customisation-editor/18-editor-shell-and-selection.md (read it from the main checkout). Route /yearbooks/:id/editor, overlay outside the page clip, panel numeric edits, e2e journey

#### Comments

### T-160 — [E09] Editor gestures: move, resize, rotate, snap, guides, with keyboard control (web)
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M4
- **Depends-on:** T-159
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-11-01 09:00Z by leader
- **Comments-seen:** 0

Spec: .team/epics/E09-customisation-editor/19-editor-gestures-and-keyboard.md (read it from the main checkout). In-house pointer-event module per ADR 0004 Q2, keyboard in the same task, 44 px touch targets

#### Comments

<!-- tasks:end -->
