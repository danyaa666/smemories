# Stage 2 threat model: the free layout, history and own templates (E09, T-179)

One page. Stage 2 lets a student send a JSON document (the layout of a book, 128 KiB), keeps 30 snapshots of it per book and lets them save pages as
own templates (20, 64 KiB each). Every one of those is untrusted input that is parsed, stored and later drawn. Limits and measured sizes:
[layout-storage-budget.md](layout-storage-budget.md); format: [templates.md](templates.md); class reads: [class-privacy-review.md](class-privacy-review.md).

## Assets
1. The student's text and image ids inside the layout, snapshots and templates (personal, may name friends or teachers).
2. The layout document as a parser input and as the thing the print route draws (an attacker's document must not break either).
3. History (30 snapshots per book) and own templates: they hold what the student deleted or changed, so they are as private as the layout.
4. Capacity: database storage, CPU of the API, the rate budget of a user.

## Trust boundaries
- **Student (owner)**: writes the document through `book-layout/*`; trusted with their own book only, not with the size or shape of what they send.
- **Other student / anonymous**: no access at all. Another user's book, snapshot, template and an unknown id are all `ERROR_NOT_FOUND`; an anonymous call is
  401 before a byte of the body is read; a foreign origin is 403 (origin guard).
- **Class admin**: reads a linked member's layout through `class-book/get` only (read only, re-validated, 8 MiB cap), never history, templates or the write routes.
- **Browser**: draws student text; nothing the student typed may become markup.

## Abuse cases, the control that stops each, the test that proves it
| Abuse | Control | Test |
|---|---|---|
| Deeply nested JSON (100 000 levels) to exhaust the parser | `tooDeep` on the raw bytes (16 levels) in the handler and in `Validate`, before anything is decoded; `save` reads at most 132 KiB | `TestHostileDocumentsFailFast`, `TestHostileBodiesFailFast` (400 `ERROR_INVALID_LAYOUT` `too_deep` in about 2 ms) |
| 2 MB body, 200 000 tiny items, 1 MB string | body cap 132 KiB (413 `ERROR_TOO_LARGE`) and the same limit inside `Validate`; the other write routes 8 KiB | the same two tests; `TestRoutesRefuseAnonymousOversizeAndTooMany`, `TestSizeLimit` |
| 43 000 items inside 128 KiB (the largest decode) | rejected for `too_many`; costs about 12 MB of allocation, under a 24 MiB ceiling | `TestHostileBodiesFailFast` |
| A document that is within 128 KiB sent but longer once stored (NFC growth) | the stored form is checked too (`too_long` at path `""`) | `TestStoredSizeLimitHoldsAfterNormalisation` |
| Documents the validator mishandles: panic, hang, a result that does not validate again, over the limits | six fuzz targets (below) with committed seeds and regression seeds | `FuzzValidate`, `FuzzValidateRestore`, `FuzzStrip`, `FuzzWithPage`, `FuzzRequestDecode`, `textx.FuzzClean` (seeds run in `go test`) |
| Control, NUL, bidi override or isolate, zero-width, line separator, lone surrogate or invalid UTF-8 in a text item or a name | `textx.Clean`/`CleanMultiline` (Cc, Cf except ZWJ, Zl, Zp, U+FFFD refused) | `TestLayoutTextSafety`, `TestTemplateNamesAreText`, `TestControlAndBidiCharactersAreRefused` |
| Composed and decomposed Vietnamese (macOS Telex) counted or stored differently | NFC before the length check; the limit counts stored characters | `TestVietnameseComposedAndDecomposedAreOneText`, `TestLayoutTextSafety`, `TestTemplateNamesAreText` |
| Stored XSS: student text rendered as markup | the server stores text only; the web never turns a string into markup under `web/src/book` (eslint `no-restricted-syntax`: `dangerouslySetInnerHTML`, `innerHTML`/`outerHTML`, `insertAdjacentHTML`, `document.write`) | `npm run lint` (CI job `web`); the rule fails on each of the four |
| Filling the database with maximal books, snapshots and templates | the caps in the budget doc; measured worst case per user 78.75 MiB payload (107 MB on disk) | `TestMaximalUserStorage`; **above the 30 MB threshold, a cap is recommended to the leader and not applied** |
| Hammering the write routes | per-user burst 20 / 10 s and 60 / minute on `save`, `version-list`, `version-get`, `restore`, the four template routes; 429 with `Retry-After`; per user, not shared | `TestRoutesRefuseAnonymousOversizeAndTooMany`, `TestSaveRateLimit*` |
| Layout content leaking into logs, metrics or errors | log lines carry the book's public id, version numbers and counts only; metric labels are closed sets; a problem list is paths and codes (a member name that is long or not plain shows as `?`) | `TestNoLayoutContentInLogsMetricsOrProblemLists` (sentinels in texts, names, image ids; log, `/metrics` text and every refusal body searched) |
| Reading or writing another user's layout, snapshot, template or image | every query is scoped by owner; media ids must be the owner's design images of the book; the origin guard on every write | `TestOwnerOnly`, `TestVersionRoutesAreOwnerOnly`, `TestMediaIdsMustBeOwnDesignImages`, `TestTemplateListOwnAndDelete` |
| A failing apply, restore or create leaving a half-written book, or two tabs losing an update | one transaction under the book's row lock; version compare; nothing written unless everything validates | `TestFailedApplyChangesNothing`, `TestConcurrentSavesExactlyOneWins`, `TestConcurrentAppliesExactlyOneWins` |

## Fuzzing
Native Go fuzz targets; `go test` runs their seeds (QA's hostile cases, the shipped templates, the documents above), `go test -fuzz=<Target> -fuzztime=5m
<package>` explores. Properties: never panics, never takes a second, an accepted document is canonical (validating it again returns the same bytes) and
within the size and item limits, a rejected one has at most 20 problems and no output, `strip` is idempotent and leaves no image, `withPage` writes
exactly the page it was given. A finding is fixed and committed under `testdata/fuzz/<Target>/` as a regression seed. Fuzzing found: `withPage` panicked on a
stored document `null` (not reachable through a validated document). Writing the properties found two more gaps, fixed here: `Validate` itself had no
nesting or size limit (only the route had one), and an NFC-lengthened document could be stored over 128 KiB.

## Deliberately not covered
- **Denial of service by many accounts**: the per-user limits do not stop a thousand accounts. That is for the global rate limiting and the edge (T-031, WAF)
  and the deploy layer; accounts need a verified email, which is the only brake here.
- **Fail-open limits**: if Redis is down the per-user limits are off (by design, they bound effort, not guessing); the body caps and size limits stay.
- **`get` and `reset` have no per-user limit** (T-153 design). `get` is a bounded database read; see the board comment on T-179.
- **Database capacity alerts** and a cap on bytes per user: the recommendation in the budget doc is waiting for the leader's decision.
- A student putting offensive text in their own book: moderation is the class admin's and the owner's, see [moderation.md](moderation.md).
