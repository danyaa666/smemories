# Team Board — SMemories

> **Source of truth for the leader / dev / QA team.**
> **Leader** writes everything here. **Dev and QA** may only change a task's status and add comments (through `board.py`). **You** answer the questions in §3 — edit the `Answer` line in place or just tell the leader.
> Status flow: `BACKLOG → TODO → IN_PROGRESS → READY_FOR_QA → IN_QA → QA_PASS | QA_FAIL → (leader review) → MERGED → DONE`.
> `MERGED` = merged to `develop` and **waiting for your review**; tell the leader "accept T-007" (or `bin/team accept T-007`) to make it `DONE`.

## 0. At a glance

<!-- summary:start -->
| Status | # | Tasks |
|---|---:|---|
| BACKLOG | 21 | T-013, T-014, T-017, T-018, T-019, T-020, T-021, T-022, T-023, T-024, T-025, T-026, T-027, T-029, T-031, T-032, T-040, T-041, T-042, T-044, T-046 |
| TODO | 1 | T-039 |
| READY_FOR_QA | 1 | T-016 |
| IN_QA | 1 | T-034 |
| QA_PASS | 1 | T-045 |
| MERGED | 7 | T-015, T-033, T-035, T-036, T-037, T-038, T-043 |
| DONE | 14 | T-001, T-002, T-003, T-004, T-005, T-006, T-007, T-008, T-009, T-010, T-011, T-012, T-028, T-030 |

**Awaiting your review (MERGED):** T-015 (Web: auth pages and session handling); T-033 (CI: do not cancel in-progress runs on develop and main); T-035 (T-010 follow-ups: template tests iterate templates.List()); T-036 (Bound the memory of image processing (caps, concurrency, memory limit)); T-037 (US Letter page size, end to end); T-038 (Template format v2: backgrounds, static text, rotation, ellipse, font families); T-043 (Note field catalogue (internal/notefields): closed set of form fields with EN/VI labels and validation)

**Open questions for you:** Q-014 (Approve merge of T-045 (register accepts a locale)?)

_Board last written 2026-10-08 09:14Z_
<!-- summary:end -->

## 1. Vision & orientation

**Product.** SMemories is a web app where students design, collect content for, and export their own yearbook. Friends leave messages and photos through a shareable link (no account needed), the owner picks a template and fills in the book's information, and the app exports a print-quality PDF. Class yearbooks (many students, one book) come after the personal flow works. Product description: [`/README.md`](../README.md).

**Users.** Primary: university/college students aged 18+ who own a yearbook. Contributors: friends, teachers, family — anonymous, link-based. Later: class admins (teacher or class monitor). School-age (under 18) account holders are a deliberate later milestone (consent, photo rules).

**Non-goals (until the owner says otherwise).** Ordering printed books in-app; payments; under-18 account holders; free-form drag-and-drop canvas editor in M1 (staged: guided customisation M1b, full editor M4, D-19); video/audio messages; native mobile apps; AI layout.

**Hard constraints.**
- Public GitHub repo (`danyaa666/smemories`): no secrets, ever. `.env*` is git-ignored; only `.env.example` with dev-only values is committed.
- UI in English and Vietnamese from the first screen (i18n from M0).
- Vietnamese diacritics and emoji must survive storage (MySQL `utf8mb4`) and PDF output.
- Dev and QA agents run and test everything **locally with no cloud credentials** (docker-compose MySQL + MinIO). Anything needing AWS or a third-party account is owner-assisted and lives in M2.

**Quality bar.**
- CI green on `develop` — Go: gofmt, `go vet`, golangci-lint, `go test -race`, govulncheck. Web: eslint, `tsc --noEmit` (strict), vitest, production build, i18n key parity.
- New or changed logic has tests that would catch a regression; service and handler packages ≥ 70 % line coverage (reported first, gating later).
- Every HTTP endpoint is in `api/openapi.yaml` and in a Postman collection; every error uses the shared envelope (§5).
- Public (unauthenticated) endpoints are rate-limited; uploads are validated; no personal data in logs.
- Performance: p95 for non-export API calls < 300 ms on the local stack. A 24-page A5 book with 30 photos exports end to end in ≤ 60 s using ≤ 512 MB peak memory.

## 2. Roadmap

| Horizon | Milestone | Goal | Exit criteria ("stable") | Status |
|---|---|---|---|---|
| Now | M0 Foundation | Repo skeleton, local stack, web scaffold, CI, PDF engine proven | see M0 exit below | in progress |
| Now | M1 Personal yearbook, end to end | One student creates a yearbook, collects friends' notes, picks a template, exports a PDF | see M1 exit below | planned (specified) |
| Next | M2 Go live on AWS and harden | Production environment, real email, observability, backups, privacy tooling | sketch only | BACKLOG |
| Next | M3 Class yearbook | Class space, invites, roles, assembling many students' pages into one book | sketch only | BACKLOG |
| Next | M1b Template-driven forms and guided customisation | Forms generated from template fields (started in M1: T-043, T-034, T-044); per-book design copy with colours, fonts, page order and visibility, PDF preview (E09 stage 1) | sketch only | idea (starts when M1 is stable) |
| Later | M4 Free-layout editor | Canva-style editor on the template document: add, remove, move, resize, rotate, replace, assets, undo/redo (E09 stage 2); order against M3 decided at the M2 retrospective | sketch only | idea (D-19) |
| Later | M5+ | Print-shop-ready PDF (bleed, CMYK note), under-18 support, more social providers, in-app print ordering | direction only | idea |

**Epics** (PRDs and task specs live in `.team/epics/`; status only on the board):

| Epic | PRD | Milestone | Goal | Status |
|---|---|---|---|---|
| E01-foundation | [PRD](epics/E01-foundation/PRD.md) | M0, M1 | Repo, local stack, web shell, CI and engineering tooling | active |
| E02-auth | [PRD](epics/E02-auth/PRD.md) | M1 | Accounts and sign-in: password, email verification, Google | active |
| E03-yearbooks | [PRD](epics/E03-yearbooks/PRD.md) | M1 | Yearbooks, profiles and photos | active |
| E04-friends-notes | [PRD](epics/E04-friends-notes/PRD.md) | M1 | Collection links, public note submission, moderation | planned |
| E05-templates-export | [PRD](epics/E05-templates-export/PRD.md) | M0, M1 | Templates, PDF renderer, export job and preview | active |
| E06-go-live | [PRD](epics/E06-go-live/PRD.md) | M2 | Go live on AWS and harden | planned |
| E07-class-yearbook | [PRD](epics/E07-class-yearbook/PRD.md) | M3 | Class spaces, roles, invites and assembly | planned |
| E08-designer-templates | [PRD](epics/E08-designer-templates/PRD.md) | M1 | Claude Design canvases become system templates (US Letter, pilot of two, then six more) | planned |
| E09-customisation-editor | [PRD](epics/E09-customisation-editor/PRD.md) | M1b, M4 | Guided customisation and the Canva-style editor (staged, D-19) | planned (sketch) |

**M0 exit.** From a clean checkout: `make up && make migrate` starts MySQL and MinIO; `make build test lint` is green; `GET /healthz` is 200 and `GET /readyz` reflects the database; the web dev server shows the home page with a working EN/VI switcher and an API status badge; CI is green on a PR; the PDF engine ADR is merged with a go/no-go verdict backed by a Vietnamese-text sample, 300 DPI image test and a memory/time measurement; no secrets in the repo.

**M1 exit (what "stable" means for the first release).** On a clean local stack a new user can, in both English and Vietnamese: register and sign in (email+password; Google against a fake OIDC provider in tests); create a personal yearbook and fill in its information; upload photos; create a notes link and, **as an anonymous visitor**, submit a note with text, emoji and two photos; approve or hide notes; choose between at least two templates; export a PDF (A5, 24 pages, 30 photos) that renders Vietnamese text correctly (no missing glyphs) within the quality-bar budget; download it. An automated end-to-end smoke test of exactly this journey runs in CI. No open P0/P1 bugs, health checklist has no open high-severity item, and every `Risk: high` task is owner-approved.

## 3. Questions & decisions for you

<!-- questions:start -->

### Q-001 — IaC tool for AWS (OpenTofu/Terraform, CDK, or CloudFormation)
- **Status:** RESOLVED
- **Asked:** 2026-10-06 10:13Z
- **Blocks:** T-023
- **Recommendation:** A: OpenTofu / Terraform
- **Answer:** A

**Decision needed:** Which tool defines the AWS infrastructure (Fargate, RDS MySQL, S3, CloudFront) as code?
**Why now / what it blocks:** Only T-023 (AWS IaC and deploy pipeline, M2). M0 and M1 are unaffected, so there is no hurry; answer before M1 is nearly done.
**Constraints:** Solo owner plus AI agents; agents cannot hold long-lived AWS keys (deploys use GitHub OIDC); everything must be reviewable in a PR as a plan/diff; no extra paid service.

| Option | Pros | Cons | Cost / effort | Risk & lock-in | Reversibility |
|---|---|---|---|---|---|
| A (recommended) OpenTofu / Terraform (HCL) | Largest ecosystem and examples; explicit `plan` diff reviewed in PRs; strong module support for VPC, RDS, ECS, CloudFront; agents write HCL reliably | Separate language and state to manage (S3 backend + lock); Terraform itself is BSL-licensed, OpenTofu is the open fork | Free; about 1 task to set up state, 2 to 3 for the stack | Low; HCL is widely portable | Medium: migrating IaC later is real work but possible |
| B AWS CDK (TypeScript) | Same language as the frontend; loops and types; generates CloudFormation | CDK upgrades churn; harder to review the real change (synthesised template); heavier toolchain | Free; similar effort | AWS-only | Medium |
| C CloudFormation / SAM (YAML) | No extra tooling; native | Verbose; weak reuse; slow feedback | Free; more effort | AWS-only | Medium |

**Recommendation:** Option A, because the `plan` output gives you a readable review gate before anything changes in your AWS account, the module ecosystem removes most of the VPC/RDS/ECS boilerplate, and it keeps working if you ever leave AWS. Prefer OpenTofu unless you already use Terraform.
**If undecided:** T-022 (Dockerfile and production config) proceeds; T-023 stays in BACKLOG. Default assumed at the start of M2: Option A.
**Revisit when:** you adopt a platform team standard, or move off AWS.

### Q-002 — Roadmap order after M1: go-live (M2) before class yearbook (M3)?
- **Status:** RESOLVED
- **Asked:** 2026-10-06 10:13Z
- **Blocks:** —
- **Recommendation:** A: M2 go-live first, then M3 class
- **Answer:** A

**Decision needed:** After M1, build "go live on AWS" (M2) first, or the class yearbook (M3) first?
**Why now / what it blocks:** Only the order of M2 and M3. M0 and M1 are the same either way.
**Constraints:** Class yearbooks are your headline use case; a personal yearbook is already usable alone; real users need a production environment.

| Option | Pros | Cons | Cost / effort | Risk & lock-in | Reversibility |
|---|---|---|---|---|---|
| A (recommended) M2 go-live, then M3 class | Real users test the personal flow early; infra, email and backups are proven before you hold a whole class's data; M3 then ships onto a live system | Class mode (the headline feature) arrives later | M2 is mostly owner-assisted infra work | Low | High: roadmap order only |
| B M3 class first, then go-live | Headline feature sooner | You build a larger, privacy-heavy feature with no production feedback; go-live is delayed and riskier | Larger M3 before any users | Medium | High |

**Recommendation:** Option A, because class mode multiplies the personal data you hold (many students, many photos) and I would rather prove deployment, email, backups and deletion with a small user base first.
**If undecided:** Roadmap stays M2 then M3; tasks T-026 and T-027 remain in BACKLOG.
**Revisit when:** a school or class pilot gets a date.

### Q-003 — Approve merge of T-004 (CI pipeline)?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 07:07Z
- **Blocks:** T-004
- **Recommendation:** approve
- **Answer:** _(pending)_

**Decision needed:** Approve the merge of T-004 (CI pipeline) into develop?
**Why now / what it blocks:** CI is Risk: high (workflow files run with repository permissions). It blocks nothing technically, but every later PR gets automatic checks only after it merges, and branch protection needs its check names.
**Evidence:** QA_PASS and my review at 0482b75. The PR's own run is green on GitHub (go, go-integration, web, security). QA proved each job fails on the right defect. No secrets, actions pinned by SHA, minimal permissions.
**Recommendation:** approve (`bin/team approve T-004`).
**After merge (you):** turn on branch protection for `develop` and `main`: require pull requests, no force-push, and the four checks go, go-integration, web, security. Dependabot starts only once this file reaches `main` (promote develop to main when you are ready).

### Q-004 — Approve merge of T-006 (email + password sign-in)?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 07:07Z
- **Blocks:** T-006
- **Recommendation:** approve
- **Answer:** Owner ran bin/team approve T-006 (2026-10-07 07:45Z). Merge waits for the gosec fix, QA re-pass and green CI, per the comment on T-006.

**Decision needed:** Approve the merge of T-006 (email + password sign-in) into develop?
**Why now / what it blocks:** Authentication is Risk: high. It blocks T-007 (email verification and reset), T-008 (yearbook API), T-011 (Google sign-in) and everything user-owned in M1.
**Evidence:** QA_PASS twice (adversarial testing on a throwaway DB: timing, CSRF, rate limits, spoofed headers, parallel logins, injection, session replay, log search) and my own review of the full auth path at 77924c1: no vulnerability found. NFKC passwords and NFC emails and names are in (L-09), so Vietnamese users are not locked out by different devices.
**Known limits (acceptable now):** rate limits are per process (fine for one API task; M2 hardening T-031 moves them to the edge), registration reveals "email already registered" (a deliberate UX trade-off, bounded by the per-IP limit).
**Recommendation:** approve (`bin/team approve T-006`). I will merge T-004 first so this PR runs through the new CI before it lands.

### Q-005 — CAPTCHA on the public friends' note form?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 11:28Z
- **Blocks:** —
- **Recommendation:** A: no CAPTCHA now, hook in place
- **Answer:** A: no CAPTCHA now, verifier hook in place (owner, chat 2026-10-07)

**Decision needed:** Should the public note form (friends sending a message and photos through a link, no account) have a CAPTCHA?
**Why now / what it blocks:** It does not block T-012 or T-034: the endpoint ships with a no-op verifier hook either way. It decides whether the web form (T-018) shows a challenge.
**Constraints:** The form is used by friends on phones, often in a hurry; it accepts text and up to three photos; it is the only unauthenticated upload in the product. Protections already specified: unguessable link, per-IP and per-link rate limits, 300 notes per link, size caps, every note pending until the owner approves it, a honeypot field, a verified-email owner.

| Option | Pros | Cons | Cost / effort | Risk & lock-in | Reversibility |
|---|---|---|---|---|---|
| A (recommended) No CAPTCHA now, hook in place | No friction for friends; nothing extra to configure; the existing limits already bound the damage to one link | A leaked link could be spammed up to the caps (the owner only has to hide or revoke) | None now | Low: worst case is junk notes on one book | Easy: switch it on later without changing the API |
| B Cloudflare Turnstile from the start | Blocks most bots; free; mostly invisible to humans | Adds a third party that sees visitors; needs a site key and secret; can fail on some phones or privacy browsers | About one small task plus an owner setup step | Vendor dependency | Easy to remove |
| C hCaptcha / reCAPTCHA | Familiar | More friction, more tracking, privacy concerns for a student product | Same as B | Vendor dependency | Easy to remove |

**Recommendation:** Option A, because the damage from abuse is bounded (caps, pending-by-default moderation, revocable links) and friction on the contributor path is the larger product risk. Revisit with Option B the first time a link is actually spammed.
**If undecided:** Build with the hook and no CAPTCHA (Option A).
**Revisit when:** a collection receives spam, or the product opens to a public (non-link) submission form.

### Q-006 — Approve merge of T-010 (PDF renderer)?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 11:38Z
- **Blocks:** T-010
- **Recommendation:** approve
- **Answer:** Approved by the owner in chat 2026-10-07; T-010 merged 357e78a.

**Decision needed:** Approve the merge of T-010 (template system and PDF page renderer) into develop?
**Why now / what it blocks:** Risk: high because it adds the core PDF dependency and handles untrusted text and photos. It blocks the export job (T-014) and the template picker (T-019), the last big pieces of the M1 journey.
**Evidence:** QA_PASS with 16 generated PDFs checked (pdfcpu strict, 300 dpi PNGs by eye, Vietnamese text exact, emoji drawn as outlines and never panic, cover photos full-bleed, pagination for 0/1/7/60 notes, hostile inputs bounded); my own read of the text, image and entry-point code found no defect; all four CI jobs passed on the previous head and are finishing on the current one (QA added one test-only commit).
**Known limits (accepted):** emoji print as monochrome outlines, not colour (ADR 0002); byte-identical output only when photo widths differ (documented); a book with no notes still gets one empty notes page. The export job must set a deadline because the renderer itself has no caps.
**Recommendation:** approve. I merge only after the CI jobs on the current head are green. Approve in a terminal: `cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-010`

### Q-007 — Approve merge of T-007 (email verification and password reset)?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 17:13Z
- **Blocks:** T-007
- **Recommendation:** approve
- **Answer:** _(pending)_

Leader review and QA both passed on PR #18 (head 3f6c9aa, CI green). It is high risk (auth tokens), so it needs your approval. New required prod setting: SMEM_PUBLIC_BASE_URL. Non-blocking findings are recorded in the specs of T-015, T-021 and T-031. Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-007

### Q-008 — Approve merge of T-009 (photo upload and storage)?
- **Status:** RESOLVED
- **Asked:** 2026-10-07 17:42Z
- **Blocks:** T-009
- **Recommendation:** approve
- **Answer:** _(pending)_

Leader review and QA both passed on PR #19 (head 21ba7aa, CI green). High risk (file uploads, personal data, CI change, new AWS S3 SDK dependency approved earlier in L-01). Merge T-007 first (migration 0005), then this one (0006). Known limit: memory use of image processing is high; follow-up T-036 fixes it before anonymous uploads (T-034). New settings: SMEM_S3_* and SMEM_MEDIA_* (copy from .env.example). Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-009

### Q-009 — Approve merge of T-011 (Google sign-in)?
- **Status:** RESOLVED
- **Asked:** 2026-10-08 02:00Z
- **Blocks:** T-011
- **Recommendation:** approve
- **Answer:** _(pending)_

QA passed and my review is clean (head 7ffdb68, CI green). High risk (sign-in, account linking, new libraries go-oidc and oauth2, approved in the spec). It is being sent back to dev only to merge develop (T-009 landed); your approval stays valid unless the code changes beyond the merge. Google is optional and off by default: nothing changes until SMEM_GOOGLE_* is configured (real Google credentials are a later owner step). Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-011

### Q-010 — Approve merge of T-012 (collection links)?
- **Status:** RESOLVED
- **Asked:** 2026-10-08 02:24Z
- **Blocks:** T-012
- **Recommendation:** approve
- **Answer:** _(pending)_

QA passed and my review is clean (head 231955e, CI green); it already merges cleanly into develop (T-011 is in). High risk: a public endpoint backed by a bearer-secret link (192-bit token, only its SHA-256 stored, shown once, revocable, rate-limited). Notes themselves come later (T-034). Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-012

### Q-011 — Approve merge of T-033 (CI: do not cancel runs on develop and main)?
- **Status:** RESOLVED
- **Asked:** 2026-10-08 03:21Z
- **Blocks:** T-033
- **Recommendation:** approve
- **Answer:** _(pending)_

QA passed and my review is clean (head 74346f4, CI green). High risk because it changes the CI workflow, but the change is small: pushes to develop and main get one concurrency group per commit (never cancelled), pull requests keep cancelling superseded runs, and the integration job's mysql:8.4 image is pinned to its digest. Today 8 of the last 15 develop runs were cancelled, so merge commits are not getting full CI. One caveat: nobody could test a real double push; I will verify with the first pushes after merge. Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-033

### Q-012 — Vietnamese labels for the friends' note form (T-043): OK to merge?
- **Status:** RESOLVED
- **Asked:** 2026-10-08 03:43Z
- **Blocks:** T-043
- **Recommendation:** Accept the table in PR #24, with one wording change: 'Chúng mình quen nhau thế nào' instead of 'Chúng ta quen nhau thế nào'
- **Answer:** _OK_

The table of EN/VI labels and hints is in the description of https://github.com/danyaa666/smemories/pull/24 (9 fields). Please reply with 'OK' or the wording you want; labels are plain text in code, so they can be changed any time without touching stored notes (field ids never change). Merging T-043 unblocks the public notes submission (T-034).

### Q-013 — Approve merge of T-037 (US Letter page size)?
- **Status:** RESOLVED
- **Asked:** 2026-10-08 03:49Z
- **Blocks:** T-037
- **Recommendation:** approve
- **Answer:** _OK_

QA passed and my review is clean (head 9546d3b, CI green, merges cleanly). High risk because it includes a database migration (0009): it adds Letter to the yearbook page_size enum; existing books are untouched, and rolling back turns Letter books into A5. Nothing in the web UI offers Letter yet (the yearbook form comes with T-016). Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-037

### Q-014 — Approve merge of T-045 (register accepts a locale)?
- **Status:** OPEN
- **Asked:** 2026-10-08 08:52Z
- **Blocks:** T-045
- **Recommendation:** approve
- **Answer:** _(pending)_

QA passed and my review is clean (head 8a9cafb, CI green). High risk only because it touches the register endpoint contract (one optional field 'locale', en or vi, default en). Fixes the Vietnamese UI sending an English verification email. Command: cd /Users/unisoft/GolandProjects/awesomeProject1 && /Users/unisoft/.claude/plugins/cache/claude-agent-team/agent-team/0.3.0/bin/team approve T-045

<!-- questions:end -->

## 4. Architecture & decision log

Owner decisions (2026-10-06, `/team-init` interview). "Rejected" lists the options the owner or leader declined.

| # | Decision | Rationale / consequences | Rejected | Revisit when |
|---|---|---|---|---|
| D-01 | **Audience: university/college (18+) first.** | Adults only, so no parental-consent flow in v1. Contributors may be anyone but the form collects the minimum (name, message, photos). | High school incl. minors; both from day one | A school pilot is planned (own milestone: consent, photo rules, teacher-gated accounts) |
| D-02 | **UI languages: English + Vietnamese, i18n from M0.** | All strings externalised (react-i18next); CI fails on key mismatch between locales. API error codes are stable; messages are localised on the client. | EN only; VI only | A third language is requested (just translation files) |
| D-03 | **M1 = personal yearbook end to end; class mode is M3.** | Proves the whole value chain without roles/invites. Class mode reuses the same book/page/notes engine. | Class first; both thin | Owner wants a class pilot sooner (see Q-002) |
| D-04 | **Print scope: PDF export only.** | Print-quality PDF (A5/A4, safe margins, 300 DPI images). User prints it anywhere. | PDF + print-shop spec (bleed/CMYK); in-app ordering | Users ask for print-shop acceptance (M4) |
| D-05 | **Stack: Go API + Vite + React + TypeScript SPA + MySQL 8.4.** | Matches the existing Go scaffold. MySQL chosen by the owner (leader had recommended PostgreSQL). Consequences: charset `utf8mb4`; no `RETURNING` (use `LastInsertId`); no Postgres-only features (RLS, partial indexes); RDS MySQL in M2. React chosen as the Vite framework (leader recommendation, owner said "Vite frontend"). | Next.js full-stack TS; Go + HTMX; Vue; Svelte | — |
| D-06 | **PDF engine: pure-Go PDF library.** | Owner chose this over the leader's recommendation (headless Chromium + HTML/CSS). Consequences: templates are a **declarative JSON spec rendered by Go**, not HTML/CSS; the M1 editor is **form/slot-based** (no free-form canvas); preview is the **real PDF shown with pdf.js**, so preview equals output; Vietnamese needs an embedded Unicode TTF and NFC-normalised text. Candidate libraries (MIT): `go-pdf/fpdf`, `signintech/gopdf`; chosen by spike **T-005**. `unipdf` is excluded unless the owner approves its commercial licence. | HTML→PDF via Chromium; hosted PDF API | T-005 finds no library meets the criteria (then Chromium returns to the table) |
| D-07 | **Auth: in-house in Go.** Email+password (argon2id) and Google sign-in (OIDC, PKCE); other social providers later. | Agents must test locally with no cloud credentials; no per-user fees; no vendor lock-in. Cost: we own the security details, so every auth task is `Risk: high` and needs `bin/team approve`. | Amazon Cognito; Clerk/Auth0/Supabase; email+password only | Compliance requires a managed IdP, or MAU grows large |
| D-08 | **Hosting: AWS (Fargate + RDS MySQL + S3 + CloudFront).** | Owner choice; provisioning is M2 and owner-assisted (account, billing, domain, Google OAuth client). Local dev is docker-compose either way. IaC tool undecided — see Q-001. | Container PaaS + R2; decide later | — |
| D-09 | **Repository: public `github.com/danyaa666/smemories`.** | Owner choice. The board (`.team/`) and all code are public — never commit secrets, real data, or student photos. Module path `github.com/danyaa666/smemories`. | Private; owner-created repo | — |
| D-10 | **IaC tool: OpenTofu / Terraform (HCL).** | Answered `A` on the board (2026-10-07, Q-001). State in an S3 backend with locking; deploys authenticate with GitHub OIDC (no long-lived AWS keys). Used by T-023 (M2). | AWS CDK; CloudFormation/SAM | A platform standard appears, or the project leaves AWS |
| D-11 | **Roadmap order: M2 go-live before M3 class yearbook.** | Answered `A` on the board (2026-10-07, Q-002). Prove deployment, email, backups and deletion with a small user base before holding a whole class's data. | M3 class first | A school or class pilot gets a date |
| D-12 | **PDF library: `codeberg.org/go-pdf/fpdf` v0.12.0** (spike T-005, ADR 0002). | Accepted by merging the ADR; `signintech/gopdf` is the fallback. Both libraries passed all criteria; fpdf wins on text wrapping and memory. Consequences: import the Codeberg path (the GitHub repo is archived); fpdf panics on runes above U+FFFF, handled with PUA aliases and a run splitter in T-010; **emoji print as monochrome outlines, colour emoji is out of scope**; small community (single-digit maintainers), so watch release health. | `gopdf`; `unipdf` (AGPL or commercial); Chromium | fpdf has no release for a year, or a template needs colour emoji, complex-script shaping or CMYK/bleed |
| D-13 | **No CAPTCHA on the public friends' note form for now; a verifier hook stays in place.** | Answered by the owner in chat 2026-10-07 (Q-005, option A). Abuse is bounded by an unguessable revocable link, per-IP and per-link rate limits, a 300-note cap, size limits, a honeypot field, a verified-email owner and pending-by-default moderation; friction for friends on phones is the larger risk. | Cloudflare Turnstile from the start; hCaptcha/reCAPTCHA | A link is actually spammed, or a public (non-link) form is added |
| D-14 | **Branch protection on `develop` and `main` (set 2026-10-07 at the owner's request).** | Required checks `go`, `go-integration`, `web`, `security`; pull request required with 0 approvals; force-push and deletion blocked; administrators NOT enforced, so the owner can promote develop to main and the leader's board-sync pushes to develop still work. A PR with failing checks cannot be merged without `--admin`. | Require approvals; enforce for admins | A second human contributor joins (then require 1 approval and enforce admins) |
| D-15 | **Designer templates: pilot two designs first.** | Owner answer in chat 2026-10-08: pilot temp1 (Memory Book) and temp2 (navy classic) of the eight Claude Design canvases in `design/canvas/`, then roll out the other six after the owner reviews the pilots. Epic E08. | All eight at once; only temp1 | The pilots are reviewed |
| D-16 | **Add US Letter (215.9 x 279.4 mm) as a third page size** next to A5 and A4. | Owner answer in chat 2026-10-08, against the leader's recommendation (re-export the designs at A5/A4, because Vietnamese print shops use A4). The designs stay US Letter; a Letter template prints only on Letter (different aspect ratio than A5/A4). Consequence: the DB enum `page_size` and the API grow a value (T-037); A4 users cannot use the designer templates until the designs are re-exported at A5/A4 and the import pipeline is rerun. | Re-export at A5/A4; scale Letter onto A4 with a margin | An A4 user needs the designer templates (then re-export and rerun the pipeline), or the first print shop asks for A4 |
| D-17 | **Designer templates cover our four page kinds only** (cover, profile, friends' notes, back). | Owner answer in chat 2026-10-08. The designs' other pages (class portraits grid, class awards, friend quiz, year in review, letters, galleries, contacts) need new data fields and UI; they wait for the class yearbook (E07) or a later product decision. Mapping per design is in the E08 PRD. | Add new page kinds now | A page kind is requested, or E07 starts |
| D-18 | **Fonts: swap non-Vietnamese designer fonts for open-licence lookalikes that support Vietnamese** and bundle them. | Owner answer in chat 2026-10-08. The canvas CSS shows every design uses at least one font without a Vietnamese subset (Fredoka, Gaegu, Jost, Caveat, Karla, DM Sans, Shrikhand, Bebas Neue, Courier Prime, Instrument Sans/Serif, DM Serif Display); only temp4 is fully covered. A registry test refuses any family that lacks Vietnamese letters. Licences (OFL) are kept next to the files. | Keep designer fonts (Vietnamese would fall back or print `?`); ship only temp4 | A design needs a font with no Vietnamese-capable lookalike |
| D-19 | **Editing is staged: guided customisation after M1 (M1b), the free-layout editor as its own milestone after go-live (M4).** | Owner answer in chat 2026-10-08 (Canva-style request). Stage 0 (M1): system templates with template-driven forms. Stage 1 (M1b): per-book design copy with guided changes. Stage 2 (M4): free-layout editor (add, remove, move, resize, replace), order against the class yearbook M3 decided at the M2 retrospective. The document model is the template JSON (format v2); the Go renderer stays the single source of truth (L-12, D-12). Epic E09. | Full editor right after M1; fixed templates only | Stage 1 is stable, or the owner brings the editor forward |
| D-20 | **Each yearbook owner edits their own copy of a template; users do not publish templates to others.** | Owner answer in chat 2026-10-08. System templates are starting points curated through the import pipeline (E08); a student's edits belong to their book only, so there is no moderation, copyright or abuse surface for shared designs. | Template gallery where users publish; admin-only template editor | Students ask to share designs |
| D-21 | **Friends' notes are template-driven from the start: answers keyed by field id from a closed catalogue, form generated from the template's fields.** | Owner answer in chat 2026-10-08 (the designs need richer forms: how we met, first impression, best memory, wish). `internal/notefields` (T-043) defines fields with limits and EN/VI labels; T-034 stores `notes.answers` JSON validated against it; the public lookup returns the form's `fields`; templates declare `note_fields` (T-044). Answers survive a template change. The catalogue has no personal-data fields beyond a name (D-01 data minimisation). | Fixed columns in M1, migrate later (rewrite of tables, form, moderation, export); fixed fields forever | A field needs a type the catalogue lacks (rating, choice), or contributors must give contact data |

Leader decisions (low-risk, inside the approved stack):

| # | Decision | Why |
|---|---|---|
| L-01 | Go: stdlib `net/http` ServeMux (Go 1.22+ patterns) + small middleware; `database/sql` + `go-sql-driver/mysql`; `sqlc` (MySQL engine) for typed queries; `pressly/goose` migrations, embedded SQL; `log/slog` JSON logging; `aws-sdk-go-v2` for S3 (MinIO locally). | Fewest dependencies; each piece is boring and replaceable. |
| L-02 | API contract is a hand-written `api/openapi.yaml` (OpenAPI 3.1); TS types generated with `openapi-typescript`; Postman collection per epic in `postman/`. | One contract both sides read; keeps FE/BE from drifting. |
| L-03 | Web: react-router, TanStack Query, react-i18next, Vitest + Testing Library, `pdfjs-dist` for the preview; Playwright for the M1 E2E smoke; system font stack (handles Vietnamese). | Mainstream choices with the best agent support. pdf.js (not an `<iframe>`) because phone browsers do not show embedded PDFs. |
| L-04 | Repo layout: `cmd/<binary>/`, `internal/<domain>/`, `migrations/`, `api/`, `postman/`, `web/`, `docs/`, `docker-compose.yml`, `Makefile`. | One Go module at the root, one Vite app in `web/`. |
| L-05 | IDs: `BIGINT UNSIGNED AUTO_INCREMENT` primary keys internally; every externally visible id is an opaque ULID (`CHAR(26)`, unique). Timestamps are `DATETIME(6)` in UTC. | No enumerable ids in URLs; cheap joins. |
| L-06 | Risk calibration: **high** = auth, anything personal-data-bearing and public, file uploads, new core dependency, infra/CI/secrets, migrations that change existing data. Greenfield **additive** migrations before the first production deploy are **low** (no data to lose). | Keeps owner approvals for what can really hurt, not for every table. |
| L-07 | HTTP paths: infrastructure routes `/healthz` and `/readyz` at the root; business routes under `/v1/…`. The web app calls the API at `/api/*` on its own origin and the edge strips `/api` (Vite proxy in dev, CloudFront in M2). Same origin means a `SameSite=Lax` session cookie works and no CORS is needed. | Simplest secure cookie setup; one place (the edge) owns the prefix. |
| L-08 | Dev/CI object store: MinIO through the frozen image `bitnamilegacy/minio:2025.4.22-debian-12-r2`, loopback-only, no real data. | MinIO stopped publishing images on Docker Hub and Quay. The frozen image gets no security patches, which is acceptable for a dev-only, loopback-only store; it is also the last release with a working web console (T-002 AC8). The app uses only the S3 API via aws-sdk-go-v2, so the store is swappable. Replacement tracked in T-029. |
| L-09 | Auth dependencies and Unicode rule: `golang.org/x/crypto` (argon2id) and `golang.org/x/text` approved. Passwords are normalised to NFKC and display names to NFC before validation and hashing/verification. | The same Vietnamese password can arrive as NFC or NFD from different devices and keyboards; normalising once, before any user exists, prevents lock-outs. NIST SP 800-63B recommends NFKC/NFKD. Changing this after users exist would break their logins. |
| L-10 | Go toolchain: `go.mod` keeps `go 1.26.0` as the minimum, but CI and production images build with the newest 1.26 patch release. | At exactly go1.26.0 `govulncheck` reports 11 reachable standard-library vulnerabilities; the current patch has none. Raising the `go` directive would force every dev machine to download a newer toolchain for no benefit, while the vulnerable code only matters in what we ship. The Dockerfile (T-022) must follow the same rule. |
| L-11 | Persistence: plain `database/sql` with parameterised queries and hand-written SQL; `sqlc` (mentioned in L-01) is not adopted. | Auth and yearbooks already use plain SQL cleanly and there is no code generation step to maintain; revisit if the query surface grows. Migrations stay numbered and ordered: always take the next free number (goose rejects out-of-order versions). |
| L-12 | **Designer templates are imported as pre-rendered decoration plus slots; the runtime stays pure Go.** | Leader decision 2026-10-08 within D-06/D-12. A dev-only Node/Playwright tool renders each canvas page to a background PNG with text and photo placeholders hidden and reads slot and label boxes from the page; the Go renderer draws the background, then localised static text and the user's text and photos (template format v2, T-038). No browser at run time. Costs about 1 MiB per page of embedded assets (budget 8 MiB per template). | Re-draw every design in JSON shapes by hand; render HTML in the server (headless browser); embed PDF pages as backgrounds (new dependency) | Assets pass 64 MiB in total, or a design needs live vector effects |

## 5. Engineering conventions

- **Build:** `make build` (Go build + web build). **Test:** `make test` (Go `-race` unit tests + vitest). **Integration:** `make test-integration` (needs `make up`). **Lint:** `make lint` (gofmt check, `go vet`, golangci-lint, eslint, `tsc --noEmit`, i18n parity). These targets are created by T-001, T-002, T-003 and mirrored in `.team/config.json`.
- **Branches:** `task/t-<id>-<slug>`, one per task, PR to `develop`, squash-merged by the leader; the owner promotes `develop` → `main` with `bin/team promote`.
- **API errors:** always `{"error":{"code":"snake_case_code","message":"English text for logs","request_id":"…"}}` with the right HTTP status. Codes are the contract; the client localises messages.
- **Data:** UTF-8 everywhere (`utf8mb4`), UTC timestamps, opaque ULIDs outside the database, no PII in logs, secrets only from environment variables.
- **i18n:** every user-visible string lives in `web/src/locales/{en,vi}.json`; both files get the key in the same PR.
- **Definition of done:** acceptance criteria met; tests written and green; build + lint green; `api/openapi.yaml` and the Postman collection updated for any HTTP change; PR open; QA_PASS; leader review passed.

## 6. Tasks

Task block anatomy (leader-written; dev/qa touch only `Status`, `Branch`, `PR`, and the Comments list):

```text
### T-007 — Short imperative title
- **Status:** TODO
- **Priority:** P1            (P0 urgent … P3 nice-to-have)
- **Type:** feature | bug | tech-debt | security | perf | docs | infra
- **Milestone:** M1
- **Depends-on:** T-003, T-004
- **Assignee:** —            (maintained automatically)
- **Branch:** —   **PR:** —
(body: a short stub that links the full spec, `.team/epics/E##-slug/NN-slug.md`; the PRD is `.team/epics/E##-slug/PRD.md`)
#### Comments                (one line per comment: timestamp · role · text)
```

<!-- tasks:start -->

### T-001 — Repo foundation and API skeleton
- **Status:** DONE
- **Priority:** P1
- **Type:** infra
- **Milestone:** M0
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-001-repo-foundation-and-api-skeleton
- **PR:** https://github.com/danyaa666/smemories/pull/1
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/01-repo-foundation-and-api-skeleton.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Replace the GoLand "hello world" stub with the skeleton every other task builds on: module path, directory layout, an HTTP server with shared middleware and the shared error envelope, env-based config, the OpenAPI/Postman starting points and the Makefile.

#### Comments
- 2026-10-07 02:37Z · leader · Board repair by leader: the dev's READY_FOR_QA transition (events.jsonl 02:20:20Z) and PR link were lost from the README (a stale overwrite); restored from the event log and `gh pr view 1` (task/t-001-repo-foundation-and-api-skeleton -> develop, open, mergeable, no CI yet). Dev hand-back, relayed and NOT yet verified (QA to confirm): all 9 AC covered by tests; `make lint build test` green; live binary checked /healthz, 404/405 envelopes, SIGTERM exit 0, invalid SMEM_ENV exits 1 naming the variable; Newman collection passed twice back to back (10/10). Deviation from the design diagram: the access log wraps recover, so a recovered panic logs as 500 and security headers also appear on 500s. No new dependencies. PR #1 and PR #2 will conflict trivially on go.mod and Makefile; dev rebases the second after the first merges.
- 2026-10-07 02:40Z · qa · QA_PASS at e19483a, verified myself from a fresh clone of the PR branch, not the dev's claims. AC1: go.mod module github.com/danyaa666/smemories, go 1.26 kept, no root main.go, go build ok. AC2: 'make lint build test' green on a fresh clone; go test -race -count=3 green; coverage config 100%, httpx 86%. AC3: live binary: /healthz 200 {status:ok}; /nope 404 not_found; POST and DELETE /healthz 405 method_not_allowed with Allow: GET, HEAD; all in envelope with request_id equal to X-Request-Id. AC4: one JSON log line per request with request_id,method,path,status,duration_ms; no query/cookie/auth in logs. X-Request-Id: valid reused; invalid or 10 kB id replaced with a fresh 32-hex id; CR injection attempt rejected by net/http with 400 before reaching our code. Panic recovery covered by unit test (logs stack, 500 envelope, keeps serving); not reachable on the live binary (no panicking route). AC5: SMEM_ENV=staging, SMEM_LOG_LEVEL=loud, SMEM_HTTP_ADDR=abc/:99999/:8080x each exit 1 naming the variable; defaults ok; .env.example has no secrets. AC6: SIGTERM and SIGINT exit 0 in 0.02 s idle; with a stalled half-sent request SIGTERM exits 0 in 4.8 s (ReadHeaderTimeout), new connections refused after SIGTERM; in-flight-handler drain covered by TestServeStartsAndShutsDownOnCancel (the real binary has no slow route to test it live); timeouts asserted by TestNewServerTimeouts; 2 MiB body cap covered by unit tests (413 payload_too_large, exact-cap OK, per-route override) because the live binary has no route that reads a body: first body-reading endpoint's task must add a live 413 case. AC7: all four headers present on 200/404/405 live and on 500 in unit test. AC8: openapi.yaml documents /healthz and Error (redocly lint: structurally valid, only style warnings); Newman run twice back to back on a fresh server: 5 requests/10 assertions, 0 failed both times. AC9: README Development section present and accurate; 'make run' starts the server (verified). make lint run on the PR: no CI exists yet (T-004), 'gh pr checks' reports none. Non-blocking: (a) golangci-lint v on this machine reports 2 findings that T-004 will hit: errcheck internal/httpx/server_test.go:46 (resp.Body.Close) and staticcheck ST1023 internal/httpx/router.go:23 ('var h http.Handler =' can drop the type); (b) openapi.yaml: 404 and 413 responses are not documented and no 'security: []'; (c) cmd/smemories-api has 0% coverage; (d) the 404 Newman test asserts the code twice. Dev deviation (access log wraps recover) confirmed and fine. PR #1 mergeable against develop on its own.
- 2026-10-07 02:47Z · leader · merged c4c25a8 (squash). Review: all 9 AC met by the diff, no new deps, middleware order differs from the diagram in a justified way (access log wraps recover, so panics log as 500 and 500s carry security headers). For owner to check: run make run and curl localhost:8080/healthz. Follow-ups in T-028.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-002 — Local stack (MySQL + MinIO), migrations and readiness
- **Status:** DONE
- **Priority:** P1
- **Type:** infra
- **Milestone:** M0
- **Depends-on:** T-001
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-002-local-stack-mysql-minio-migrations-and-r
- **PR:** https://github.com/danyaa666/smemories/pull/3
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/02-local-stack-mysql-minio-migrations-and.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Give every later task a database and object store that start locally with one command, a migration mechanism that can also run as a one-off task on ECS later, a readiness endpoint, and an integration-test harness.

#### Comments
- 2026-10-07 03:07Z · dev · All 9 AC done, see PR #3. Test: docker running; make up migrate; make lint build test test-integration; make run then curl /readyz; docker compose stop mysql -> 503, start -> 200; newman run postman/platform.postman_collection.json --folder Health --folder 'Edge cases'. Leader FYI: MinIO no longer publishes images (Docker Hub/Quay pulls fail), so compose uses frozen bitnamilegacy/minio:2025.4.22-debian-12-r2 (last release with a working console, needed for AC8); dev-only, no security patches; swap later if desired. New deps goose v3.28 + mysql driver v1.10.1 (L-01); go directive 1.26.0 will conflict trivially with PR #2's go.mod.
- 2026-10-07 03:14Z · qa · Verified on a fresh clone of PR #3 head a35e918 (no CI checks on the branch). Isolated compose project 'smemqa' on ports 3406/9100/9101 because dev's 'smemories' stack was already holding 3306/9000. Evidence by AC: AC1 make up created .env from .env.example, both services healthy (~7 s), second make up idempotent, make down keeps volumes (row survived down/up), published ports are 127.0.0.1 only (docker compose config host_ip, lsof) and nc to LAN IP 192.168.1.11 on all three ports = closed, loopback open; on default ports with the other stack present make up fails with a clear 'port is already allocated'. AC2 SELECT @@: utf8mb4 / utf8mb4_0900_ai_ci / sql_mode has STRICT_ALL_TABLES / time_zone +00:00; strict mode rejects a 300-char value in VARCHAR(255) (Error 1406). AC3 smemories-migrate up|down|status work, make migrate = up; up->down->up run twice, table and ('schema_epoch','1') recreated each time, SHOW CREATE TABLE matches spec; embedded FS (migrations/embed.go). AC4 unit tests for Normalize/pool pass; API with MySQL stopped: 8 warn retries over 10.0 s then exit 1 'database at 127.0.0.1:3406 not reachable after 10s'; DB started 3 s into the window -> API comes up and /readyz 200; wrong password and malformed DSN never print the password (grep count 0 in API and migrate output and docker compose logs). AC5 /readyz 200 {"status":"ready"}; mysql stopped -> 503 error envelope code not_ready, body 'dependency not ready', driver text only in the log; docker pause mysql -> 503 after 1.00 s; start -> 200 again within 2 s; POST -> 405 envelope. AC6 TestUTF8MB4RoundTrip + TestSessionTimesAreUTC pass; own check via mysql client: 'Chúc mừng 🎓 Đặng Thị Hồng' stored and read back, hex 4368C3BA..F09F8E93.. (25 chars/38 bytes), character_set_client/connection/results = utf8mb4. AC7 make test-integration twice back to back green with -count=1 (TestMigrateCycle, TestReadyzAgainstRealDatabase etc. all ran), no smem_test_* database left behind; unit tests also pass with all SMEM_* env unset. AC8 bucket smemories-dev exists (mc ls), put/get/rm object works with the root creds, anonymous access 403 (private), console 127.0.0.1:9101 returns the 'MinIO Console' page, image is multi-arch (arm64 ran natively). AC9 openapi /readyz has 200/503/405; newman platform collection (--folder Health --folder 'Edge cases') run twice back to back: 7 requests, 13 assertions, 0 failed both times; the manual 'stop the database first' folder passes (503 not_ready, no driver details) with MySQL stopped. Also: make lint (gofmt, go vet, go vet -tags integration), make build, make test all green. go.mod conflict with PR #2 (not resolved by me): merging PR #2 into this branch conflicts in go.mod, go.sum (add/add) and Makefile (.PHONY and targets); whoever merges second resolves as the union of requires/targets and re-runs go mod tidy + make lint build test (+spike, +test-integration). Non-blocking observations: (1) MinIO image: bitnamilegacy/minio:2025.4.22-debian-12-r2 pulls fine (manifest lists amd64 and arm64, image labelled AGPL-3.0-only, built 2025-05-17 by Broadcom/Bitnami, source github.com/bitnami/containers); the bitnamilegacy namespace is Broadcom's unsupported archive that can be deleted without notice, receives no CVE fixes, and MinIO server is AGPL-3.0: acceptable for dev-only, loopback-only, no data, but pin by digest or replace before anything depends on it, and never ship this image beyond local dev. (2) Wrong DB password (Error 1045) is retried for the full 10 s like a transient error; harmless, could fail fast. (3) With SMEM_ENV=test and empty SMEM_DB_DSN the API still dials the driver default (127.0.0.1:3306, anonymous) and exits after 10 s: AC4 only says the DSN is optional in test, intent is unclear. (4) migrate down with nothing applied exits 1 'no next version found' (fine, just noting). (5) AC4 text says charset=utf8mb4 in the DSN; implementation deliberately sets only collation (driver derives the charset), outcome verified by the session-variable checks. (6) Process note: I first ran make up from my clone without the isolation env and docker compose recreated the dev's running 'smemories-mysql-1' container (same project name, named volumes kept, same config, healthy again); no data lost, flagging in case the dev sees a restart. (7) test grants for smem_test_% are created only when the MySQL volume is first created (init script), so a pre-existing volume needs make down + docker compose down -v once; worth a line in docs.
- 2026-10-07 03:16Z · leader · merged 61dba35 (squash). Review: all 9 AC met; ports loopback-only, DSN/password never in errors or logs, /readyz hides driver errors, migration reversible, strict sql_mode, utf8mb4. For owner to check: install Docker, run make up migrate run and curl localhost:8080/readyz. Follow-ups in T-030 (compose isolation, fail-fast on auth errors, test grants) and T-029 (frozen MinIO image).
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-003 — Web scaffold: Vite + React + TypeScript + EN/VI i18n
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M0
- **Depends-on:** T-001
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-003-web-scaffold-vite-react-typescript-en-vi
- **PR:** https://github.com/danyaa666/smemories/pull/4
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 7

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/03-web-scaffold-vite-react-typescript-en-vi-i18n.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Create the web app skeleton: Vite + React + TypeScript with English/Vietnamese i18n from the first screen, a typed API client generated from `api/openapi.yaml`, tests, and make targets.

#### Comments
- 2026-10-07 05:57Z · dev · Web scaffold in web/ (PR #4). Test: make lint build test (Node 22), cd web && npm ci && npm run build/lint/typecheck/test/lint:i18n/check:api; delete a key in vi.json and lint:i18n names it; cd web && npm run dev with API on :8080 shows API: ok. Deps flagged: typescript ~5.9 and eslint ^9 (peer limits of typescript-eslint/jsx-a11y). web/go.mod is deliberate (keeps Go tooling out of node_modules). Makefile will conflict trivially with T-005 (spike target, .PHONY). Not covered: real browser visual check, CI (T-004).
- 2026-10-07 06:08Z · qa · Verified on a fresh clone of PR #4 head 173af5d (no CI checks on the branch; node v22.17.0, npm 10.9.2). EVIDENCE BY AC. AC1: 'npm ci' (386 pkgs) then 'npm run build' rc 0 (tsc + vite build, 344 kB js / 109 kB gzip); tsconfig strict + noUncheckedIndexedAccess read; .nvmrc=22, engines >=22.12.0 <23. AC2: npm run lint (eslint --max-warnings 0 + prettier --check), typecheck, test (5 files / 25 tests) all rc 0; .prettierrc.json committed. AC3: switcher is role=group + native buttons with aria-pressed (tests assert true/false and group relabel EN/VI); persistence key smemories.lang; getItem and setItem each in try/catch (tests with Storage.prototype throwing: falls back to browser language / still switches, html lang follows); detectLanguage maps vi-VN, vi -> vi and en-US, fr-FR -> en; real headless Chrome: --lang=en-US gives html lang=en with EN pressed; --lang=vi-VN gives html lang=vi, VI pressed, group label in Vietnamese. AC4: lint:i18n rc 0 on the real files; deleting notFound.home from vi.json -> rc 1 'notFound.home: missing in vi.json'; emptying api.ok in vi.json -> rc 1 'api.ok: empty or non-string value in vi.json'; extra key only in en.json -> rc 1 'extra: missing in vi.json'; fixture-pair tests in scripts/check-i18n.test.ts pass; files restored (git status clean). AC5: real Chrome against vite dev + API on :8080: home shows SMemories, tagline, switcher and 'API: ok' (vi: 'API: hoạt động'); /nope renders 'Page not found' with a link home. AC6: check:api rc 0 on the commit; appending a line to schema.d.ts -> rc 1 'src/api/schema.d.ts is stale: run npm run gen:api'; renaming /healthz in openapi.yaml -> rc 1 as well; both restored, rc 0 again. client.ts maps the envelope to ApiError{status,code,message,requestId}; client.test.ts passes (envelope, non-JSON, network error status 0). AC7: dev server :5173 with the API running: curl localhost:5173/api/healthz -> 200 {status:ok} with the API's security headers and x-request-id; API access log shows /healthz, /nope, /readyz (prefix stripped). With the API stopped: curl -> 502 Bad Gateway and Chrome renders 'API: unreachable' (vi: 'API: không kết nối được'). AC8: make web-install/web-build/web-test/web-lint exist; 'make lint build test' from the clone rc 0 and runs the web steps (node_modules installed on demand from the lockfile). AC9: banner/main landmarks asserted in App.test, :focus-visible 3px outline #0b4fa8, contrast computed by me: body 17.4, link 7.78, ok badge 7.16, error badge 8.11, checking badge 14.73 (all >= 4.5); keyboard test Tab to EN, Tab, Enter -> VI pressed (native buttons so Enter and Space work). HYGIENE: no node_modules or dist tracked (0 matches in git ls-files web; .gitignore covers them); package-lock.json committed and 'npm ci' reproducible (git status clean afterwards); npm audit and npm audit --omit=dev: 0 vulnerabilities. DEV FLAGS: (1) web/go.mod is justified: with it, go build ./... && go vet ./... pass and go list ./... lists only the 7 repo packages (0 under web); with web/go.mod temporarily removed, go list picks up web/node_modules/flatted/golang/pkg/flatted, so it does what the dev says. (2) typescript ~5.9: typescript-eslint 8.71.1 peers typescript >=4.8.4 <6.1.0 (latest TS 7.0.2 is out of range), pin is correct (5.9 is conservative, 6.0.x would also be allowed). eslint ^9: eslint-plugin-jsx-a11y 6.10.2 (latest) peers eslint up to ^9, eslint latest is 10.12.0, pin is correct. MAKEFILE vs PR #2: 'git merge-tree pr4 pr2' conflicts in Makefile only (go.mod, go.sum and everything else merge cleanly); both changes are additive (PR #4 edits .PHONY and build/test/lint and adds web targets, PR #2 edits .PHONY and appends the spike block): resolve as the union; whoever merges second re-runs make lint build test (+spike). Not resolved by me (PR #2 is the one the leader will probably merge first). NON-BLOCKING: (a) npm ci prints 'eslint@9.39.5 deprecated: no longer supported' (npm dist-tag for 9.x is maintenance); held back by jsx-a11y, revisit when the plugin supports eslint 10; (b) licences: all prod packages are MIT or Apache-2.0 (react, react-dom, react-router-dom 7, i18next 26, react-i18next 17, @tanstack/react-query 5); the dev tree has MPL-2.0 (13 pkgs, build tooling), CC-BY-4.0 (1), Python-2.0 (argparse), no GPL/AGPL; every top-level dep was published within the last 4 months so all are actively maintained; (c) most deps are on very new majors (vite 8, vitest 5, jsdom 29), dependabot in T-004 will help with churn; (d) localStorage failure covered by unit tests only (headless Chrome --disable-local-storage gave no usable result); (e) no CI yet (T-004), so all checks above are local.
- 2026-10-07 06:10Z · leader · No code issues: QA_PASS and leader review OK. Conflict only. T-005 is merged (develop b1eaffa) and its spike target conflicts with this branch in the Makefile. 1) Merge origin/develop into the branch (no force-push). 2) Resolve the Makefile as the union: develop's targets (build/test/lint/run/up/down/migrate/test-integration, the .env include block, the spike target and its .PHONY entry) plus your web-* targets and the build/test/lint additions. 3) Re-run make lint build test from your task worktree. 4) Change nothing else, then READY_FOR_QA; QA only verifies the merge result. Heads-up: T-028 (PR #5) also edits the Makefile lint recipe, so the second of T-003/T-028 to merge will need one more conflict resolution; keep your lint recipe change minimal to make that easy.
- 2026-10-07 06:15Z · dev · Merged origin/develop (ff046a4) into the branch, no force-push. Makefile resolved as the union (spike target + .PHONY, develop's git-ls-files gofmt lint recipe + go vet integration tag, plus web-* targets and web steps in build/test/lint). README command table merged the same way. One extra change forced by the merge: T-028 changed api/openapi.yaml (404/405), so check:api reported stale types; regenerated web/src/api/schema.d.ts with npm run gen:api. make lint build test all green (25 web tests, go tests). QA: verify merge result only.
- 2026-10-07 06:20Z · qa · Merge-result verification, fresh clone of PR #4 head febf7c9 (develop ff046a4 is an ancestor; no CI checks exist on the branch). EVIDENCE. (1) 'git diff 173af5d febf7c9 --stat -- web' shows only web/src/api/schema.d.ts (+11/-1); everything else in the 173af5d..febf7c9 diff is develop's content (T-005 spike/ADR/fonts, T-028 httpx/db/cmd/openapi, .team, README). (2) The regenerated schema.d.ts matches the OpenAPI change: new responses.NotFound (404, Error schema), descriptions 'Error code not_found' / 'method_not_allowed', and 404 added to /healthz operation, exactly mirroring api/openapi.yaml diff. 'npm run check:api' -> 'api schema ok' rc 0; 'npm run gen:api' leaves git clean. (3) Makefile is the union: 'git diff origin/develop HEAD -- Makefile' shows only additions (web-* targets, .PHONY entries, go build -o bin/smemories-api ./cmd/smemories-api
  go build -o bin/smemories-migrate ./cmd/smemories-migrate web-build/web-test/web-lint in build/test/lint); develop's git ls-files gofmt recipe, vet + vet -tags integration, .env include block, spike target and its PDFCPU vars are intact. (4) make lint rc 0, make build rc 0, make test rc 0 (go test -race all ok; 5 web files / 25 tests passed). (5) go list ./... = 7 packages, 0 under web/. NON-BLOCKING / HEADS-UP FOR WHOEVER MERGES SECOND: 'git merge-tree' of PR #4 and PR #6 is textually clean (go.mod, go.sum, Makefile, openapi), BUT after merging PR #6 on top of PR #4, 'npm run check:api' fails ('src/api/schema.d.ts is stale') because PR #6 adds auth paths to api/openapi.yaml; the second merger must run 'cd web && npm run gen:api' and commit the regenerated schema.d.ts, then re-run make lint build test.
- 2026-10-07 06:28Z · leader · merged 91ec2b8 (squash). Review: strict TS, no raw-HTML rendering, storage failures caught, error envelope mapped to a typed ApiError, locale parity script correct; QA verified every AC and the merge result. I also ran the page myself with the API up: EN and VI render (diacritics fine), html lang follows, the choice persists, the API badge turns green, and the phone layout (375 px) has no horizontal scroll. For owner to check: cd web && npm run dev with the API on :8080, open http://localhost:5173. Small follow-ups parked in T-015.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-004 — CI pipeline (Go, web, integration, security)
- **Status:** DONE
- **Priority:** P1
- **Type:** infra
- **Milestone:** M0
- **Depends-on:** T-001, T-002, T-003, T-028
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-004-ci-pipeline-go-web-integration-security
- **PR:** https://github.com/danyaa666/smemories/pull/8
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 7

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/04-ci-pipeline-go-web-integration-security.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Continuous integration for the Go API and the web app so `develop` and `main` can be protected by required checks.

#### Comments
- 2026-10-07 02:47Z · leader · Leader note from the T-001 review: golangci-lint currently reports 2 findings on develop (errcheck at internal/httpx/server_test.go:46, ST1023 at internal/httpx/router.go:23). T-028 fixes them and now blocks this task, so the new CI starts green.
- 2026-10-07 06:41Z · dev · PR #8 green on its own run: https://github.com/danyaa666/smemories/actions/runs/37582588508 (go, go-integration, web, security all pass; no secrets referenced; actions pinned to full SHAs incl. golangci-lint-action v9.3.0 peeled commit). AC1-AC8 mapped in the PR body. LEADER DECISION NEEDED: go.mod says go 1.26.0, with which govulncheck (blocking) reports 11 reachable stdlib vulns (fixed in 1.26.2..1.26.6); CI therefore derives 1.26 from go.mod and uses check-latest (resolved go1.26.8). Alternative is raising the go directive in go.mod. Other deviations: MySQL started with docker run (compose flags) since a services: block cannot pass server args; golangci-lint built with install-mode goinstall because the v2.8.0 release binary is built with Go 1.25 and rejects a go 1.26 module; bodyclose (required linter) found 2 issues in internal/httpx/server_test.go, fixed; golangci-lint added to make lint. QA: do the planned throwaway-branch probes (unformatted file, failing test, vi.json key, stale schema.d.ts). Branch protection is for the owner; names in docs/ci.md.
- 2026-10-07 07:06Z · qa · Verified at PR #8 head 0482b75 (fresh clone). AC1: ci.yml read in full: on pull_request (no branch filter) + push [develop, main]; concurrency group ci-${{ github.ref }} cancel-in-progress true (proved: my earlier pushes to one probe branch left 2 runs 'cancelled'); top-level permissions contents: read; no pull_request_target/workflow_run/secrets/github.token (grep empty; only fake runner-only MySQL password). AC2: job go = gofmt -l, go vet (+ -tags integration), golangci-lint v2.8.0 action (.golangci.yml: standard = errcheck, govet, ineffassign, staticcheck, unused, + gosec, bodyclose, build-tags integration), go test -race -coverprofile, coverage written to GITHUB_STEP_SUMMARY; run log: golangci-lint '0 issues', coverage printed, setup-go cache hit. AC3: MySQL 8.4 via docker run with compose flags, step asserts utf8mb4/utf8mb4_0900_ai_ci, make test-integration ran against it (internal/db ok in 2.3 s, not skipped). AC4: web job: node-version-file web/.nvmrc, npm ci, lint, lint:i18n, typecheck, test, build, check:api. AC5: govulncheck blocking, npm audit continue-on-error with explanatory comment. AC6: all four third-party actions are full 40-char SHAs with version comments and each checked with git ls-remote: checkout v7.0.1 -> 3d3c42e5, setup-go v7.0.0 -> b7ad1dad, setup-node v7.0.0 -> 82076278 (all lightweight tags equal the SHA); golangci-lint-action v9.3.0 is an annotated tag, peeled commit refs/tags/v9.3.0^{} = ba0d7d2e matches (tag object d583c34f is not the pin, correct). dependabot.yml: gomod /, npm /web, github-actions /, weekly, grouped, target develop. AC7: docs/ci.md lists go, go-integration, web, security, what each runs and the local make/commands; matches the workflow. AC8: gh pr checks 8: go/go-integration/security/web all pass; run 37582588508 linked in the PR comment; read full logs: no ##[warning] annotations; only cosmetic notices (npm warn deprecated eslint 9.39.5, mysql password-on-CLI warning, git detached HEAD hint). Job names exactly go / go-integration / web / security. PROBES on throwaway branches qa/t004-* with draft PRs #9-#15 based on the task branch (no push to develop/main), all PRs closed and all 7 branches deleted (git ls-remote shows none): (a) unformatted internal/httpx/qa_fmt.go -> job go FAILS at step gofmt ('gofmt needed on: internal/httpx/qa_fmt.go'), other 3 pass; (b) failing test -> go FAILS in 'Unit tests with the race detector' and go-integration FAILS (make test-integration runs all tests), web/security pass; (c) vi.json without app.tagline -> web FAILS at lint:i18n ('app.tagline: missing in vi.json'), others pass; (d) stale schema.d.ts -> web FAILS at check:api ('schema.d.ts is stale'), others pass. Extra: (e) unhandled error + chmod 0777 -> go FAILS in golangci-lint (G104 gosec, '1 issues'); (f) old x/crypto (replace directive) with ssh.NewServerConn -> security FAILS in govulncheck listing GO-2026-6355 etc. with the call path from my file (blocking proven); (g) lodash 4.17.15 as prod dep -> npm audit prints the high-severity report but job security is green (non-blocking proven). TOOLCHAIN (L-10): setup-go resolves spec '1.26' with check-latest to go1.26.8 in every job; setup-go sets GOTOOLCHAIN=local, so govulncheck (go run ...@v1.8.0) is compiled and run by that same go1.26.8 and reports 'No vulnerabilities found'. Cross-check: on my local go1.26.1 the same govulncheck run on this code reports 11 stdlib vulnerabilities (crypto/tls, net/http, ...) - so the scan really evaluates the building toolchain, and the dev's claim holds. golangci-lint built via install-mode goinstall by the job's go (log shows go: downloading ... module builds); docker run MySQL justified (services cannot take server args) and verified by the charset assertion. NON-BLOCKING: (1) cancel-in-progress also applies to push runs on develop/main, so a quick second push can cancel the first run on that branch leaving no result for that commit; consider cancel-in-progress: ${{ github.event_name == 'pull_request' }}; (2) mysql:8.4 image is a floating tag (not digest-pinned); (3) dependabot.yml only takes effect from the default branch (main); it reaches main only on the next release merge, so Dependabot starts then (the dev noted this); (4) with go.mod still at 1.26.0, local developers on an older 1.26 patch see govulncheck findings that CI does not (documented in docs/ci.md); raising the go directive would remove the divergence; (5) make lint now requires golangci-lint v2 installed locally (documented in README). Owner action for the merge: set branch protection on develop and main with the four check names.
- 2026-10-07 07:07Z · leader · Leader review OK at 0482b75 (QA_PASS, same head). Read ci.yml, .golangci.yml, dependabot.yml and the Makefile change in full: least-privilege permissions, every action pinned to a full SHA (QA checked each against upstream), persist-credentials false, no secrets, no pull_request_target, fake throwaway DB credentials, govulncheck blocking against the toolchain that builds (L-10), npm audit reported but non-blocking, and QA proved each job fails on the right defect with throwaway PRs it then closed. Awaiting owner approval (Risk: high). Non-blocking, to fold into later work: cancel-in-progress also cancels push runs on develop (use github.event_name == 'pull_request'); mysql:8.4 is not digest-pinned; Dependabot is inactive until this file reaches main. After merge the owner sets branch protection on develop and main with the four check names in docs/ci.md.
- 2026-10-07 07:34Z · human · owner approved merge
- 2026-10-07 07:34Z · leader · merged 61439ff (squash), owner-approved. Review: least-privilege permissions, every action pinned to a verified SHA, no secrets, govulncheck blocking against the toolchain that builds (L-10), QA proved each job fails on the right defect. For owner to check: the Actions tab (first run on develop), then set branch protection on develop and main with the checks go, go-integration, web, security; promote develop to main so Dependabot starts. Follow-ups: cancel-in-progress only for pull requests, digest-pin the mysql image.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-005 — Spike: choose the pure-Go PDF engine
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M0
- **Depends-on:** —
- **Risk:** low
- **Rework:** 2
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-005-spike-choose-the-pure-go-pdf-engine
- **PR:** https://github.com/danyaa666/smemories/pull/2
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 12

**Spec — read this first, it is the source of truth:** `.team/epics/E05-templates-export/01-spike-choose-the-pure-go-pdf-engine.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E05-templates-export · **PRD:** `.team/epics/E05-templates-export/PRD.md`

Spike: decide which pure-Go PDF library SMemories uses (board D-06: the owner chose a pure-Go engine over headless Chromium).

#### Comments
- 2026-10-07 02:35Z · dev · VERDICT: GO codeberg.org/go-pdf/fpdf v0.12.0 (gopdf v0.38.1 close second). Both pass C1-C7; tie-break (text wrapping, memory 442 vs 539 MB) picks fpdf. Caveats: (1) github.com/go-pdf/fpdf is archived, the live repo is on Codeberg, go directive becomes 1.26.0; (2) fpdf panics on runes above U+FFFF, so emoji use PUA aliases in the emoji font + a rune-run splitter; see ADR 'What T-010 must do'. Full table: docs/adr/0002-pdf-engine.md, PR #2. Spike adds go.mod deps (fpdf, gopdf, x/image, x/text, go-pdfium); T-010 should delete internal/pdf/spike and tidy. PR #1 and #2 will conflict trivially on go.mod and Makefile; I can rebase the second one after the first merges.
- 2026-10-07 02:35Z · dev · Spike done, verdict GO fpdf (ADR docs/adr/0002-pdf-engine.md). QA: run make spike, compare numbers (wall/RSS within reason), eyeball docs/adr/0002-assets/*.png and out/*.pdf, check licence claims. All spike code behind the spike build tag. Known: first commit had a stray binary, removed in the second; squash-merge please.
- 2026-10-07 02:44Z · qa · QA, merge-conflict check (not resolved by me): merging PR #1 (T-001) into PR #2's branch gives 1 textual conflict, Makefile (add/add: T-001's build/test/lint/run vs T-005's spike target; union of both is the fix; both .PHONY lines must be merged). go.mod auto-merges to module github.com/danyaa666/smemories with go 1.26.0 (T-005 does not rename the module; its go directive 1.26.0 differs from T-001 AC1 'keep go 1.26', harmless since the library requires it). Root main.go: deleted by T-001, still present on PR #2, merges cleanly. Whoever merges second must rebase and re-run make lint build test spike.
- 2026-10-07 02:44Z · qa · QA_FAIL (1 major). Re-verified on a clean worktree at d721b3e: make spike green; numbers reproduce (fpdf 0.12-0.18 s, 436-442 MB; gopdf 0.10-0.14 s, 514-534 MB; ADR says 0.12 s/442 MB and 0.09 s/539 MB: OK); pdfcpu --mode strict 'validation ok' on all 10 PDFs; pdftotext of fpdf-text.pdf and gopdf-text.pdf contains the NFC Vietnamese sentence exactly; PNGs under 300 KB (max 286 KB), fonts total 1.04 MB; no 2.5 MB artefact in the final diff (the 'awesomeProject1' binary was added in 3dc2b81 and removed in d721b3e, so only squash-merge keeps it out of history, as the dev said; largest file in the tree is the 765 KB emoji font). Licence claims checked upstream: fpdf MIT (Codeberg LICENSE; GitHub copy archived, v0.12.0 tagged 2026-05-18, repo updated 2026-09-14, 29 stars, 36 open issues), gopdf MIT (2.9k stars, 123 issues, latest tag v0.38.1), go-pdfium MIT, wazero/pdfcpu Apache-2.0, gofpdi MIT, pkg/errors BSD-2; Be Vietnam Pro TTFs are byte-identical to google/fonts and the OFL texts are identical to upstream; Noto Emoji: OFL.txt identical to google/fonts, no Reserved Font Name is declared in it or in the font's name table, so the modification (static instance, subset, PUA cmap aliases) and redistribution under OFL-1.1 is permitted; the file keeps its copyright line and the OFL text is shipped beside it (name IDs 13/14 were stripped by the subsetter, fine because the licence file is bundled). Eyeballed all PNGs and a Quick Look (PDFKit) render: diacritics all present, emoji outlines (cap, popper, heart) render, '?' shown for the missing rune. ISSUE 1 (major, AC4 / C3 fpdf, evidence is wrong): the fpdf full-bleed cover is not full-bleed or centred. Repro: make spike, open internal/pdf/spike/out/fpdf-image.pdf page 1 (or docs/adr/0002-assets/fpdf-image-cover.png): a white strip about 10 mm wide on the left edge, the dark photo border visible on the left, and the crop is the left half of the photo, not the centre (gopdf's PNG is correct). Cause: coverRect gives x=-66 mm, but fpdf silently replaces a negative x with the current x (left margin 10 mm) unless fpdf.ImageOptions.AllowNegativePosition is true; the content stream is 'q 793.7 0 0 595.28 28.35 0 cm'. The test only checks the placement maths, never the rendered page, so ADR C3 'pass: 363 effective DPI, full-bleed' is not backed for fpdf. Expected: x=-66 in the PDF. Fix: fpdf_test.go:112 (and any other call where x can be negative, e.g. lines 149, 225) pass fpdf.ImageOptions{ImageType: "JPG", AllowNegativePosition: true}; I verified locally that this makes the page render full-bleed (JPEG still verbatim). Then regenerate fpdf-image-cover.png (keep under 300 KB), add the pitfall to ADR 'What T-010 must do' item 5 and the C3 row, and add TestQA_ to the Makefile spike pattern (it is not run by 'make spike' today). I committed 4940c72 on the branch: internal/pdf/spike/qa_cover_test.go, TestQA_CoverIsFullBleed, renders both *-image.pdf and fails if the page edge is white; today it fails for fpdf and passes for gopdf; it passes with the one-line fix. The GO fpdf verdict itself looks sound (one-option fix, no change to the ranking), but the leader should know the ADR evidence was wrong until fixed. Non-blocking: (a) ADR C2/C7: pdftotext drops the PUA emoji for fpdf rather than returning PUA code points as the ADR says, and Chrome/Preview were not opened (PDFium = Chrome's engine and Quick Look = Preview's renderer were; fine, wording only); (b) the 'x  x' line in fpdf-text.png is the unguarded CJK-rune demo (extracted as 'x中x', drawn blank) and is unexplained in the ADR, add one sentence so readers do not take it for a bug; (c) consider noting in fonts/README.md that the modified font keeps the name 'Noto Emoji' (allowed, no RFN) so nobody mistakes it for the upstream file; (d) the tree is rebased on the pre-T-001 go.mod (module awesomeProject1): see my merge-conflict comment.
- 2026-10-07 02:47Z · leader · Leader note after QA_FAIL: T-001 is merged (develop c4c25a8). Please (1) apply the AllowNegativePosition fix QA confirmed, regenerate fpdf-image-cover.png and update the ADR C3 row; (2) add qa_cover_test.go to the make spike pattern; (3) rebase onto develop: Makefile conflicts, resolve as the union of both (build/test/lint/run plus spike, merge the .PHONY lines), keep T-001's go module path and keep root main.go deleted; (4) re-run make lint build test spike and report the numbers. Do not commit build artefacts; the PR is squash-merged so the earlier stray binary stays out of develop.
- 2026-10-07 02:50Z · dev · Rework: ISSUE 1 fixed in 48a73f5 (AllowNegativePosition via jpgCover in fpdf_test.go; fpdf-image-cover.png regenerated 286 KB, now identical to gopdf's; covered by TestQA_CoverIsFullBleed, passing for both libs). ADR C3 row + T-010 item 5 carry the pitfall. Also: TestQA_ in make spike pattern; non-blocking (a) extraction wording, (b) 'x  x' CJK demo explained, (c) fonts/README note. Merged origin/develop instead of rebasing (no force-push allowed): Makefile = union (.PHONY build test lint run spike), module path kept, main.go stays deleted; merge-tree vs develop is clean. make lint build test spike green; numbers: fpdf 0.13 s / 441 MB, gopdf 0.09 s / 534 MB (Apple M3 Pro), pdfcpu strict ok on all 10 PDFs. Squash-merge.
- 2026-10-07 03:08Z · qa · Rework round 1 re-verified on a fresh clone of PR #2 head 48a73f5 (no CI checks configured on the branch). ISSUE 1 fixed: jpgCover passes AllowNegativePosition; TestQA_CoverIsFullBleed passes for fpdf and gopdf and now runs in make spike (pattern TestFpdf_|TestGopdf_|TestQA_); regenerated fpdf-image-cover.png (286 KB) viewed by eye: edge-to-edge, centre crop, no white strip (the dark bands top/bottom are the synthetic photo's own border); regeneration leaves git status clean (committed PNGs equal to fresh output). ADR C3 row and T-010 item 5 carry the pitfall. Makefile = union with develop (diff vs origin/develop only adds spike to .PHONY and the spike block; build/test/lint/run intact); go.mod module github.com/danyaa666/smemories, go 1.26.0; root main.go still deleted; no tracked bin/ or out/. make lint build test spike all green. Numbers re-measured (M3 Pro): fpdf 0.14 s / 437 MB, gopdf 0.10 s / 534 MB (ADR 0.13 s/441 MB and 0.09 s/534 MB: consistent, targets 10 s/512 MB: fpdf passes, gopdf RSS 534 MB is slightly over 512 MB which ADR already notes as within 2x). pdfcpu v0.16.1 --mode strict: validation ok on all 10 PDFs. AC1-AC9 evidence as in my first QA round (unchanged files). Non-blocking: PR is squash-merge only because an earlier commit contained a binary; go.mod will conflict with PR #3 (T-002 needs goose, go 1.26.0): whoever merges second must resolve go.mod/go.sum (union of requires, go mod tidy) and Makefile.
- 2026-10-07 03:16Z · leader · No code issues: QA_PASS and leader review OK. Conflict only. T-002 merged (develop 61dba35) and conflicts with this branch in go.mod, go.sum and Makefile. 1) Merge origin/develop into the branch (no force-push). 2) Resolve as the union: go.mod = both dependency sets (goose and mysql from develop plus fpdf, gopdf, x/image, x/text, go-pdfium from here; go directive 1.26.0), then go mod tidy; Makefile = develop's targets (build/test/lint/run/up/down/migrate/test-integration, the .env include block) plus spike, with the .PHONY lines merged. 3) Re-run make lint build test spike from your task worktree and report the numbers. 4) Change nothing else, then READY_FOR_QA; QA only needs to verify the merge result.
- 2026-10-07 05:53Z · dev · Conflict-only rework: merged origin/develop (61dba35..) into the branch (fe7f5d6, no force-push). go.mod/go.sum = develop's (goose, mysql) plus fpdf, gopdf, go-pdfium, x/image, x/text via go mod tidy (go 1.26.0; tidy also bumped x/sync to v0.23.0 as required by the spike deps). Makefile = develop's file with spike added to .PHONY and the spike block appended. Nothing else changed. From the task worktree: make lint build test spike all green; fpdf 0.15 s / 444 MB, gopdf 0.14 s / 534 MB (M3 Pro), pdfcpu strict ok on all 10 PDFs; committed PNGs unchanged. Squash-merge.
- 2026-10-07 05:59Z · qa · Conflict-only rework verified on a fresh clone of PR #2 head fe7f5d6 (no CI checks on the branch). Merge result: origin/develop 0e32db9 is an ancestor of the head; 'git diff 48a73f5 fe7f5d6' touches only develop's files (env, compose, db, migrations, httpx, config, postman, README, .team) and nothing under internal/pdf, docs/adr, fonts or THIRD_PARTY_NOTICES.md. go.mod: module github.com/danyaa666/smemories, go 1.26.0, requires = develop's (goose v3.28.0, mysql v1.10.1) + fpdf v0.12.0, gopdf v0.38.1, go-pdfium v1.21.1, x/image v0.46.0, x/text v0.42.0; x/sync bumped v0.22.0 -> v0.23.0 (indirect, required by spike deps; fine). 'go mod tidy' leaves git status clean. Makefile vs origin/develop: only adds spike to .PHONY and the spike block (build/test/lint/run/up/down/migrate/migrate-down/test-integration and .env include untouched). Commands: make lint build test = rc 0; make spike = rc 0 (TestFixtures, TestSplitRuns, TestFpdf_*, TestGopdf_*, TestQA_CoverIsFullBleed all PASS); go build ./... && go vet ./... ok; pdfcpu v0.16.1 strict: validation ok on all 10 PDFs; working tree clean after make spike (committed PNGs equal fresh output; no tracked bin/ or out/). Numbers (M3 Pro): fpdf 0.12 s / 438 MB, gopdf 0.11 s / 534 MB (ADR 0.15/444 and 0.14/534: consistent). AC1-AC9 unchanged since 48a73f5. Non-blocking: PR must stay squash-merge (earlier stray binary in history).
- 2026-10-07 06:09Z · leader · merged b1eaffa (squash). Review: ADR 0002 is thorough and honest about fpdf's quirks; verdict GO codeberg.org/go-pdf/fpdf v0.12.0 (gopdf the fallback); spike code is behind the spike build tag and excluded from default builds; licences checked by QA. For owner to check: docs/adr/0002-pdf-engine.md and the PNGs in docs/adr/0002-assets/ (note: emoji print as monochrome outlines, colour emoji is out of scope).
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-006 — Email + password auth core (register, login, sessions)
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-001, T-002
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-006-email-password-auth-core-register-login-
- **PR:** https://github.com/danyaa666/smemories/pull/6
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 22

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/02-email-password-auth-core-register-login.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

The core of email+password authentication: register, login, logout, "who am I", cookie sessions, password hashing, rate limiting and CSRF protection.

#### Comments
- 2026-10-07 06:12Z · dev · AC1-9 done, PR #6 lists the test per AC. Test: make up; make test-integration; newman run postman/auth.postman_collection.json against SMEM_ENV=dev API (2 counted registers per run, 5/h limit: restart API after 2 runs). New dep: golang.org/x/crypto (argon2). Gaps: no Unicode normalisation of passwords (needs x/text, leader decision); limiter per process.
- 2026-10-07 06:12Z · dev · Leader FYI: new dependency golang.org/x/crypto (argon2id, unavoidable for AC2). Follow-up to decide: NFC/NFKC normalisation of passwords for Vietnamese diacritics needs golang.org/x/text/unicode/norm.
- 2026-10-07 06:13Z · leader · Review before QA (no defects found so far; two changes needed before QA tests it). 1) Unicode normalisation, decided now because it cannot change after users exist: normalise the password to NFKC before the length rules and before hashing AND before verifying at login; normalise the display name to NFC (then trim, then reject control characters). Use golang.org/x/text/unicode/norm (x/text is already in go.mod via the T-005 spike and stays). Length limits count characters after normalisation. Tests: register with an NFC Vietnamese password and log in with its NFD form (and the reverse), the 10-character minimum counts after normalisation, the display name is stored as NFC, a full-width-digit password logs in with its ASCII form (NFKC). Mention the rule in the OpenAPI field descriptions. 2) Merge origin/develop (T-028 merged, a0d9429): expect a textual conflict in TestMigrateCycle in internal/db/integration_test.go; keep your rollback change and T-028's Close() fixes; re-run make lint build test test-integration and the Postman collection twice. The golang.org/x/crypto dependency (argon2) is approved. Then READY_FOR_QA.
- 2026-10-07 06:13Z · leader · Leader FYI for QA later: the register rate limit is 5/hour per IP, and each Newman run makes 2 counted registers, so run the collection twice back to back at most, or restart the API or raise SMEM_RATE_REGISTER_PER_HOUR before a third run.
- 2026-10-07 06:18Z · dev · Rework 1. (1) L-09 normalisation: normalizePassword (NFKC, x/text/unicode/norm) runs before length rules, hashing and verifying in Register and Login (internal/auth/validate.go, service.go); display name NFC then trim then control check. Tests: unit TestNormalizePasswordNFKC, TestValidPasswordCountsAfterNormalisation, TestCleanDisplayNameIsNFC; integration TestUnicodeNormalisation (NFC register/NFD login and reverse, hash verifies over NFKC form, full-width digits, 10-char min after normalisation, display name NFC in response and DB). OpenAPI password/display_name descriptions state the rule. Postman step 5 logs in with the NFD form. (2) Merged origin/develop (T-028): go.mod/go.sum union (x/crypto added), openapi responses union; TestMigrateCycle auto-merged cleanly (rollback change + Close() fixes kept). make lint build test test-integration green; Newman x2 on a throwaway DB: 30 requests, 82 assertions, 0 failures each; throwaway DB dropped. PR body updated with per-AC mapping. QA: test normalisation with raw bytes via curl too.
- 2026-10-07 06:28Z · qa · Fresh clone of PR #6 head e9dc7c2 (no CI checks exist on the branch). Throwaway DB smem_test_qa_t006 (see ENV NOTE), migrated up, 8 API instances on 127.0.0.1:18081-18089 with different env, DB dropped and instances killed afterwards; 'smemories' DB never touched. EVIDENCE BY AC. AC1: register trims+lower-cases ('  Alice@Example.COM ' -> alice@example.com, name '  Alice  ' -> 'Alice'), 201 + cookie; duplicate 'ALICE@example.com' -> 409 email_taken; 40-case validation matrix: invalid_email for no-dot domain, trailing/leading-dot domain, 'Bob <x@y.com>', comment, 2 addresses, empty, spaces, quoted local, newline, NUL, 255 chars (254 ok); weak_password for 9 chars, pw==email (also upper-case), 9 emoji (10 emoji ok), 10 spaces ok; invalid_display_name for empty, spaces, 101 chars, tab, LF, NUL, DEL, C1, lone surrogate (100 chars/100 emoji ok). AC2: DB row is $argon2id$v=19$m=19456,t=2,p=1$<16B salt>$<32B key>; startup rejects SMEM_AUTH_MAX_CONCURRENT_HASHES=0, ARGON_MEMORY_KIB=4, bad TRUST_PROXY; 50 parallel logins (cap 4) -> 50x200 in 0.38 s, 50 unknown-email -> 50x401 0.40 s, RSS stable ~190 MB over 5 more bursts; cap=1 + 128 MiB/t=3: 50 parallel -> 9x401 + 41x503 (Retry-After: 2), max latency 2.29 s, no crash, healthz 200 after. AC3: wrong pw / unknown email / case-variant email give byte-identical responses (md5 equal after stripping request id); timing 20 each, sequential: wrong-pw median 37.0 ms (sd 8.6) vs unknown-email 35.9 ms (sd 8.2); login issues a fresh token every time (T1!=T2!=T3), an attacker-chosen cookie is never adopted, and logging in while holding an old cookie revokes it (200 -> 401). AC4: token base64url 43 chars; DB stores sha256 only (hex matches my own sha256 of the cookie); cookie 'Path=/; Max-Age=2592000; HttpOnly; SameSite=Lax', Secure present for SMEM_ENV=prod and test, absent for dev. AC5: /v1/me 200 / 401 without cookie, bogus cookie, expired session (DB expires_at set in the past); sliding: 20 days left -> no Set-Cookie, 10 days left -> Set-Cookie with same token and expiry moved to +30d; logout 204 + cleared cookie, replay of the old cookie after logout 401, second logout 204, logout with no cookie 204; each login sweeps the user's expired rows (1 -> 0). AC6: login: 10 failures per ip+email, 11th -> 429 rate_limited + Retry-After 900, correct password while locked also 429, other email still 401, case-variant email shares the bucket; per-IP: exactly 100th failure then 429; register: 5/h, 6th -> 429 Retry-After 3592 (invalid input does not consume); X-Forwarded-For rotated on every request is ignored with TRUST_PROXY off (still 429); with TRUST_PROXY=true the last hop is the key (rotating the first hop does not evade, a new last hop gets its own budget, garbage XFF falls back to RemoteAddr, IPv6 ok). AC7: with a session cookie: no Origin 403, evil 403, 'null' 403, other port/scheme 403, matching 401 (passes the guard), Referer-only matching passes, Referer prefix trick 'http://localhost:5173.evil.example' 403, userinfo trick 403, wrong Origin + matching Referer 403; in prod only https://app.example.com passes; a rejected logout leaves the session alive; 415 for form-encoded, text/plain, text/json, multipart, application/jsonx, missing Content-Type, chunked text; 'application/json; charset=utf-8' and 'APPLICATION/JSON' accepted; 413 for a 1 MiB+ body. AC8: grepped all server logs (about 1000 lines, plus a forced 500 by renaming the sessions table) for the passwords, argon2id, tokens, Set-Cookie, emails, injected strings: 0 hits, access log carries only method/route/status/duration; user object has exactly id,email,email_verified,display_name,locale,created_at; 1,000,000-char and 129-char passwords -> 400 weak_password in 5 ms (register) and 401 in 5 ms (login), no hashing; 128 ok. AC9: openapi-typescript parses api/openapi.yaml; newman run twice back to back on a fresh API: 30 requests / 82 assertions / 0 failures each time; third run would hit the 5/h limit as announced. OTHER: SQL-injection strings (9 variants) in email/password/display name/cookie/User-Agent: no 500, no match, tables intact; mass assignment (id, email_verified, locale, created_at in register body) ignored (DB: locale en, verified NULL, server ULID); new Unicode rules (L-09): NFC register / NFD login and reverse, full-width digits == ASCII, 9 NFC chars written in NFD (27 code points) rejected while 10 accepted, 128 chars as NFD accepted / 129 rejected, 5 ligatures count as 10, fw-form of email as password rejected, display name stored as NFC (hex E1BB85 for the e-circumflex-tilde), 100 NFD-expanded chars accepted / 101 rejected; wrong methods 405 with Allow; migration down/up round trip on the throwaway DB ok. Mechanical: make lint build test rc 0; make test-integration rc 0 and the verbose run showed 38 auth tests PASS 0 SKIP (run once, before the DB grant fault, see below); unit tests 3 more times with -race: green. READ-THROUGH of password.go, service.go, csrf.go, handler.go, store.go, ratelimit.go: no vulnerability found (parameterised SQL, constant-time compare, stored-hash parameters capped, ErrBusy refunds the budget, semaphore released on defer). NON-BLOCKING OBSERVATIONS, for the leader to weigh (1 and 2 cannot be changed cheaply once users exist): (1) email is not Unicode-normalised: 'nguyễn@example.com' in NFC and NFD (and a full-width 'ｅ' lookalike) register as separate accounts, and rate-limit keys differ; consider NFKC/NFC on the email like L-09 does for the password, before real users exist. (2) display name check is IsControl only: zero-width space U+200B, RLO U+202E and a name made only of U+200B are accepted (invisible/spoofable name); consider rejecting Unicode category Cf and requiring one visible character. (3) local part > 64 characters is accepted (spec only caps the whole address at 254). (4) Login CSRF: without a session cookie no Origin check applies (per AC7), so a cross-site form-less fetch cannot log a victim in only because of the 415 JSON rule; fine, noted. (5) Rate limiter keys IPv6 per /128; one /64 host can rotate addresses (per-process limiter ceiling is already documented). (6) A 10-failure lockout applies to the correct password too (ip+email scope, expected). (7) OpenAPI lists no 500 and /v1/me no 405; trivial. (8) HEADS-UP merge order with PR #4 (T-003): textually clean merge, but web 'npm run check:api' fails afterwards because web/src/api/schema.d.ts must be regenerated (cd web && npm run gen:api) by whoever merges second. ENV NOTE: creating smem_test_qa_t006 as user smemories failed with Error 1044 (no privilege on smem_test_%), and at the end 'make test-integration' failed the same way (create smem_test_<hex>: Access denied) while it had passed earlier in this run; so integration re-runs for flakiness could not be done. I did NOT touch the grant: I created my throwaway DB with the dev root account from .env.example (docker exec ... mysql -uroot), ran the API against it, and dropped it (also an accidental empty smem_test_qat006 created earlier) afterwards; remaining DBs are information_schema, mysql, performance_schema, smemories, sys.
- 2026-10-07 06:28Z · qa · ENV FAULT (non-blocking, not a T-006 defect, for the leader/owner): on smemories-mysql-1 the account smemories gets 'Error 1044 Access denied ... to database smem_test_*' on CREATE DATABASE (reproduced with mysql CLI and with make test-integration at ~13:30 local). It worked at ~13:21 (make test-integration green). SHOW GRANTS FOR smemories still lists `smem\\_test\\_%` (doubled backslashes) and mysql.db has that row, so the pattern looks mis-escaped or was re-granted differently; docker/mysql/init-test-grants.sh grants `smem\_test\_%`. I did not change grants or the container. Until fixed, make test-integration fails for every task.
- 2026-10-07 06:30Z · leader · My review found no vulnerability; QA_PASS at e9dc7c2 stands for what it covered. One required change and one bundled hardening, so the owner approves once. 1) Required: T-003 merged first, so web/src/api/schema.d.ts is stale against this PR's api/openapi.yaml and make lint (check:api) would fail on develop. Merge origin/develop, run cd web && npm run gen:api, commit the result, re-run make lint build test test-integration and both Postman runs. 2) Hardening, from QA's non-blocking notes, cheap now and expensive once users exist: (a) email: normalise to NFC, then lower-case and trim; reject an address whose local part exceeds 64 bytes and any address containing control or format characters (Unicode categories Cc, Cf); test that the NFC and NFD spellings of the same address cannot both register (second gets 409 email_taken) and that login works with either spelling. (b) display name: after NFC and trim, also reject format characters (category Cf: zero-width space, bidi overrides and isolates, LRM/RLM, BOM) except U+200D (ZWJ, used in emoji sequences), and line/paragraph separators (Zl, Zp); tests for each class including a Vietnamese name with a ZWJ emoji accepted and a bidi-override name rejected. Document both rules in the OpenAPI field descriptions. Keep the diff to these changes; this is the task's last allowed rework round.
- 2026-10-07 06:30Z · leader · Leader note on the environment: the smemories DB account currently lacks the smem_test_% grant on the shared compose MySQL (Error 1044 on CREATE DATABASE), so make test-integration fails with the default DSN. Do NOT change grants or touch the shared containers. To run the integration tests, point SMEM_TEST_DB_DSN at the dev root account (the dev-only value in .env.example), which can create the throwaway smem_test_* databases; drop what you create. The owner will restore the grant separately.
- 2026-10-07 06:33Z · dev · Rework 2 (head 77924c1). (1) Merged origin/develop, ran cd web && npm run gen:api, committed schema.d.ts (npm run check:api passes inside make lint). (2a) Email: NFC + lower + trim (validate.go normalizeEmail, also used by login); local part > 64 bytes and any Cc/Cf char -> invalid_email. Tests: unit TestNormalizeEmailNFC, TestValidEmail (64/65 bytes, 32/33 two-byte chars, U+200B, U+202E, BOM, C0, C1; the 254-char case now uses a 64-byte local part); integration TestEmailNFCvsNFD (NFD register after NFC -> 409 email_taken, 1 row, stored NFC, login with either spelling) and TestEmailAndDisplayNameHardening. (2b) Display name: also rejects Cf except U+200D, Zl, Zp. Tests: TestCleanDisplayName (ZWSP, only-ZWSP, RLO, isolates, LRM/RLM, BOM, U+2028, U+2029, C1 rejected; Vietnamese + ZWJ emoji accepted) and integration. OpenAPI: email/display_name descriptions on register, login email note. Also fixed a pre-existing data race in TestParallelLoginsRespectTheHashCap (maxInFlight read without the mutex; showed up once under -race). Ran with SMEM_TEST_DB_DSN=dev root (grant fault): make lint build test test-integration green; internal/auth integration x3 green; Newman x2 on a throwaway DB (SMEM_ENV=dev): 30 requests / 82 assertions / 0 failures each; DB dropped, API stopped. Known gap: a display name consisting only of ZWJ (U+200D) is still accepted (not in the spec; say if you want a visible-character rule).
- 2026-10-07 06:33Z · dev · Rework 2 pushed to PR #6 (77924c1); see status note. Heads-up: dev root DSN used for integration tests because of the smem_test_% grant fault; nothing left behind in MySQL.
- 2026-10-07 06:46Z · qa · Round 2 (head 77924c1, fresh clone). Delta e9dc7c2..77924c1 read: internal/auth/validate.go, validate_test.go, integration_test.go, api/openapi.yaml, Makefile, web (develop merge). EVIDENCE. (1) Merge: merging current origin/develop (5925b31) into the PR head is clean (only .team/README.md differs); PR vs develop diff contains no .team files. 'make lint build test' rc 0 (gofmt, vet, eslint, tsc, i18n, 'npm run check:api' = 'api schema ok' so schema.d.ts matches openapi.yaml; go test -race all ok; vitest 25/25). 'make test-integration' with SMEM_TEST_DB_DSN=dev root: all packages ok. (2) Live curl on binary vs throwaway DB smem_test_qa_t006b (root account, dropped afterwards; show databases = information_schema, mysql, performance_schema, smemories, sys; API processes killed). EMAIL: NFC 'nguyễn@example.com' 201; NFD spelling 409 email_taken; ' NGUYE+U0302+U0303N@Example.COM ' 409 email_taken; login 200 with NFC and with NFD; DB stores hex 6E677579E1BB856E = NFC, 1 row. Local part 64 bytes 201, 65 bytes 400 invalid_email; 32 x 2-byte chars 201, 33 x 2-byte 400; ZWSP, RLO, leading BOM, soft hyphen (Cf), ZWJ in email, SOH (Cc), U+0085 (Cc) all 400 invalid_email; total 254 chars 201, 255 400. DISPLAY NAME: accepted: Vietnamese (NFC and NFD input), woman-woman-girl ZWJ family emoji, man-technologist ZWJ. Rejected 400 invalid_display_name: ZWSP, name of only ZWSP, RLO, LRO, RLE, LRE, PDF, LRI, RLI, FSI, PDI, LRM, RLM, BOM, word joiner U+2060, soft hyphen, U+2028, U+2029, ALM U+061C, TAB, tag char U+E0041, ZWNJ U+200C. Tests: TestNormalizeEmailNFC, TestValidEmail (64/65 bytes, 32/33 two-byte, Cc/Cf cases), TestCleanDisplayName, integration TestEmailNFCvsNFD and TestEmailAndDisplayNameHardening: 'go test -race -tags integration -count=5' on those plus TestParallelLogins and TestUnicodeNormalisation all PASS 5/5. (3) NO REGRESSION, re-run live: wrong-password vs unknown-email identical response (headers+body, only request_id differs); timing 8 sequential each, two rounds on restarted instances: median 27.3/26.3 ms (known) vs 24.9/25.0 ms (unknown); password NFC register / NFD / NFKD login 200, wrong 401; prod cookie 'Path=/; Max-Age=2592000; HttpOnly; Secure; SameSite=Lax', dev without Secure; CSRF with session cookie: no origin 403, evil 403, 'null' 403, http vs https 403, other port 403, Referer prefix trick app.example.com.evil.example 403, allowed Origin 204, allowed Referer 204; 415 for form, text/plain, multipart, missing Content-Type; 1 MiB+ body 413; 129-char password 400 in 0.5 ms; register limit: 5x201 then 429 Retry-After 3600; login: 10 failures 401 then 429; server logs (121 lines across all instances) 0 hits for passwords, argon2, session token, Set-Cookie. Postman via newman 6.2.2 twice back to back on one API: 30 requests / 82 assertions / 0 failures both times. TestParallelLoginsRespectTheHashCap: diff only adds mu.Lock()/defer mu.Unlock() before the 'maxInFlight > 2' check (fixes a read race with the sampler); the 50 goroutines, cap=2, 503-required and 'maxInFlight > 2' assertions are unchanged, so the test is not weakened. NON-BLOCKING: (a) a display name consisting only of ZWJ U+200D is accepted (dev already flagged; invisible name, consider a visible-character rule later); (b) the Postman collection was not extended for the new rules (no NFD-email / ZWSP-name / 65-byte-local-part entries); the same endpoints and error codes are covered by Go tests, so I did not fail on it; (c) PR branch has no CI checks yet (T-004 pending). ENV: make test-integration needs SMEM_TEST_DB_DSN=root DSN until the smem_test_% grant is restored (not a T-006 defect).
- 2026-10-07 07:07Z · leader · Leader review OK at 77924c1 (QA_PASS, same head). Read the whole auth path and the rework delta: all queries parameterised, session tokens stored only as SHA-256, cookie flags correct, CSRF origin check and JSON-only bodies, password hashing with parameter caps and a concurrency cap, dummy verification keeps unknown-email and wrong-password logins indistinguishable, oversize passwords rejected before hashing, rate limits counted up front and refunded on success, NFKC passwords, NFC emails and names, Cc/Cf rejected. No vulnerability found. Awaiting owner approval (Risk: high). Merge plan: T-004 first so this PR gets real CI before it merges (close and reopen the PR to trigger the new workflow). Non-blocking: an all-ZWJ display name is still accepted (folded into T-031), and the Postman collection has no entries for the new email and name rules.
- 2026-10-07 07:37Z · leader · Not a QA or review defect: the base changed after QA. T-004 (CI, merged 61439ff) turned on gosec and bodyclose, and a simulated merge of develop plus this PR (clean textual merge) fails make lint with 6 gosec G115 findings (integer overflow conversion): internal/auth/password.go:107 uint32(len(want)); internal/auth/store.go:72 uint64(id) from LastInsertId; internal/config/config.go:117 uint32(n), :121 uint32(n), :125 uint8(n) for the argon parameters; internal/ulid/ulid.go:16 uint64(t.UnixMilli()). Merged as is, CI on develop would go red. Required: 1) merge origin/develop into the branch (no force-push); 2) fix each finding with an explicit range check before the conversion (return an error or clamp where the value is invalid: negative ids and timestamps, parameter values above the type's maximum, a stored key length above a sane bound) so the conversion is provably safe; where a check already exists, add the narrowest //nolint:gosec comment with the reason naming that check. Do NOT disable G115 or change .golangci.yml or the workflow. 3) make lint build test test-integration must be green with golangci-lint included (it is part of make lint now; install v2.8.0 as the README says), plus the Postman collection twice. 4) Change nothing else, then READY_FOR_QA; QA verifies the delta and runs the full suite.
- 2026-10-07 07:37Z · leader · Leader reset of the rework budget: this round comes from the new CI rules landing after QA, not from repeated defects. The branch, PR #6 and worktree are reused; claim it like a normal task. The owner's approval of the merge (Q-004) should wait until QA re-passes the new head.
- 2026-10-07 07:37Z · leader · Leader FYI for the dev: golangci-lint is on PATH at ~/go/bin/golangci-lint (v2.8.0) if it is not found; run make lint from your task worktree.
- 2026-10-07 07:45Z · human · owner approved merge
- 2026-10-07 07:45Z · leader · Leader: the owner approved this merge at 07:45Z (bin/team approve T-006), after the gosec findings were reported. I merge only when all of these hold: 1) the dev's fix lands (range checks for the six G115 findings, nothing else); 2) QA re-passes the new head; 3) the CI jobs go, go-integration, web and security are green on the PR (if GitHub does not start a run for the PR, I run the full make lint build test on a simulated merge of develop plus the PR instead); 4) my review of the delta finds nothing beyond range checks. If the fix changes any auth behaviour, I ask the owner again before merging.
- 2026-10-07 11:00Z · dev · Rework 3 (CI gosec only): merged origin/develop, fixed the 6 G115 findings and nothing else. password.go: key len > 1024 rejected, nolint names that check (test case in TestVerifyRejectsMalformedHashes); store.go: id<=0 -> error; config.go x3: nolint, ranges already checked; ulid.go: pre-1970 clamps to 0 (TestPreEpochTimeClampsToZero). make lint (golangci-lint v2.8.0) build test test-integration green (dev root DSN); Newman x2 30 req/82 assert/0 failures; throwaway DB dropped. PR body has an updated section. Delta is one commit on top of the merge.
- 2026-10-07 11:07Z · qa · Round 3 (delta 77924c1..06a970d, fresh clone of PR #6 head 06a970d, gosec G115 fixes). EVIDENCE. (1) DELTA: 'git diff 77924c1 06a970d -- internal/auth internal/config internal/ulid' = exactly 6 files, 23+/8-: password.go (maxStoredKeyLen=1024 bound added to the existing empty/decode check + one narrow nolint:gosec), password_test.go (one malformed-hash case), store.go (id<=0 -> error before uint64(id)), config.go (three narrow nolint:gosec, no logic change), ulid.go (ms<0 clamp to 0, then nolint on uint64), ulid_test.go (TestPreEpochTimeClampsToZero). Rest of the PR-vs-77924c1 diff is develop merged in: .github/dependabot.yml, ci.yml, .golangci.yml, docs/ci.md, Makefile (golangci-lint line), README.md, httpx/server_test.go (bodyclose fix), .team/README.md; 'git diff origin/develop pr6' outside auth/migrations/api/postman/web shows only T-006 feature files, nothing foreign. (2) NOLINT COMMENTS TRUE: config.go checks sit on the line directly above each conversion and return an error: MEMORY_KIB 'err!=nil || n>1<<20' with getInt min 8, TIME 'n>100' min 1, PARALLELISM 'n>255' min 1. I ran config.Load with each value via a scratch program (SMEM_ENV=dev, fake DSN). MEMORY_KIB: 0 REJECT, 1 REJECT, 7 REJECT, 8 ok, 100 ok, 101 ok, 255 ok, 256 ok, 1048576 ok, 1048577 REJECT; also -1, 4294967296, 99999999999999999999, abc all REJECT (message 'want an integer between 8 and 1048576'). TIME: 0 REJECT, 1 ok, 7 ok, 8 ok, 100 ok, 101 REJECT, 255 256 1048576 1048577 -1 4294967296 huge abc all REJECT. PARALLELISM: 0 REJECT, 1 7 8 100 101 255 ok (stored as 1,7,8,100,101,255), 256 REJECT, 1048576 1048577 -1 4294967296 huge abc REJECT. Empty = default (19456/2/1). password.go: len(want)==0 or >1024 returns 'auth: bad argon2 encoding' before uint32(len(want)); store.go: id<=0 returns an error before uint64(id); ulid: ms<0 clamped before uint64. (3) UNCHANGED BEHAVIOUR: golangci-lint v2.8.0 'make lint' rc 0 (gofmt, vet, vet -tags integration, golangci-lint '0 issues.', also 0 issues with --build-tags integration, web lint/check:api); 'make build' rc 0; 'make test' (go test -race ./..., vitest 25/25) green; 'make test-integration' with SMEM_TEST_DB_DSN=dev root DSN from .env.example: all packages ok (-race -count=1 -tags integration). Postman via newman 6.2.2 against the built API on throwaway DB smem_test_qa_t006c (SMEM_ENV=dev, 127.0.0.1:18090), twice back to back: 30 requests / 82 assertions / 0 failures both runs. Live curl: register 201 + cookie (HttpOnly, SameSite=Lax, Path=/, Max-Age 2592000, no Secure in dev) with email lower-cased; /v1/me 200; logout 204 + cookie cleared; /v1/me after logout 401; login with different-case email 200 and /v1/me 200; wrong password 401 invalid_credentials. API log (69 lines): 0 hits for the password, argon2id, smem_session. (4) CI: gh pr checks 6 = go, go-integration, security, web, dependabot all pass on the run for this head; the golangci-lint step in the go job: 'golangci-lint found no issues', '0 issues.', no warnings (only a git 'hint' line from checkout). (5) OVERSIZE KEY / PRE-1970: scratch tests (not committed, removed): Verify with stored key of 1,1023,1024 bytes proceeds to hashing (ok=false err=nil), 1025 and 1026 bytes -> 'auth: bad argon2 encoding' in about 20 microseconds; a 100000-char key string is rejected with 377 KiB total allocation (no spike, no panic), 5 KiB for the ~1 KB case. Note the column password_hash is varchar(255), so a >1024-byte key cannot even be stored via the app; a crafted DB row is therefore impossible, the guard is defence in depth and the unit case (strings.Repeat QQ x1025 -> 1537 bytes) is in TestVerifyRejectsMalformedHashes. ulid.New: time.Time{} (year 1), UnixMilli(-1), Unix(-2^40) all give timestamp prefix 0000000000 (random 16-char tail kept, so the ULID is the zero-TIME one, not an all-zero string), UnixMilli(0) prefix 0000000000, UnixMilli(1) prefix 0000000001. Cleanup: API stopped, smem_test_qa_t006c dropped (show databases = information_schema, mysql, performance_schema, smemories, sys). Nothing touched on the shared containers except the throwaway DB created/dropped via root. NON-BLOCKING: none new (earlier ones stand: all-ZWJ display name, Postman has no entries for NFD-email/ZWSP-name rules). No auth behaviour changed for valid data.
- 2026-10-07 11:12Z · leader · merged ff48f5b (squash), owner-approved at 07:45Z; merge conditions met: gosec fix landed with only the six changes, QA re-passed at 06a970d, CI green on the PR (go, go-integration, web, security), delta reviewed. For owner to check: with the stack up and make run, curl -i -X POST localhost:8080/v1/auth/register -H 'Content-Type: application/json' -d '{"email":"you@example.com","password":"a long test password","display_name":"Test"}' then GET /v1/me with the cookie. Follow-ups: T-031 (go-live hardening, incl. all-ZWJ display names).
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-007 — Email verification and password reset
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-006
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-007-email-verification-and-password-reset
- **PR:** https://github.com/danyaa666/smemories/pull/18
- **Updated:** 2026-10-08 02:35Z by leader
- **Comments-seen:** 8

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/03-email-verification-and-password-reset.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

Email verification and password reset for the in-house auth from T-006, plus the mailer abstraction they need.

#### Comments
- 2026-10-07 11:38Z · leader · Leader note: migration numbering. T-008 (PR #17) uses migration 0004, so use the NEXT FREE number at the time you write yours (0005 if T-008 has merged), not the 0003 the spec mentions: goose refuses an out-of-order lower number on databases that already applied a higher one.
- 2026-10-07 14:14Z · leader · Spec updated (now in .team/epics/E02-auth/03-email-verification-and-password-reset.md): the migration file name is now '<next free number>_email_tokens.sql' instead of 0003; see the numbering note at the top of the spec.
- 2026-10-07 17:07Z · dev · Implemented AC1-AC9, migration 0005_email_tokens. Test: make test-integration (new file internal/auth/verify_reset_integration_test.go), newman run postman/auth.postman_collection.json (restart API between 3rd runs: forgot is 5/h per IP). Gaps: text-only emails (no HTML part); resend mailer failure returns 500.
- 2026-10-07 17:11Z · qa · PR #18 head 3f6c9aa, CI green (go, go-integration, security, web). Mechanical: go build, go test -race ./..., make test-integration (all ok), go vet -tags integration, golangci-lint 0 issues, make web-lint ok. Live: built API, fresh DB smem_test_qa007 + migrations (0005 applies), curl + newman. Evidence AC->result: AC1 SMEM_ENV=prod -> exit 'mailer setup failed ... no real mailer configured'; dev prints full message with link. AC2 register -> 201 + verification mail logged (mailer-failure path covered by TestRegisterSurvivesMailerFailure). AC3 resend: 202,202,202,429 (Retry-After 3600); verified user 200 {already_verified:true}; unauthenticated 401. AC4 verify -> 204, email_verified_at set; reuse/unknown/empty/garbage -> 400 invalid_token; DB token_hash == sha256(token), 32 bytes decoded, expires-created 24h by tests; a verify token used as reset -> 400 invalid_token. AC5 known vs unknown: identical 202 {} body, timing ~1.2ms both; limits: 5/h IP -> 429 on 6th, per-email 3/h (case-insensitive) -> 429 on 4th. AC6 weak password -> 400 and token NOT consumed; 8 concurrent resets of one token -> exactly 1x204 + 7x400; all 3 sessions deleted, old cookies 401; other outstanding reset links retired; only the winning password logs in; expired token (expires_at moved to past) -> 400; reset TTL = 60 min. AC7 VI user gets Vietnamese mail with /reset-password?token= link built from SMEM_PUBLIC_BASE_URL (trailing slash trimmed); EN default; TestRenderEmailBothLanguages. AC8 startup cleanup logged 'removed dead email tokens' count 4 and 7 on restarts; live tokens kept; TestCleanupRemovesExpiredAndUsedTokens. AC9 openapi has 200/202/204/400/401/429 for the 4 endpoints; newman auth collection run twice back to back: 45 requests, 110 assertions, 0 failed both times (new flow + edge entries present). No token seen in structured logs. Non-blocking: (1) resend returns 500 when mailer fails (dev-noted; maybe 502/202 + log); (2) Postman has no 429 case for new limits (needs restart between runs, noted by dev); (3) per-email forgot limit lets anyone throttle a victim's reset to 3/h (accepted by spec); (4) text-only email. Test DB dropped.
- 2026-10-07 17:13Z · leader · Leader review OK on head 3f6c9aa (same head QA tested; CI go, go-integration, web, security green): spec ACs met by the diff; tokens 32 random bytes, SHA-256 at rest, single use via one UPDATE with row lock, wrong-purpose rejected; known and unknown forgot-password do the same work on the request path; reset deletes all sessions in one transaction; LogMailer refuses to exist outside dev/test; cleanup at startup and daily; limits swept. Non-blocking, moved to specs: resend 500 on mailer failure and register-time send on the request path (T-021), link-token handling on the web pages (T-015), per-email forgot cap can throttle a victim (T-031). Awaiting owner approval. For the owner to check: README 'Auth' paragraph (new SMEM_PUBLIC_BASE_URL, required in prod) and that registering locally prints the verification link in the API log.
- 2026-10-08 01:30Z · human · owner approved merge
- 2026-10-08 01:31Z · leader · merged 186ac52 (squash), owner-approved in chat 2026-10-08. Review: tokens are 32 random bytes hashed at rest and single use (one UPDATE with row lock), forgot-password does the same work for known and unknown emails, reset deletes all sessions in one transaction, LogMailer cannot start outside dev/test. For owner to check: register locally and read the verification link in the API log; SMEM_PUBLIC_BASE_URL is required in prod. Follow-ups recorded in T-015, T-021, T-031 specs.
- 2026-10-08 02:35Z · leader · accepted by owner (chat, 2026-10-08: 'team accept T-007')

### T-008 — Yearbook CRUD and profile information
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-006
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-008-yearbook-crud-and-profile-information
- **PR:** https://github.com/danyaa666/smemories/pull/17
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E03-yearbooks/01-yearbook-crud-and-profile-information.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E03-yearbooks · **PRD:** `.team/epics/E03-yearbooks/PRD.md`

The yearbook itself: create, list, read, update and delete a user's yearbooks, plus the owner's profile page information.

#### Comments
- 2026-10-07 11:37Z · dev · Yearbook+profile CRUD done, CI green (go, go-integration, web, security). Test: make lint; make test; SMEM_TEST_DB_DSN=<dev root> go test -tags integration ./internal/yearbook; newman run postman/yearbooks.postman_collection.json (85 req, run twice OK). Notes: plain database/sql (no sqlc in repo); shared textx.Clean + httpx.DecodeJSON(strict); birthday >= 1900 and strictly past; migration 0004 vs T-007's planned 0003 -> goose out-of-order on dev DBs that already applied 0004 (merge T-007 first or reset dev DB).
- 2026-10-07 11:42Z · qa · QA_PASS on PR #17 head 0c252d8 (fresh worktree, up to date). Evidence I produced:
  MECHANICAL: make lint (gofmt, vet x2, golangci-lint v2.8.0, eslint, tsc, i18n, check:api) exit 0; make build OK; make test (go -race + vitest 25) green; make test-integration x2 green (dev root DSN); gh pr checks 17: go, go-integration, security, web all pass.
  POSTMAN (newman, live binary on throwaway DB smem_test_qa_t008): yearbooks 85 req / 172 assertions, 0 failed, run twice back to back; auth 30 req / 82 assertions 0 failed x2; platform only fails the documented manual 'stop the DB first' step.
  MIGRATION: up (0,1,2,4) -> down (yearbooks+profiles dropped, users/sessions/app_meta intact) -> up OK. No orphan profiles, owner_flag unique key holds. DB dropped afterwards; no smem_test_* leftovers on the shared MySQL (only information_schema, mysql, performance_schema, smemories, sys).
  AC1 create 201, profile auto-created with full_name = display name (also NFC for NFD display name, emoji ZWJ name), page_size default A5, language en|vi enforced, missing title/language -> invalid_title/invalid_language.
  AC2 list: own books only, updated_at desc (verified), default 20, limit 1..50 else 400 invalid_limit (0,-1,51,abc,1.5,huge), 3 pages of 7 = 20 unique and identical order to a one-shot list, no next_cursor at exactly 20. Cursor tampering (garbage, negative, huge, SQL text, std base64, 100KB) -> 400 invalid_cursor or an empty page; forged cursor holding Bob's book id used by Alice returns only Alice's rows; Alice's cursor used by Bob returns nothing of Alice's.
  AC3/AC7 authorization matrix (A vs B, valid and invalid bodies, GET/PATCH x4 bodies/PUT profile x3/DELETE): Bob's request on Alice's book returns byte-identical status+body (modulo request_id) to a nonexistent ULID, a non-ULID and '1' (SQLi-ish id too): 404 not_found. Alice's book unchanged afterwards. Wrong-verb routes give identical 405 for all ids. Unauthenticated and garbage-cookie: 401 unauthenticated on all 6 endpoints, identical bodies. Responses contain no numeric ids (only ULID id/profile.id).
  AC4/mass assignment: owner_id, id, public_id, is_owner, created_at, updated_at, template_id, profile, internal_id on create/PATCH/PUT -> 400 unknown_field; is_owner on PUT profile -> 400. PATCH null graduation_year clears it; PUT omits clear optional fields.
  AC5 boundaries (all pass at limit and fail at limit+1 with invalid_<field>): title 120/121 (ASCII, Vietnamese, emoji counted as characters), school/class 120, motto 200, quote 500, hobbies 300, future plans 300, full name 1/100/empty/missing, nickname 50, grad year 1949/1950/2100/2101/0/-5/65536, birthday 1900-01-01 ok, 1899-12-31, bad dates (2001-02-29, 0000, formats, datetime) rejected, today and tomorrow rejected, yesterday ok, leap day ok. Trim, NFD -> NFC stored, NUL/newline/tab/ZWSP/BOM/RLO/LS/lone surrogate/invalid UTF-8 rejected, ZWJ family emoji accepted. Vietnamese+emoji round trip identical.
  AC6: DELETE 204, book and profile gone from DB (cascade), second delete 404, recreate works. 30 parallel creates by one user: exactly 20 x 201 and 10 x 409 limit_reached, list shows 20; other user unaffected. 90 mixed concurrent PATCH/PUT/GET: all 200; delete vs patch race: 1x204 + 39x404; no 500s, zero ERROR lines in the server log.
  HTTP hygiene: wrong/missing Content-Type (text/plain, form, none, merge-patch+json) -> 415 (also on other user's book, so no leak); malformed/trailing data/array/empty -> 400 invalid_body; 1 MiB body -> 413 payload_too_large; foreign/missing/null Origin on POST/PUT/PATCH/DELETE with a cookie -> 403 csrf_origin_mismatch; GET unaffected. Server log has no field values (no birthday/names/quotes/passwords).
  AUTH REGRESSION (httpx.DecodeJSON strict=false, textx.Clean): NFD display name stored NFC, ZWSP/RLO/blank rejected invalid_display_name, ZWJ emoji accepted, 100/101 chars boundary, unknown extra field still accepted as before, trailing data 400 invalid_body, 1 MiB 413, malformed 400, text/plain 415; auth Postman 30/30 twice.
  AC8: openapi.yaml documents all 6 operations with the limits (check:api passes); Postman collection covers cross-user 404 for GET/PATCH/PUT/DELETE + list, 401s, boundaries, CSRF, limit 409, and runs twice.
  NON-BLOCKING observations: (1) PATCH with body 'null' or {} is a 200 no-op that still bumps updated_at (moves the book to the top of the list); harmless. (2) Duplicate JSON keys (last wins) and case-insensitive key matching ('TITLE' accepted) are standard encoding/json behaviour; unknown-field check still applies to differently named keys. (3) Migration number 0004 vs T-007's planned 0003: dev DBs that already applied 0004 will make goose reject an out-of-order 0003 (L-11); merge order or a DB reset needed. (4) Birthday 'strictly past' uses the UTC date, so a user ahead of UTC can't enter 'today' for a few hours; irrelevant for realistic birthdays. No test files added; dev's unit+integration tests (ownership matrix, limit, boundaries) already cover what I probed.
- 2026-10-07 11:44Z · leader · merged a6e621d (squash). Review: every read, update and delete scoped by owner_id and public_id; the 20-book limit takes a row lock on the user inside a transaction (held at exactly 20 under 30 parallel creates); QA's authorization matrix is byte-identical 404 for foreign, nonexistent and malformed ids. Shared helpers (textx.Clean, httpx.DecodeJSON) replaced auth's private copies with no auth regression. For owner to check: sign in, then POST /v1/yearbooks and GET /v1/yearbooks (see postman/yearbooks.postman_collection.json). Non-blocking: PATCH {} is a 200 no-op that still bumps updated_at; birthday 'strictly past' uses the UTC date.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-009 — Photo upload and storage (MinIO/S3)
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-004, T-008
- **Risk:** high
- **Rework:** 1
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-009-photo-upload-and-storage-minio-s3
- **PR:** https://github.com/danyaa666/smemories/pull/19
- **Updated:** 2026-10-08 02:46Z by leader
- **Comments-seen:** 12

**Spec — read this first, it is the source of truth:** `.team/epics/E03-yearbooks/02-photo-upload-and-storage-minio-s3.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E03-yearbooks · **PRD:** `.team/epics/E03-yearbooks/PRD.md`

Photo upload and storage for yearbook owners.

#### Comments
- 2026-10-07 02:47Z · leader · Leader note from the T-001 review: the server timeouts are ReadTimeout 15 s and WriteTimeout 30 s (global, from T-001). A 10 MiB upload over a slow phone connection, or a streamed download, can exceed them. Do not raise the global values; extend the deadline per route with http.NewResponseController(w).SetReadDeadline / SetWriteDeadline (the statusWriter already implements Unwrap) and add a test. Same applies to the PDF download in T-014.
- 2026-10-07 14:14Z · leader · Spec updated (now in .team/epics/E03-yearbooks/02-photo-upload-and-storage-minio-s3.md): the migration file name is now '<next free number>_media.sql' instead of 0005; see the numbering note at the top of the spec.
- 2026-10-07 17:13Z · leader · Leader: migration number. PR #18 (T-007) is about to take migrations/0005_email_tokens.sql. Use 0006 for your media migration (re-check 'ls migrations/' and git fetch before writing it); two files with the same version would stop goose at startup.
- 2026-10-07 17:29Z · dev · PR #19, all 4 CI checks green (go-integration now runs MinIO). All 9 AC + the per-route deadline note done. Test: make up; make lint test test-integration; cd postman && newman run media.postman_collection.json --working-dir . against a local API (SMEM_S3_* from .env.example). Migration is 0006_media.sql (0005 = T-007). New deps: aws-sdk-go-v2 (aws, credentials, s3). Also added auth.GuardMultipart. Known gaps: PNG eXIf orientation ignored; an upload racing a yearbook delete can leave one orphan object; 413/quota/rate limit covered by Go integration tests only. Risk high: needs owner approval.
- 2026-10-07 17:39Z · qa · QA_PASS T-009 (PR #19, head 21ba7aa, CI 4/4 green). Tested on a FRESH db (0001..0006 applied, migrate down/up of 0006 OK) + real MinIO + real API binary.
  Mechanical: go build, go test -race ./..., go test -race -tags integration -count=1 ./... (incl. MinIO), gofmt, go vet (+integration tag), golangci-lint: all clean. newman media collection (44 req / 77 assertions) run 2x back to back: 0 failures; yearbooks (85/172) and auth (30/82) collections also 0 failures (regression).
  AC1: PASS - 201 {media:{id,width,height,bytes}}; other user / unknown book -> 404; no session 401; foreign Origin 403; JSON body 415; wrong field name 400; 300 MB body -> 413 in 10 ms; 11 MiB junk -> 413; 9.6 MB valid noise JPEG -> 201.
  AC2: PASS - renamed .html/.svg/.exe/.gif/0-byte as .jpg -> 415; polyglot JPEG+<script> -> 201 but stored bytes re-encoded (payload gone); truncated JPEG and animated WebP -> 400 invalid_image; 12000x10 PNG 201, 12001x10 400; 8000x7000 (56 MP, 54 KB file) 400, 7000x7000 (49 MP) 201.
  AC3: PASS - 4000x3000 JPEG with EXIF (Make, Artist, GPS), ICC, COM comment, orientation 6 -> stored 2250x3000, pixels upright (red top / blue bottom), PIL shows no EXIF/ICC/comment, grep for SECRET/Exif/GPS/ICC_PROFILE in display + thumb = none; thumb 360x480 (long edge 480); small images not upscaled; RGBA PNG with alpha stays PNG, opaque PNG becomes JPEG; WebP with EXIF orientation applied; JPEG quality 92.
  AC4: PASS - bucket listing: yearbooks/<ulid>/<ulid>.jpg|png and -thumb.jpg only; upload with filename '../../etc/x"\r\nX-Evil: 1.html' -> keys generated, no echo in any header; bucket anonymous access 403 / policy private.
  AC5: PASS - GET display/thumb: Content-Type from DB, X-Content-Type-Options nosniff, Cache-Control private, max-age=3600, ETag; Range 0-99 -> 206 (100 B), out of range 416, thumb Range 206; owner only (other user 404, no session 401, bad size 400). DELETE: 204, objects and row gone (mc ls), repeated delete 204, other user's DELETE 204 but media untouched, garbage id 204, foreign Origin 403; cover cleared (SET NULL).
  AC6: PASS - 199 rows seeded by SQL: 200th upload 201, 201st 409 quota_exceeded; user bytes quota: exactly 500 MiB reached -> 201, next -> 409; 64 uploads from one user: 60x201 then 429 rate_limited with Retry-After 599.
  AC7: PASS - MinIO stopped: DELETE yearbook -> 502 storage_error, book still readable afterwards; upload with MinIO down -> 502 and afterwards objects == 2 x rows (no orphans either way); after MinIO restart DELETE yearbook -> 204, bucket lists only the other book's objects, media rows removed.
  AC8: PASS - PATCH cover_media_id / PUT profile photo_media_id with own media 200; other book's media, unknown id, malformed id -> 400 invalid_media; number type -> 400; create with cover -> 400 invalid_media; null clears; PATCH of other fields keeps both; PUT profile omitting photo_media_id clears it (documented in OpenAPI as replacing PUT).
  AC9: PASS - ci.yml starts MinIO and go-integration is green; storage + media integration tests pass against local MinIO; OpenAPI covers new routes/codes; postman media collection covers full flow + 24 edge entries, re-runnable.
  Per-route deadlines: 9.6 MB upload throttled to 400 kB/s took 23.9 s (> global 15 s ReadTimeout) -> 201.
  Load: 200 parallel uploads of a 4000x3000 JPEG: 16 accepted (201), the rest instant 503 busy; server stayed up.
  NON-BLOCKING observations: (1) Memory ceiling is high: one 4000x3000 upload leaves RSS ~430 MB; 4 parallel 49 MP PNGs (47 KB each, within spec limits) peaked at 3.2 GB RSS (SMEM_MEDIA_MAX_CONCURRENT=4 default). Spec-compliant, but a small M2 instance could OOM: consider GOMEMLIMIT, a lower pixel cap or a 2-slot default, and a ponytail: comment. (2) Known gaps already declared by dev: PNG eXIf orientation ignored; upload racing a yearbook delete can leave an orphan object. (3) 413/quota/rate-limit are not in the postman collection (Go tests + my manual runs cover them).
  Risk is high: owner approval needed before merge. No test files added by QA (existing coverage adequate).
- 2026-10-07 17:42Z · leader · Leader review OK on head 21ba7aa (the commit QA tested; CI go, go-integration, web, security green). Checked in the diff, not only in the tests: type by sniffing, dimensions from the header before decode, re-encode strips all metadata, generated keys only, owner-only reads with nosniff/private cache, quota recheck under a lock on the user row, upload rate limit, in-flight cap (16) answers 503 busy, yearbook delete purges storage first and fails 502 with nothing deleted, orphans cleaned on failed upload, migration reversible, CI change adds only a MinIO container and fake credentials, no new action. Non-blocking, handled: peak memory 3.2 GB for 4 worst-case PNGs -> new task T-036 (P1, blocks T-034; also sizing note in T-022); aws modules marked '// indirect' in go.mod -> T-036 AC5; orphan sweep -> T-025 spec; PUT profile clears photo_media_id when absent -> T-016 spec. Merge order: T-007 first (its migration is 0005, this one is 0006). Awaiting owner approval.
- 2026-10-08 01:30Z · human · owner approved merge
- 2026-10-08 01:32Z · leader · Leader: T-007 merged to develop (186ac52) and PR #19 now conflicts (api/openapi.yaml, cmd/smemories-api/main.go, internal/config/config_test.go; schema.d.ts merges textually but is stale). No code defect. Please: (1) git fetch; merge origin/develop into the task branch (do not rebase a pushed branch); (2) resolve keeping BOTH sides: T-007's auth/mailer wiring, SMEM_PUBLIC_BASE_URL and email routes, and T-009's media/storage wiring, SMEM_S3_*/SMEM_MEDIA_* and routes; the NewHandler for yearbooks keeps T-009's purger argument; (3) cd web && npm run gen:api and commit schema.d.ts (check:api must pass); (4) ls migrations/ must show 0005_email_tokens.sql and 0006_media.sql; test a fresh DB and one that already applied 0005; (5) make lint build test test-integration, run the auth, yearbooks and media Postman collections twice; (6) push and mark READY_FOR_QA with the merge-commit SHA. Nothing else in scope changes.
- 2026-10-08 01:35Z · dev · Rework done: merged origin/develop (T-007) into the branch, merge commit 845b4c5, head 7d8cfe8 (extra fix: media integration test now passes auth.Mail to NewService). Conflicts resolved keeping both sides (main.go: mailer + media/storage wiring, go svc.RunCleanup, yearbook handler keeps media purger; openapi error code examples include invalid_token + media codes; config tests set SMEM_PUBLIC_BASE_URL and SMEM_S3_BUCKET where prod is used). web: npm run gen:api gives no diff vs merged schema.d.ts, check:api ok. migrations: 0005_email_tokens.sql + 0006_media.sql; tested a DB with 0005 applied (develop) then up/down/up of 0006 OK; fresh DBs via integration tests. make lint build test test-integration green. newman auth (45/110), yearbooks (85/172), media (44/77) each run twice against the merged API: 0 failures. No .team files in PR diff.
- 2026-10-08 01:52Z · qa · QA_PASS T-009 re-verify after merge of develop (T-007); head 7d8cfe8 (merge 845b4c5), PR #19 CI 4/4 green (go, go-integration, web, security). Diff vs develop is only T-009 content, no .team files.
  Mechanical (in worktree, local MySQL+MinIO): make lint build test = green (gofmt, vet, golangci-lint, web lint/typecheck/i18n/check:api, go test -race, vitest 25/25); make test-integration = all packages ok (auth, media, storage, mailer, yearbook, ...).
  Migrations: ls migrations = 0001,0002,0004,0005_email_tokens,0006_media. DB A: develop binary applied up to 0005, then branch 'migrate up' applied 0006, down (0006 removed), up again, status shows 0001..0006. DB B fresh: up applies 0001..0006. No version clash.
  Postman against the merged API binary (SMEM_ENV=dev, fresh DB B), each run twice back to back: auth 45 req/110 assertions 0 failures x2; yearbooks 85/172 0 failures x2; media 44/77 0 failures x2. Note: the register/forgot-password limits (5/h per IP, documented in the collections) make a 3rd auth run within the hour 429; I restarted the API and set SMEM_RATE_REGISTER_PER_HOUR=1000 for the runs, as the collection descriptions say.
  Earlier QA findings re-checked on the merged build (manual script, real MinIO): EXIF/comment JPEG (orientation 6) -> 201, display 300x400 upright with no EXIF and no SECRET bytes; Content-Type image/jpeg, nosniff, private max-age=3600; thumb 200; other user 404, anon 401; HTML renamed .jpg 415; foreign Origin 403; 11 MiB body 413; cover set own 200 / other user's yearbook 404; other user DELETE 204 but media untouched, owner DELETE 204 then 404; yearbook delete 204 purges. 502-on-storage-failure paths are covered by internal/media integration tests (green); I did not stop the shared MinIO again. Registration, login and email routes still work with media wiring (auth collection).
  Non-blocking: unchanged from the first QA note (memory ceiling with 4 parallel 49 MP PNGs -> T-036). No test files added.
- 2026-10-08 01:59Z · leader · merged d46aec5 (squash), owner-approved 2026-10-08, QA re-verified the merge of develop (head 7d8cfe8); I checked that T-007's mailer, cleanup, Wait and email routes survived the conflict resolution. For owner to check: copy the SMEM_S3_* and SMEM_MEDIA_* variables from .env.example into .env, run make up migrate, upload a photo with curl or the Postman media collection and open it back. Follow-ups: T-036 (memory bound), notes in T-016, T-022, T-025.
- 2026-10-08 02:46Z · leader · accepted by owner (chat, 2026-10-08: 'team accept T-009')

### T-010 — Template spec and PDF page renderer
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-005
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-010-template-spec-and-pdf-page-renderer
- **PR:** https://github.com/danyaa666/smemories/pull/16
- **Updated:** 2026-10-08 01:30Z by leader
- **Comments-seen:** 9

**Spec — read this first, it is the source of truth:** `.team/epics/E05-templates-export/02-template-spec-and-pdf-page-renderer.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E05-templates-export · **PRD:** `.team/epics/E05-templates-export/PRD.md`

The template system and the PDF page renderer, built on the library chosen in T-005.

#### Comments
- 2026-10-07 06:09Z · leader · Leader: ADR 0002 is accepted (board D-12) and its section 'What T-010 must do' (9 items) is binding for this task: import codeberg.org/go-pdf/fpdf (go 1.26.0); NFC-normalise all text; split runs with the splitRuns logic and never pass a rune above U+FFFF to fpdf (emoji via toPUA aliases); ship the prepared internal/pdf/fonts/NotoEmoji-Regular.ttf (do not swap in an unmodified one); keep spaces in the primary font; cover-cropped images always use ImageOptions.AllowNegativePosition=true, pinned by a rendered-page test like TestQA_CoverIsFullBleed; keep a test pinning the non-BMP panic (TestFpdf_NonBMPRunePanics); delete internal/pdf/spike and its dev-only deps (go-pdfium, wazero, gopdf) with go mod tidy, keeping x/image and x/text. Where the ADR differs from AC5 the ADR wins: a rune found in no font is drawn as '?' and reported in Report.Warnings (code missing_glyph). Emoji are monochrome outlines by design; colour emoji is out of scope.
- 2026-10-07 06:25Z · dev · Dev: I claimed this by mistake just before the second usage gate, which answered SLEEP (cache stale), so I did no work and stopped. Status is IN_PROGRESS but empty (a worktree exists, no commits). Leader: please reset to TODO (or reclaim). Nothing to review.
- 2026-10-07 06:27Z · leader · Reclaimed by leader: the dev claimed it, then its second usage-gate check answered SLEEP because the usage cache had gone stale (the 5-minute freshness guard), so no work was done. Worktree is empty and will be reused. Nothing lost.
- 2026-10-07 11:24Z · dev · All 10 ACs + ADR 0002 items 1-9. Test: go test -race ./..., make lint, samples docs/templates/*.pdf (pdfcpu strict ok). Known: (1) rendered-page cover test reads drawing commands since go-pdfium is gone, fails if AllowNegativePosition dropped; (2) fpdf orders equal-width images randomly, so byte-identical output needs distinct photo widths (docs/templates.md, ADR addendum), leader may want a follow-up; (3) extra warning code extra_photos. CI go, go-integration, web, security green.
- 2026-10-07 11:37Z · qa · QA_PASS on PR #16 head cdced0e (fresh clone). QA added one test-only commit 5d542cd (internal/pdf/hostile_test.go, passes -race, lint 0 issues); CI on cdced0e: go, go-integration, security, web all pass.
  EVIDENCE (AC -> how -> result)
  Mechanical: make lint build test (golangci-lint 2.8.0 incl gosec) green; go test -race -count=3 ./internal/pdf ./internal/templates green; go mod tidy leaves no diff; go.mod/go.sum have 0 hits for pdfium|wazero|gopdf|signintech, x/image v0.46.0 + x/text v0.42.0 + fpdf v0.12.0 present; internal/pdf/spike and Makefile spike target gone, git grep finds no dependants; THIRD_PARTY_NOTICES.md + fonts/README.md cover BeVietnamPro and the modified NotoEmoji, OFL texts present (no Reserved Font Name).
  AC1: scratch test with 19 bad variants of classic.json (unknown slot, slot on wrong page, image slot on text, x<0, w too wide, flow element outside item, bad unit, missing unit, unknown font, bad colour, unknown colour ref, min_size>size, negative size, item_h 0, A3, fit contain, unknown JSON field, huge h): all rejected; element-level errors read e.g. template "classic", page 1 (cover), element 3 (text "titel"): unknown slot ... ; templates.List() = classic{en Classic, vi Cổ điển}, modern{Hiện đại}. PASS.
  AC2: Render signature as specified; PDFs valid: pdfcpu v0.16.1 validate --mode strict ok on 16 generated PDFs + both committed docs/templates PDFs; pdfinfo A5 419.53x595.28, A4 595.28x841.89. PASS.
  AC3: both templates x A5/A4 rendered with sample and own book, cover/profile/notes/back inspected in PNGs (pdftoppm -r 100/300). Pagination 0/1/7/60 notes = 4/4/6/23 pages, no overflow, notes in order. PASS.
  AC4: pdftotext of own book (classic+modern, A5+A4): 'Nguyễn Quỳnh Phương' x4, 'Đặng Thị Hồng' x3, 'Trần Văn Ưu' x3, all NFC; PNG zoom at 300 dpi: stacked diacritics (ễ ỳ ặ ị ồ Ư) complete, nothing clipped; dev test checks exact 'Chúc mừng tốt nghiệp! Đặng Thị Hồng' for NFC and NFD input. PASS.
  AC5: emoji 🎓🎉❤️, emoji-only note, ZWJ family, flag: no panic, no warnings, drawn as outlines (PNG checked). CJK 你好, U+10FFFF, ᚠ, invalid UTF-8 bytes, lone-surrogate bytes -> '?' + missing_glyph with rune and page/slot/note. PASS.
  AC6: 6000-char note shrinks to min_size then ends with '…' + text_truncated, inside its box in the PNG (A5 and A4, both templates); 500-char single word wrapped by character; pdftotext -bbox over 8 PDFs: 0 words outside the page. PASS.
  AC7: missing id, 'not an image', truncated JPEG -> grey placeholder + missing_image (3 of 3); small photos -> low_resolution with media id and DPI; 4 photos on 3 slots -> extra_photos (4th dropped); cover is full-bleed and centred in the PNG. Mutation proof: in a scratch copy removing AllowNegativePosition makes TestCoverImageIsPlacedAtNegativeOffset FAIL ('x = 0.00: negative offset expected = true'); PR branch untouched. PASS.
  AC8: 40 renders each: distinct widths -> 1 distinct hash (also the 5-photo sample book on modern/A4); equal widths -> 3 distinct hashes. So the limitation in docs/templates.md and ADR addendum is accurate. Accepted limitation, non-blocking. PASS.
  AC9: TestBenchmarkBook: 24 pages, 30 photos, wall 0.02 s, peak heap 37 MB, PDF 15.4 MB; whole test binary 0.47 s real, 139 MB max RSS (/usr/bin/time -l, includes generating the photos). 30 distinct 12-MP PNGs: 5.8 s, 302 MB RSS incl. generation. PASS.
  AC10: followed docs/templates.md literally in a scratch copy: copied classic.json to tiny.json (new id/name, A5 only, other colours, moved title); it validates, renders (6 pages, pdfcpu strict ok), A4 correctly refused for an A5-only template. No renderer change needed. See non-blocking 1. PASS.
  SAFETY: 10,000 short notes -> 3337 pages in 0.2 s, 50 MB RSS; 50-megapixel JPEG -> embedded in 6 ms, 72 MB; 50-MP PNG -> placeholder + warning (cap 25 MP); 500 notes x 3 refs to a PNG header -> fine; 400 randomised renders (invalid UTF-8, lone surrogates, NUL, ESC, RTL override, BOM, all-plane runes, PUA, ids like '../../etc/passwd', 3000-char names, all 4 template/size combos): no panic, no error, no path access (photos only through ImageSource). Worst case 10,000 notes x three 100 KB fields: 87 s CPU, memory stays bounded (65 MB at 1k notes, 150 MB at 3k, input excluded); context deadline stops it (2 s deadline returned context deadline exceeded).
  NON-BLOCKING OBSERVATIONS
  1. Adding a template as docs/templates.md describes turns the suite red: TestListHasBuiltIns asserts exactly {classic, modern}, and step 3 of 'Adding a template' says to edit the loop in TestSamples (Go). Suggest follow-up: assert classic and modern are present and have en+vi names, and let TestSamples loop over templates.List().
  2. An empty book (0 notes) still gets one decoration-only notes page (4 pages); documented and pinned by TestNotesPagination, so I read it as intended.
  3. No cap on notes/pages/text inside Render (cost about 9 ms per 100 KB field, memory bounded). T-014 must pass a context deadline and the notes API must cap message length and note count.
  4. AC8: T-009-normalised photos will often share a width, so T-014 cannot assume byte-identical re-renders unless it caches the first output.
  5. Flag emoji (regional indicators) draw as two letter-like glyphs and ZWJ family splits into 4 emoji (documented); emoji do not appear in pdftotext output (outlines, expected).
  6. Missing-font and invalid-unit errors name only the template, which is correct because both are theme-level (no page or element exists).
- 2026-10-07 11:38Z · leader · Leader review OK at 5d542cd (QA_PASS; the only commit since the dev's cdced0e is QA's own test-only hostile_test.go). I read text.go, image.go and the render entry point in full plus the validator and ADR addendum summaries: wrapping always makes progress (no loop on a box narrower than a glyph), photo dimensions are checked before any allocation with no integer overflow, a bad photo costs only a placeholder, fpdf's sticky error is cleared, cover images use AllowNegativePosition, fpdf panics are recovered into an error. Awaiting owner approval (Risk: high) and green CI on this head. Accepted limits: byte-identical output only for distinct photo widths (documented); one empty notes page for a book with 0 notes (pinned by a test). Follow-ups: T-035 (template tests iterate List()); the export job T-014 MUST set a context deadline and use only approved notes (Render itself has no caps; T-034 caps notes at 300 and messages at 2000).
- 2026-10-07 14:03Z · human · owner approved merge
- 2026-10-07 14:04Z · leader · merged 357e78a (squash), owner-approved in chat 2026-10-07. Review: wrapping always makes progress, photo dimensions checked before allocation, a bad photo costs only a placeholder, fpdf panics recovered into errors, cover photos use AllowNegativePosition, spike code and dev-only dependencies removed. For owner to check: open docs/templates/classic.pdf and modern.pdf (synthetic photos). Accepted limits: emoji print as monochrome outlines; byte-identical output only for distinct photo widths; one empty notes page for a book with 0 notes. The export job (T-014) must set a context deadline.
- 2026-10-08 01:30Z · leader · accepted by owner (chat, 2026-10-08: 'team accept T-010')

### T-011 — Google sign-in (OIDC + PKCE, account linking)
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-006, T-007
- **Risk:** high
- **Rework:** 1
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-011-google-sign-in-oidc-pkce-account-linking
- **PR:** https://github.com/danyaa666/smemories/pull/20
- **Updated:** 2026-10-08 02:46Z by leader
- **Comments-seen:** 11

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/01-google-sign-in.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

Google sign-in with OIDC and PKCE and safe account linking (including the pre-hijacking defence).

#### Comments
- 2026-10-07 11:28Z · leader · Promoted: full spec written in .team/epics/E02-auth/01-google-sign-in.md (first task in the new epic-file layout).
- 2026-10-08 01:48Z · dev · PR #20, head 7ffdb68. All 10 AC done. Test: make up; make lint build test test-integration (green); Go tests use an in-process fake OIDC provider (internal/auth/oidctest), no Google credentials needed. Newman: start API with SMEM_GOOGLE_CLIENT_ID/SECRET, SMEM_PUBLIC_BASE_URL, SMEM_OIDC_COOKIE_KEY (README 'Google sign-in setup'), run postman/auth.postman_collection.json (folder 'Google sign-in' needs network to accounts.google.com for the start redirect; forgot-password limit still means restart after 2 runs). Migration is 0007_user_identities.sql because T-009 takes 0006: merge T-009 first; go.mod/go.sum/README/.env.example/openapi/schema.d.ts/config_test will conflict textually with T-009 (keep both). New deps (approved in spec): go-oidc/v3, oauth2 (+ go-jose/v4 indirect). Known gaps: no unlinking, no web button (T-015), discovery lock held during the 5 s fetch (ponytail comment). Found and fixed while testing: base64 decoding ignores spare bits, so cookie signature is compared in encoded form. Risk high: owner approval needed.
- 2026-10-08 01:58Z · qa · QA_PASS T-011 (PR #20, head 7ffdb68, CI 4/4 green: go, go-integration, web, security; PR mergeable into develop).
  Mechanical: make lint build test green; go test -race -tags integration ./... all ok; internal/auth + config integration 3x back to back, no flake. Runtime: real API binary + real MySQL, fresh DB (0001..0005,0007; migrate down/up of 0007 OK) against a standalone copy of the repo's fake OIDC provider (temporary harness, removed, not committed) driven by my own python client.
  AC1 PASS: start -> 302 to provider with response_type=code, scope 'openid email profile', state, nonce, code_challenge S256, redirect_uri=<base>/api/v1/auth/google/callback; cookie smem_oidc Max-Age=600, HttpOnly, SameSite=Lax, Path=/, not Secure in dev (Secure in prod covered by Go test with secure true/false); cookie content matches URL (Go test). Against the real default issuer (network up) Location is https://accounts.google.com/o/oauth2/v2/auth.
  AC2 PASS: all redirect to /login?error=<code>, no user/identity/session created: no cookie, payload flipped, signature flipped/truncated/empty/no dot/spare-bit last char -> oidc_state; state differs/missing/another flow's state -> oidc_state; error=access_denied (+ description not echoed) -> oidc_denied (without valid cookie -> oidc_state); bad/missing code, no id_token, empty email -> oidc_failed; bad signature, wrong iss, wrong aud, expired, wrong nonce, empty nonce -> oidc_failed; email_verified false, string 'true', number 1 -> email_unverified. Replay of a used code+state (cookie replayed by hand) -> oidc_failed; replay without cookie -> oidc_state; cookie cleared after callback. Expired-cookie case only via Go test (cannot wait 10 min). A failed callback leaves an existing session alive. Logs: reason + error text only, no code, tokens, state, cookie, client secret or key (grepped).
  AC3 PASS: second sign-in with known sub -> 302 return_to, same user, old session cookie 401 afterwards, new one 200; same sub with a changed email at Google -> still the same user.
  AC4 PASS: verified local account (email given in different case by Google) -> linked, user count unchanged, password login still works.
  AC5 PASS: victim registered with password + live session, then Google sign-in same email: user is password_hash NULL and verified, old password login 401, old session 401, one fresh session.
  AC6 PASS: unknown email -> user with NULL hash, verified, name from claim (Vietnamese diacritics kept), locale vi for 'vi-VN', en for 'fr'/'en-US'; empty or control-char name -> local part of the email; social-only password login gives the same status/body as an unknown account; Forgot password -> reset (log mailer link) -> password login works -> Google sign-in still works.
  AC7 PASS: 15 return_to cases: //evil, https://evil, /\evil, javascript:, relative 'books', empty, CRLF, tab, DEL, U+202E, 200+ chars -> '/'; '/books', '/ok?next=https://evil.example', 199-char path kept; start without return_to OK.
  AC8 PASS: start and callback each 30x302 then 429 rate_limited with Retry-After 900; token/code never logged; an existing session is ignored until success. Concurrency: 6 parallel callbacks for one new email -> 6x302, 1 user, 1 identity; same for an unverified local account; no 500.
  AC9 PASS: no SMEM_GOOGLE_CLIENT_ID -> start and callback 404 not_found, /healthz 200. With it: missing secret, missing/empty PUBLIC_BASE_URL, missing key, 5-byte key, ftp:// issuer, http issuer in prod -> startup 'config error' naming the variable.
  AC10 PASS: openapi documents both routes (302s, codes, 404/429), README 'Google sign-in setup' present with redirect URI and variables, .env.example updated; newman auth collection (52 req/134 assertions, incl. 7-request Google folder) run twice against the API with Google enabled: 0 failures both runs.
  NON-BLOCKING: (1) Migration order: a dev DB that already applied 0007 (this branch) cannot take T-009's 0006 afterwards: 'migrate up failed: detected 1 missing (out-of-order) migration lower than database version (7): version 6'. Fresh DBs and CI are fine; merge T-009 first (as dev noted) and anyone who ran this branch must recreate or manually apply 0006. (2) A second Google sub with the same verified email links to the same account (2 identities): consistent with the spec's table. (3) Postman Google folder needs outbound access to accounts.google.com (documented). No test files added; existing Go coverage is thorough. Risk high: owner approval needed before merge.
- 2026-10-08 02:00Z · leader · Leader review OK on head 7ffdb68 (the commit QA tested, CI 4/4 green): flow matches the spec: state, nonce and S256 PKCE, signed 10-minute HttpOnly SameSite=Lax cookie compared in constant time and in encoded form, ID token verified by go-oidc (signature, iss, aud, exp) plus nonce, email_verified must be the boolean true, return_to allow-list, no tokens or codes in logs, identity and user creation in one transaction with FOR UPDATE and a retry on duplicate key or deadlock, pre-hijacking defence clears password and sessions before linking. Not blocking, recorded: campus NAT vs per-IP 30/15 min and a discovery lock held during the fetch (T-031 spec), web button and error codes (T-015 spec). One thing left: PR #20 conflicts with develop now that T-009 merged (d46aec5). Sending back only for the merge, no code defect.
- 2026-10-08 02:00Z · leader · Leader: merge develop (T-009 is in, d46aec5) into the task branch; no rebase of a pushed branch. Expected conflicts: go.mod, go.sum, README.md, .env.example, api/openapi.yaml, web/src/api/schema.d.ts, internal/config/config_test.go (keep both sides), possibly cmd/smemories-api/main.go. Then: (1) cd web && npm run gen:api, commit schema.d.ts, npm run check:api passes; (2) ls migrations/ shows 0005, 0006 and 0007_user_identities.sql in that order; test migrate on a fresh DB and on one that already applied 0006; (3) make lint build test test-integration; auth, yearbooks, media Postman collections twice (restart the API between runs: forgot-password is limited to 5 per hour per IP); (4) push, mark READY_FOR_QA with the merge-commit SHA. Nothing else in scope changes.
- 2026-10-08 02:04Z · dev · Merge commit 4109b7b (origin/develop incl. T-009 merged into the branch, pushed to PR #20). Conflicts kept both sides in .env.example, go.mod, go.sum (go mod tidy), internal/config/config.go; other files auto-merged. (1) npm run gen:api produced no diff to schema.d.ts, check:api ok. (2) migrations/ 0005, 0006_media, 0007_user_identities in order; fresh DB up OK; DB built by develop binary (0001-0006) then branch up applies only 0007; 0007 down/up OK. (3) make lint build test test-integration green; newman auth (52/134), yearbooks (85/172), media (44/77) twice each, API restarted before every collection (register limit): 0 failures. No other code change.
- 2026-10-08 02:15Z · human · owner approved merge
- 2026-10-08 02:15Z · leader · Owner approved the merge in chat (2026-10-08: 'team approve T-011'). Merge as soon as QA passes the merge result and CI is green on the unchanged head.
- 2026-10-08 02:22Z · qa · QA_PASS T-011 re-verification of merge 4109b7b (PR #20, CI 4/4 green on that head).
  Migrations: fresh scratch DB -> 0001,0002,0004,0005,0006,0007 applied; DB built by develop (0001..0006) then branch up applies only 0007; 0007 down/up OK; user_identities schema as spec (unique provider+subject, FK cascade).
  Mechanical (T-011 worktree): make lint build test green; make test-integration green (auth, config, db, httpx, media, yearbook ...); go test -race -tags integration ./internal/auth -run Google 3x back to back, all ok; npm run gen:api leaves no diff.
  Newman, API restarted before every run (Google enabled, real issuer): auth 52 req/134 assertions, yearbooks 85/172, media 44/77, each run twice: 0 failures.
  Google findings on the merged build: runtime check with real binary: start -> 302 to accounts.google.com with S256 challenge/state/nonce, redirect_uri=<base>/api/v1/auth/google/callback, smem_oidc Max-Age=600 HttpOnly SameSite=Lax; callback without cookie -> /login?error=oidc_state and cookie cleared; callback rate limit 30x302 then 429 Retry-After 900; no SMEM_GOOGLE_CLIENT_ID -> start 404, /healthz ok. Provider-dependent cases (pre-hijacking, linking, replay, ID-token claim rejections, return_to, concurrency, log secrecy) are the 20 Go tests TestGoogle* in internal/auth, which pass; my earlier runtime proof of them is unaffected because the merge diff vs 7ffdb68 touches only csrf.go (T-009 media), config, main.go wiring and go.mod, not google.go.
  NON-BLOCKING: out-of-order migration risk from earlier note still stands for anyone who applied 0007 before 0006; fresh/CI fine. Risk high: owner approval needed before merge.
- 2026-10-08 02:23Z · leader · merged d209266 (squash), owner-approved in chat 2026-10-08, QA re-verified the merge result (head 4109b7b). Review: ID token verified by go-oidc plus nonce, state and S256 PKCE, signed 10-minute cookie compared in encoded form, email_verified must be boolean true, return_to allow-list, pre-hijacking defence in one transaction, no tokens in logs. For owner to check: Google is OFF until SMEM_GOOGLE_CLIENT_ID is set; README section 'Google sign-in setup' lists the Google Cloud steps (real credentials are a later owner step). Follow-ups in T-015 and T-031 specs.
- 2026-10-08 02:46Z · leader · accepted by owner (chat, 2026-10-08: 'team accept T-011, T-012')

### T-012 — Collection links (owner API and public lookup)
- **Status:** DONE
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-007, T-008
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-012-collection-links-owner-api-and-public-lo
- **PR:** https://github.com/danyaa666/smemories/pull/21
- **Updated:** 2026-10-08 02:46Z by leader
- **Comments-seen:** 8

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/01-collection-links.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Owner-created private collection links (create, list, revoke) and the public lookup the contributor form needs.

#### Comments
- 2026-10-07 11:28Z · leader · Promoted: full spec written in .team/epics/E04-friends-notes/01-collection-links.md. Needs T-007 (verified-email check) and T-008 (yearbooks); public submission is the new T-034.
- 2026-10-08 02:14Z · dev · PR #21, head 231955e. All 8 AC. Test: make up; make lint build test test-integration (green). Newman notes collection needs verified accounts (dev mail only in API stdout): run --folder 'Setup (once per database)', then 'make verify-newman-users' (or the UPDATE in the collection description for a scratch DB), then the whole collection (I ran it 3x back to back, 0 failures). Migration 0008_note_collections.sql (0007 is T-011): a DB that applied 0008 first cannot take 0007 later (out-of-order), merge T-011 first or recreate. Decisions: email_not_verified on create only; note_count is 0 until T-034; auth.Handler.ClientIP exported for the public limiter; Makefile target verify-newman-users added. Risk high: owner approval needed.
- 2026-10-08 02:14Z · leader · Leader pre-review of head 231955e (QA still running, CI pending): code is sound. 192-bit token hashed with SHA-256 and shown once, tokenHash rejects wrong length/alphabet before the database, lookup is one indexed query, every owner query is scoped through yearbooks.owner_id, create takes FOR UPDATE on the book row so the 5-link limit holds under concurrency, revoke is idempotent, public route reads no cookie and the global headers (no-store, no-referrer) apply. Not blocking: the public lookup limiter counts every request per IP (60/15 min), which a class on one campus network would hit; fix recorded in the T-034 and T-031 specs (count misses only). Merge order: T-011 first (migration 0007), then this one (0008); it will need a develop merge after T-011 lands.
- 2026-10-08 02:22Z · qa · QA_PASS T-012 (PR #21, head 231955e, CI 4/4 green: go, go-integration, web, security).
  Mechanical (T-012 worktree): make lint build test green; make test-integration green (notes 6.5s, all packages ok). Migration 0008 tested on a fresh DB without 0007 (0001..0006,0008; down/up OK) and on a DB built by the T-011 branch (0001..0007) then 0008 applied. The number gap is not a defect. Note: on the DB that has both 7 and 8, `migrate down` from the T-012 binary says 'version not found' because that binary does not contain 0007; disappears once T-011 is merged.
  Postman notes collection (Setup folder, SQL verify of users A/B, then whole collection) run 3x back to back: 49 req/89 assertions, 0 failures each time; covers create, list, revoke, lookup active/revoked/expired/garbage, cross-user 404.
  Evidence table (own python client + curl against the real binary and real MySQL):
  AC1 PASS: 201 with exactly {id,label,deadline_at,created_at,token}; token 32 chars base64url (24 bytes); DB token_hash = sha256(token) (compared hex), token not stored. Label: 60 ok, 61 -> invalid_label, 60 emoji ok, 61 emoji bad, zero-width, RLO, newline -> invalid_label, ' Cafe+U+0301 class ' -> NFC 'Café class' trimmed, whitespace-only/empty/null -> '', number/array -> invalid_body. Deadline: now, now-1s, +1y+1min, +1y+2d, 2100, date only, no tz, empty, 0000 -> 400 invalid_deadline; +1s, +364d, +1y-1min, +07:00 offset (stored/returned UTC) -> 201. Wrong Origin or none -> 403 csrf_origin_mismatch; no session -> 401.
  AC2 PASS: B/non-owner and nonexistent book -> 404 for create, list and revoke (identical); unverified owner create -> 403 email_not_verified; 5 active then 6th -> 409 limit_reached; 12 concurrent creates at an empty book -> exactly 5x201 + 7x409, 5 active in DB; revoke then create frees a slot and the new token differs.
  AC3 PASS: list newest first with id,label,deadline_at,created_at,revoked_at,note_count (0); token and hash absent (searched whole body).
  AC4 PASS: DELETE 204, repeat 204 (revoked_at unchanged), B -> 404, garbage id / yearbook id as collection id -> 404; revoked token stays 404.
  AC5 PASS: 200 {"yearbook":{"title"},"owner":{"display_name"},"deadline_at","open":true} only; revoked and never-valid token give byte-identical 404 (apart from request_id) and the same headers; deadline passed -> 410 collection_closed (2s deadline: 200 before, 410 after); expired-then-revoked -> 404. Odd tokens (10 KB, 64 KB, unicode, %00, %2F, space, SQL quote, wrong length, wrong case, '=' padding) -> 404; '..' -> 307 from the Go mux, harmless.
  AC6 PASS: no Set-Cookie, no Access-Control-* (also with an allowed Origin; OPTIONS 405); a valid owner session cookie sent along is ignored; 60/15min per IP: 40 answered after the earlier ~20 then 429 with Retry-After 880; garbage tokens also 429, owner endpoints unaffected; X-Forwarded-For ignored with TRUST_PROXY=false. Access log lines carry route 'GET /v1/public/collect/{token}', also for 404/405 unmatched variants with a marker token; grep of the whole API log for every token and the session cookie: 0 hits. The link token as session cookie / Bearer -> 401.
  AC7 PASS: deleted a yearbook with 5 collections -> 204, 0 orphan rows, list afterwards 404.
  AC8 PASS: openapi.yaml documents the four endpoints with the error codes; `make lint` includes check:api; Postman as above.
  NON-BLOCKING: (1) POST create with no body at all -> 400 invalid_body; spec shows both fields optional, `{}` works (consistent with other create endpoints, documented behaviour in the collection). (2) An unverified user who is not the owner gets 403 email_not_verified instead of 404 on someone else's yearbook (no information leak; the dev noted the check is create-only; unverified users can still list/revoke their own, which are empty). (3) Postman does not cover the public 429 (60 per 15 min impractical); covered by Go tests and my manual run. (4) note_count is always 0 until T-034, as documented. Risk high: owner approval needed before merge.
- 2026-10-08 02:24Z · leader · Leader review OK on head 231955e (QA_PASS on the same head, CI 4/4 green). After T-011 merged (d209266) I tested the real merge result in a scratch worktree: merges without conflicts, go build, go vet (also with the integration tag), go test ./... and web check:api all pass, migrations 0005..0008 in order. So no dev round is needed. Non-blocking, recorded: public lookup limiter should count only misses (T-034 and T-031 specs). Awaiting owner approval; merge right after.
- 2026-10-08 02:34Z · human · owner approved merge
- 2026-10-08 02:34Z · leader · merged 7ca8137 (squash), owner-approved in chat 2026-10-08, QA_PASS on the same head, and I verified the merge result with T-011 in a scratch worktree (build, vet, unit tests, schema check). Review: 192-bit token hashed at rest and shown once, one indexed lookup, owner queries scoped through the yearbook, row lock keeps the 5-link limit under concurrency, revoke idempotent. For owner to check: after make up migrate, create a link with the Postman notes collection (README in postman/): the token appears only in the create response. Follow-up: public lookup limiter counts only misses (T-034 and T-031 specs).
- 2026-10-08 02:46Z · leader · accepted by owner (chat, 2026-10-08: 'team accept T-011, T-012')

### T-013 — Notes moderation API (approve, hide, reorder, delete)
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-034
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/03-notes-moderation-api-approve-hide-reorder.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Owner lists pending/approved/hidden notes per yearbook, approves or hides them, reorders approved notes, deletes a note (and its photos).

#### Comments

### T-014 — Export job: assemble book, render PDF, store, download
- **Status:** BACKLOG
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-009, T-010, T-013
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 1

**Spec — read this first, it is the source of truth:** `.team/epics/E05-templates-export/03-export-job-assemble-book-render-pdf-store.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E05-templates-export · **PRD:** `.team/epics/E05-templates-export/PRD.md`

Asynchronous export: POST creates a job (one active export per book), a bounded worker maps DB rows to the pdf.Book value, renders with T-010, writes the PDF to storage, and exposes status (queued, running, done, failed with a safe reason)

#### Comments
- 2026-10-07 11:38Z · leader · Leader note from the T-010 review: pdf.Render has no caps on notes or text length (10,000 notes with three 100 KB fields took 87 s of CPU in QA's adversarial test, memory bounded). The export job must (1) run Render under a context deadline (60 s, matching the quality bar), (2) pass only approved notes (T-034 already caps a collection at 300 notes and messages at 2000 characters), (3) report warnings (low_resolution, missing_glyph, missing_image, extra_photos, text_truncated) to the owner in plain language (T-019).

### T-015 — Web: auth pages and session handling
- **Status:** MERGED
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-003, T-007
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-015-web-auth-pages-and-session-handling
- **PR:** https://github.com/danyaa666/smemories/pull/25
- **Updated:** 2026-10-08 03:54Z by leader
- **Comments-seen:** 8

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/04-web-auth-pages-and-session-handling.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

Register, login, logout, verify-email, forgot/reset password pages; session bootstrap via GET /v1/me; protected-route wrapper; all strings in EN and VI; accessible forms with error messages mapped from API error codes.

#### Comments
- 2026-10-07 06:10Z · leader · Leader note from the T-003 review (non-blocking, do with this task): the language buttons show only EN/VI. Give each an accessible name from the existing language.en / language.vi locale keys (aria-label), which are unused today; update the LanguageSwitcher test accordingly. Also pass Headers instances safely in api/client.ts (spreading a Headers object drops its entries).
- 2026-10-08 02:24Z · leader · T-007 is merged: unblocked. Spec updated with leader notes from the T-007 and T-011 reviews (token handling on verify/reset pages, Google button and error codes).
- 2026-10-08 03:30Z · dev · Auth pages + session bootstrap + RequireAuth + Google link (VITE_GOOGLE_SIGNIN) + leader notes from T-003/T-007/T-011 done. make lint build test green, 56 vitest tests. QA: run web against the API (make up, make run, npm run dev) incl. real email links, Vietnamese strings, keyboard/screen-reader pass. Spec had no numbered AC; checklist in PR. Known: not run in a real browser by me.
- 2026-10-08 03:42Z · qa · Tested against a real API (make up, migrate, API built from the task worktree, vite dev, headless Chrome via playwright-core; mail from the log mailer) in EN and VI and at 375 px. Passing: register (field error aria-invalid/aria-describedby, email_taken, weak_password, empty submit), login (wrong pw, 429 lockout 'try again in 15 min' from Retry-After), logout (nav, redirect), session bootstrap (reload keeps session, 401 -> signed out, API unreachable -> RequireAuth shows retry, retry works), RequireAuth round trip (/account?tab=1 -> /login -> back to /account?tab=1), signed-in /login and /register redirect, verify-email from the logged mail link (exactly one POST under StrictMode, account flips to Email verified without reload, reuse/garbage/no token -> expired message), forgot password (identical text for unknown and known address, bad email field error), reset from the logged link (weak password field error, success, link reuse -> expired + ask for new, old password rejected, new works), resend limit (3 ok, 4th 429 'try again in 60 min'), ?error= oidc_state/oidc_denied/oidc_failed/email_unverified/unknown/HTML payload (translated, escaped), Google button hidden by default and shown with VITE_GOOGLE_SIGNIN=true with href /api/v1/auth/google/start?return_to=<encoded path>, EN/VI switch (html lang, persisted), keyboard tab order and Enter submit, :focus-visible ring, no horizontal overflow at 375 px for normal names. web: lint, typecheck, i18n, check:api, 56 vitest x3 and build all green; PR CI green on head 31c2546. ISSUES: 1 (major, violates leader note 'removes the token before any other work or network call') the first GET /api/v1/me (session bootstrap from AuthNav) is sent while the address bar still holds the token and its Referer is the full /verify-email?token=... (same for /reset-password). Repro: playwright request log shows 'GET /api/v1/me | page url at request: /reset-password?token=MktbKs4V...| referer: http://localhost:5173/reset-password?token=...'; only the later POST sees the clean URL. Cause: AuthNav (earlier in the tree) fires useMe in its effect before useUrlToken's effect in the page runs. Fix options: strip ?token synchronously before React mounts (e.g. in main.tsx/index.html inline script, keeping it in a module variable for the page), and/or add <meta name="referrer" content="no-referrer"> to web/index.html; add a test that no request is issued while location.search holds the token. 2 (minor) offline: with the browser offline, submit on login stays on 'Please wait…' with a disabled button and no message (TanStack default networkMode 'online' pauses the mutation, then fires it automatically when the network returns). Repro: open /login, context offline, submit. Expected: the network_error text. Fix: new QueryClient({defaultOptions:{queries:{networkMode:'always'},mutations:{networkMode:'always'}}}) in web/src/main.tsx; same pause applies to the useMe bootstrap (stuck 'Loading' instead of the retry panel). 3 (minor) at 375 px a 100-character display name without spaces (allowed by the API) makes the page 1391 px wide (header link does not wrap/shrink). Repro: register with 'W' x 100, open /account at 375 px. Expected no horizontal scroll (min-width:0 / overflow-wrap:anywhere on .auth-nav a). 4 (minor, cosmetic) at 375 px the brand touches the nav links, nav links wrap to two lines ('Đăng nhập', 'Create account') and the Google button sits flush under the submit button; Sign out and 'Send the link again' use unstyled default buttons. Observation for the leader, not T-015: a Vietnamese-UI user receives the English verification email because POST /v1/auth/register has no locale input (users.locale defaults to en); consider a locale field or Accept-Language in T-006/T-007. Not checked: real Google round trip (API had no Google config, /google/start answers 404), screen reader (VoiceOver/NVDA) pass, Safari/Firefox/real phone, vite preview production build, CSRF behaviour on a non-allowed origin (403 csrf_origin_mismatch falls back to the generic message).
- 2026-10-08 03:43Z · leader · Leader: QA_FAIL accepted; fix in the same PR (branch and worktree are reused), then READY_FOR_QA. (1) MAJOR, spec requirement: on /verify-email and /reset-password the token must leave the address bar before ANY network call; today AuthNav's effect fires GET /api/v1/me while the URL still holds the token and the Referer carries it. Fix: strip the token synchronously before anything that can fetch mounts (read it into memory, history.replaceState before AuthNav mounts, or hold the /me query until the token is stripped); add a test that fails if any request is made while location.search still contains a token. Also add <meta name=referrer content=no-referrer> to index.html as defence in depth. (2) networkMode 'always' for the mutations (and queries where it matters) so an offline submit shows the network-error message instead of 'Please wait'. (3) A 100-character display name with no spaces makes the page 1391 px wide at 375 px: wrap with overflow-wrap:anywhere wherever the name is shown, with a test or note. (4) Minor spacing at 375 px: fix if cheap. Not in this PR: the English verification email for Vietnamese users is the API register endpoint (new task T-045). The generic message for 403 csrf_origin_mismatch is acceptable.
- 2026-10-08 03:45Z · dev · Head f24eb25. (1) token: captureUrlToken() in main.tsx moves ?token= into memory via history.replaceState before React renders, so GET /v1/me (AuthNav) and the POST never see it; useUrlToken reads the stash (path-matched, cleared after first use); <meta name=referrer no-referrer> in index.html. Tests (auth.test.tsx 'token in the address bar', verify + reset under StrictMode, every fetch records location.search; plus reset test now checks all calls) fail when the capture is removed. (2) src/queryClient.ts networkMode 'always' for queries and mutations, used by main.tsx and test renderApp; 'offline' test fails with networkMode online. (3) overflow-wrap:anywhere on main and .auth-nav a, header/nav flex-wrap, min-width 0: CSS only, jsdom cannot measure; QA please recheck 100-char name at 375 px. (4) spacing: header gap, nav button padding, margin under the form before the Google button. make lint build test green, 60 vitest tests. Not run in a real browser by me.
- 2026-10-08 03:53Z · qa · Re-test of head f24eb25 (PR #25 CI green: go, go-integration, security, web). Real API (built from worktree, fresh DB, log mailer) + headless Chrome (playwright-core), both vite dev and the production build via vite preview. Mechanical: web lint+prettier, 60 vitest, build green. (1) TOKEN, PASS: verify and reset links opened from the logged mail: first GET /api/v1/me, the POST and the follow-up GET /me all see the clean address bar (/verify-email, /reset-password) with empty Referer; only the document GET and static asset/module fetches happen while the token is in the URL (unavoidable, Referer empty thanks to the meta tag). Verify: POST sent once, account flips to verified. Reset: weak password -> 400 field error and the token is still held in memory, strong password then succeeds, new password logs in. Hard reload after removal: verify shows 'expired or already used', reset shows the same plus 'Ask for a new link', no POST fired; back/forward does not bring the token back. Link reuse -> expired message. (2) OFFLINE, PASS: context offline, submit on login -> 'Could not reach the server...' (no stuck 'Please wait'); /me aborted on /account -> retry panel, retry works. 500 html, 503 busy and 403 csrf messages ok. (3) 100-char 'W' name at 375 px (header and account): scrollWidth-clientWidth = 0, name wraps; spaced long name and long email also 0. (4) SPACING 375 px, PASS: brand no longer touches nav, links stay on one line in VI, Google button has a gap under Sign in (built with VITE_GOOGLE_SIGNIN=true, href /api/v1/auth/google/start?return_to=%2Faccount). (5) REGRESSION, PASS: register (bad email, weak pw, duplicate, empty), login wrong pw, 429 lockout 'try again in 15 min', logout, reload keeps session, RequireAuth, verify from mail (EN+VI), reuse/garbage/no token, resend 3 ok then 429 '60 min', forgot (same text), reset (EN+VI), ?error= codes incl. HTML payload escaped, EN/VI switch persisted, keyboard Enter + focus ring. Non-blocking observations: (a) reload on /reset-password BEFORE submitting loses the still-valid token (shown as 'expired or already used'; user must request a new link); acceptable consequence of the strip-first rule, a sessionStorage stash could soften it. (b) Sign out and 'Send the link again' remain default grey browser buttons (cosmetic). (c) Vietnamese UI user still gets the English verify mail: T-045. Not covered: real Google round trip, screen reader pass, Safari/Firefox.
- 2026-10-08 03:54Z · leader · merged 66d5b80 (squash), QA_PASS on head f24eb25 after one rework, CI green. Review: token moved into memory before React renders (no request carries it, Referer empty, verified in a real browser), redirect target allow-listed, return_to URL-encoded, no raw HTML, typed API client, EN/VI parity, 60 tests. For owner to check: cd web && npm run dev with the API on :8080; register, read the verification link in the API log, open it (the token disappears from the address bar), try forgot and reset. Google button appears only with VITE_GOOGLE_SIGNIN=true. Known: reloading /reset-password before submitting loses the token (by design: ask for a new link); verification mail is English for Vietnamese users until T-045.

### T-016 — Web: yearbook list, create/edit, profile and photo upload UI
- **Status:** READY_FOR_QA
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-009, T-015
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-016-web-yearbook-list-create-edit-profile-an
- **PR:** https://github.com/danyaa666/smemories/pull/31
- **Updated:** 2026-10-08 09:13Z by dev
- **Comments-seen:** 3

**Spec — read this first, it is the source of truth:** `.team/epics/E03-yearbooks/03-web-yearbook-list-create-edit-profile-and.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E03-yearbooks · **PRD:** `.team/epics/E03-yearbooks/PRD.md`

Dashboard of the user's yearbooks; create/edit form for book information and the owner profile; photo upload with progress, thumbnails and delete; set cover and profile photo.

#### Comments
- 2026-10-08 09:04Z · leader · T-009 and T-015 are merged: unblocked. Spec carries leader notes (PUT profile clears photo_media_id unless sent; Letter page size selector; upload error codes).
- 2026-10-08 09:06Z · dev · Spec gap, not blocking: the API has no way to LIST a yearbook's photos (no GET /v1/yearbooks/{id}/media, and Yearbook only carries cover_media_id and profile.photo_media_id). So after a page reload the web UI can show only the cover and profile photo; the gallery of the other photos exists only for the current session's uploads. I am building the UI so that a list endpoint plugs in with one query (items: id,width,height,bytes). Suggest a follow-up API task: GET /v1/yearbooks/{id}/media -> {media:[{id,width,height,bytes,created_at}]} (owner only, newest first), then a tiny web follow-up. Tell me if you want me to do the API part inside T-016 instead.
- 2026-10-08 09:13Z · dev · Web UI done (list, create/edit, profile, photo upload/progress/thumbs/delete, cover and profile photo, A5/A4/Letter, en+vi). 82 vitest tests, lint/typecheck/i18n/build green. Test: npm test in web/; manual: /yearbooks. NOT verified in a real browser against a real API (private compose stack was blocked for me); QA please check 375 px, real upload through Vite proxy (CSRF Origin), thumbnails, 413/429/503 live. Known gap: API has no list-photos endpoint, so after reload only cover/profile photo show (see comment #2).

### T-017 — Web: notes link management and moderation inbox
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-013, T-016
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/04-web-notes-link-management-and-moderation.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Create/copy/revoke the collection link, set a deadline, see submissions grouped by status, approve/hide/reorder, preview the note as it will print.

#### Comments

### T-018 — Web: public anonymous notes form
- **Status:** BACKLOG
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-003, T-034
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/05-web-public-anonymous-notes-form.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Mobile-first public page opened from the shared link: name, relationship, message, photo picker; clear success and error states; EN and VI; works without an account or cookies; no personal data of the owner beyond the book title.

#### Comments

### T-019 — Web: template picker, PDF preview (pdf.js) and export/download
- **Status:** BACKLOG
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-014, T-016
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E05-templates-export/04-web-template-picker-pdf-preview-pdf-js-and.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E05-templates-export · **PRD:** `.team/epics/E05-templates-export/PRD.md`

Choose a template (thumbnails from sample renders), trigger export, show progress, preview the real PDF with pdf.js (works on phones), download.

#### Comments

### T-020 — End-to-end smoke test of the M1 journey in CI (Playwright)
- **Status:** BACKLOG
- **Priority:** P1
- **Type:** infra
- **Milestone:** M1
- **Depends-on:** T-017, T-018, T-019
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/10-end-to-end-smoke-test-of-the-m1-journey-in.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

One Playwright test drives the full M1 journey on the compose stack in CI: register, create book, upload photo, create link, submit an anonymous note, approve it, export, download, verify the PDF text.

#### Comments

### T-021 — Transactional email provider and domain setup
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** infra
- **Milestone:** M2
- **Depends-on:** T-007
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E06-go-live/01-transactional-email-provider-and-domain-setup.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E06-go-live · **PRD:** `.team/epics/E06-go-live/PRD.md`

Replace LogMailer with a real provider (SES is the natural fit on AWS) including SPF/DKIM/DMARC runbook and bounce/complaint handling.

#### Comments

### T-022 — Dockerfile, production config and migrations as a one-off task
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** infra
- **Milestone:** M2
- **Depends-on:** T-004
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 1

**Spec — read this first, it is the source of truth:** `.team/epics/E06-go-live/02-dockerfile-production-config-and-migrations.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E06-go-live · **PRD:** `.team/epics/E06-go-live/PRD.md`

Multi-stage Dockerfile (distroless or alpine, non-root), production config validation, graceful shutdown, migrate-as-task entrypoint, image scan in CI.

#### Comments
- 2026-10-07 06:42Z · leader · Leader note from the T-004 review: go.mod says go 1.26.0, and govulncheck reports 11 reachable standard-library vulnerabilities at that exact patch. CI therefore builds with the newest 1.26 patch (board L-10). The Dockerfile must do the same: base image on the newest golang:1.26 patch, never an old pinned patch, and rebuild regularly (Dependabot's docker ecosystem should watch it).

### T-023 — AWS infrastructure as code and deploy pipeline
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** infra
- **Milestone:** M2
- **Depends-on:** T-022
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E06-go-live/03-aws-infrastructure-as-code-and-deploy.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E06-go-live · **PRD:** `.team/epics/E06-go-live/PRD.md`

Fargate service, RDS MySQL (private subnets, encrypted, backups), S3 (private, encrypted, lifecycle), CloudFront with /api origin and prefix strip, ACM cert, secrets in Secrets Manager, deploy workflow with OIDC to AWS (no long-lived keys).

#### Comments

### T-024 — Observability: metrics, alarms, uptime check, log retention
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** infra
- **Milestone:** M2
- **Depends-on:** T-023
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E06-go-live/04-observability-metrics-alarms-uptime-check.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E06-go-live · **PRD:** `.team/epics/E06-go-live/PRD.md`

RED metrics and DB/pool/export-queue gauges, CloudWatch alarms (5xx rate, p95 latency, export failures, RDS CPU/storage), external uptime check on /readyz, log retention and PII-free log review.

#### Comments

### T-025 — Backups, restore drill, and user data export/deletion
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** security
- **Milestone:** M2
- **Depends-on:** T-023
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E06-go-live/05-backups-restore-drill-and-user-data-export.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E06-go-live · **PRD:** `.team/epics/E06-go-live/PRD.md`

Automated RDS and S3 backups with a documented and tested restore drill; user-initiated export and deletion of account and yearbooks including media and contributor submissions; retention policy written down (owner decision on retention periods).

#### Comments

### T-026 — Class yearbook: classes, memberships, roles and invites
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** feature
- **Milestone:** M3
- **Depends-on:** —
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E07-class-yearbook/01-class-yearbook-classes-memberships-roles-and.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E07-class-yearbook · **PRD:** `.team/epics/E07-class-yearbook/PRD.md`

Class space owned by an admin (teacher or monitor), invite links/emails for students, roles (admin, student), permissions on who edits which profile page.

#### Comments

### T-027 — Class book assembly: many profiles per book, class pages and templates
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** feature
- **Milestone:** M3
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-06 10:13Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E07-class-yearbook/02-class-book-assembly-many-profiles-per-book.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E07-class-yearbook · **PRD:** `.team/epics/E07-class-yearbook/PRD.md`

Book with many student profiles in a chosen order, class-level pages (intro, teachers, events, superlatives), group photo layouts, per-student notes, template extensions in the spec; export budget scales to 120 pages.

#### Comments

### T-028 — T-001 follow-ups: log route not path, lint scope and findings, OpenAPI 404/405
- **Status:** DONE
- **Priority:** P2
- **Type:** tech-debt
- **Milestone:** M0
- **Depends-on:** T-001
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-028-t-001-follow-ups-log-route-not-path-lint
- **PR:** https://github.com/danyaa666/smemories/pull/5
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/05-t-001-follow-ups-log-route-not-path-lint.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Small follow-ups found in the T-001 review and QA.

#### Comments
- 2026-10-07 06:01Z · dev · AC1-5 done; see PR #5. Test: make test, make lint, golangci-lint run (+ --build-tags integration), npx newman run postman/platform.postman_collection.json (skip the Manual folder). Gap: Newman collection has no new request (no HTTP change besides docs).
- 2026-10-07 06:12Z · qa · Tested PR #5 head ad6c5e0 in worktree (clean). No CI checks exist on the PR (CI is T-004); web/ absent on this branch, so no type-generation check.
  AC1 PASS: real binary run (SMEM_ENV dev, dev DSN, port 18281). Log lines: GET /healthz -> route 'GET /healthz'; GET /nope/abc123secret?token=abc123secret -> route '-' 404; POST /healthz -> route '-' 405 (Allow: GET, HEAD); /healthz/..%2Fx -> '-' 404; no 'path' key anywhere. Throwaway real-listener test (deleted, not committed) with GET /v1/secret/{token}, /boom/{token}, /files/{rest...}: token/%2F/../ variants and a ?token= query log route patterns only, 0 occurrences of abc123secret in any access line; panic -> one access line route 'GET /boom/{token}' status 500, plus the separate 'panic' line (request_id/panic/stack, no path); DELETE on registered route -> 405 route '-'. Committed test TestAccessLogRouteNeverLogsRawPath covers the same and asserts exactly 1 line (2 on panic).
  AC2 PASS: with an unformatted .team/worktrees/zz/bad.go (git-ignored) make lint rc=0; with an unformatted new internal/httpx/zz_bad.go, and with a tracked router.go made unformatted, make lint rc=2 printing 'gofmt needed on:' plus the file name. Temp files removed.
  AC3 PASS: golangci-lint 2.8.0: 'golangci-lint run' 0 issues; '--build-tags integration' 0 issues; README Development records 2.8.0.
  AC4 PASS: openapi.yaml parses (ruby YAML), /healthz get has 200/404/405; 404 -> components/responses/NotFound and 405 -> MethodNotAllowed (Allow header), both content application/json with schema #/components/schemas/Error. Live 405 response confirmed Allow header and error envelope.
  AC5 PASS: go build ./..., go test -race ./... all ok (httpx -count=3 ok); make lint ok; newman (npx) run twice back to back against the real binary, Health + Edge cases folders (Manual folder skipped): 7 requests, 13 assertions, 0 failures both runs.
  Non-blocking: (a) make lint uses 'xargs gofmt -l' with no -r: on GNU xargs an empty file list would make gofmt read stdin; irrelevant while the repo has Go files, and BSD xargs is fine. (b) filenames with spaces would break the xargs pipeline (use -print0/-0 if it ever matters). (c) a mux path-clean redirect (307, e.g. /v1/secret/../secret/x) logs the route pattern of the cleaned target; harmless, no leak. (d) a client-supplied X-Request-Id (8-64 alnum) is echoed into the log by design; pre-existing.
- 2026-10-07 06:13Z · leader · merged a0d9429 (squash). Review: route pattern passed to the outer logger via a context holder as designed, no raw path or query in logs, make lint scope fixed (verified in the main checkout), 9 golangci-lint findings fixed, OpenAPI documents 404/405. Non-blocking: the lint pipeline uses xargs without -r (GNU xargs on an empty file list) and git ls-files -c also lists tracked files deleted in the working tree. For owner to check: nothing user-visible.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-029 — Replace the frozen MinIO dev image with a maintained S3-compatible store
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** tech-debt
- **Milestone:** M1
- **Depends-on:** T-002
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-07 03:07Z by leader
- **Comments-seen:** 1

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/07-replace-the-frozen-minio-dev-image-with-a.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

MinIO no longer publishes container images (Docker Hub and Quay pulls fail), so the local stack and CI (T-009) use the frozen bitnamilegacy/minio:2025.4.22-debian-12-r2, which receives no security patches.

#### Comments
- 2026-10-07 03:16Z · leader · QA note: bitnamilegacy/minio works (arm64 native) but is AGPL-3.0, unsupported by Broadcom, gets no CVE fixes, and the Docker Hub namespace can vanish. Fine for dev-only and loopback-only use. Until this task runs, pin the image by digest in docker-compose.yml and the T-009 CI service so a vanished tag cannot silently change what we run.

### T-030 — T-002 follow-ups: isolate compose stacks, fail fast on auth errors, test-DB grants
- **Status:** DONE
- **Priority:** P2
- **Type:** tech-debt
- **Milestone:** M0
- **Depends-on:** T-002
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-030-t-002-follow-ups-isolate-compose-stacks-
- **PR:** https://github.com/danyaa666/smemories/pull/7
- **Updated:** 2026-10-07 14:03Z by leader
- **Comments-seen:** 7

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/06-t-002-follow-ups-isolate-compose-stacks-fail.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

T-002 follow-ups: isolate compose stacks, fail fast on auth errors, test-DB grants

#### Comments
- 2026-10-07 06:25Z · dev · PR #7. Tests: go test -race ./internal/db (new TestWaitReady*, TestOpenRejectsEmptyDSN); AC1/AC3 shown with isolated stacks. QA: set MYSQL_PORT etc in .env for a second checkout; main-checkout project name is now the dir name (the shared 'smemories' stack was started from other dirs, retire with COMPOSE_PROJECT_NAME=smemories docker compose down). NOTE: while testing I accidentally ran a REVOKE/GRANT of the smem_test_% grant on the shared smemories-mysql-1 (bare docker compose from my scratch dir resolved to the fixed name); last state seen: grant MISSING there. Restore: docker exec smemories-mysql-1 sh /docker-entrypoint-initdb.d/10-test-grants.sh (my attempt was denied by the permission classifier).
- 2026-10-07 06:42Z · leader · Conflict only, before QA: PR #7 conflicts with develop since T-003 (web targets in the Makefile and README) and T-006's neighbours landed. 1) Merge origin/develop into the branch (no force-push) and resolve the Makefile and README as the union; keep your COMPOSE_PROJECT_NAME and grants logic in make up. 2) Re-run make lint build test and test-integration (dev root DSN via SMEM_TEST_DB_DSN while the shared stack's smem_test_% grant is missing; do not touch the shared containers; your isolated two-stack check can be re-run quickly). 3) Change nothing else, then READY_FOR_QA.
- 2026-10-07 07:37Z · leader · Leader FYI: T-004 (CI) is merged, so make lint now runs golangci-lint v2.8.0 with gosec and bodyclose (see .golangci.yml; install per the README, binary at ~/go/bin if not on PATH). After you merge origin/develop, fix any findings in your own code with explicit range checks, never by disabling a linter or editing .golangci.yml, and make sure the CI jobs go, go-integration, web and security pass on the PR.
- 2026-10-07 11:03Z · dev · Conflict-only rework: merged origin/develop (Makefile conflict resolved as the union: COMPOSE_PROJECT_NAME block kept + web targets in .PHONY; README merged cleanly, both changes present). No other change. make lint (golangci-lint v2.8.0 clean) build test test-integration green (dev root DSN). Re-checked AC1/AC3 on an isolated stack (project t030chk, ports 23306/29000/29001): make up grants smem_test_% and smemories account runs the db integration tests there; stack removed with down -v. Shared smemories containers untouched. CI on the PR: go, go-integration, web, security all pass.
- 2026-10-07 11:10Z · qa · Fresh clone of PR #7 head 6c22600 (two clones in dirs qa-t030-a and qa-t030-b, isolated compose stacks on MYSQL_PORT 23306/24306, MINIO 29000/29100, console 29001/29101; shared smemories-mysql-1 and smemories-minio-1 never touched, still 'Up 8 hours' at the end, their grants and databases unchanged; all my stacks removed with down -v). CI: gh pr checks 7 = go, go-integration, security, web, dependabot all pass. EVIDENCE BY AC. AC1: no COMPOSE_PROJECT_NAME in the env; 'make up' in qa-t030-a and qa-t030-b both succeeded at the same time, containers qa-t030-a-{mysql,minio}-1 and qa-t030-b-*, separate volumes qa-t030-{a,b}_{mysql,minio}-data (the name comes from the directory); 'make down' in a removed only a's containers and network (volumes kept as documented), b stayed healthy (docker ps healthy, MinIO ready 200 on 29100, mysqladmin ping alive). Makefile logic read: COMPOSE_PROJECT_NAME is only derived when empty (shell or .env wins), the old name 'smemories' is kept for a checkout whose containers carry that name AND working_dir label equals CURDIR. For the MAIN checkout (/Users/unisoft/GolandProjects/awesomeProject1): I evaluated the Makefile shell expression read-only against the live labels: smemories-mysql-1 has working_dir .../scratchpad/t2 and smemories-minio-1 has .../.team/worktrees/T-002, neither equals the main checkout, so it would resolve to project 'awesomeproject1', i.e. a NEW project; it does not rename or recreate 'smemories' (it would fail on ports 3306/9000 while the shared stack holds them). A checkout whose dir matches a label (simulated with the T-002 worktree) keeps 'smemories'. AC2: unit tests TestWaitReadyFailsFastOnPermanentErrors (1045 and 1049, one attempt, <1 s, error names address and user), TestWaitReadyRetriesTransientErrors (1040 retried, ok on 3rd), run -race -count=3 green. Live with the built smemories-api against the isolated MySQL: wrong password (fake) -> rc 1 in 0.4 s 'database at 127.0.0.1:24306 refused user "smemories" ... Error 1045', the password string appears 0 times in the output; unknown database as root -> rc 1 in 0.0 s with Error 1049; connection refused (port 24399) -> retries and fails after 10.0 s 'not reachable after 10s'; MySQL stopped at start then started 3 s later -> /readyz 200 at about 4 s (retry works). AC3: on the isolated stack I ran REVOKE of the smem_test_% grant (SHOW GRANTS then lists only smemories.*), 'make test-integration' failed with Error 1044 on every CREATE DATABASE; 'make up' re-applied the grant (SHOW GRANTS lists it again), a second 'make up' is idempotent (still 4 lines), and 'make test-integration' with the smemories account (SMEM_TEST_DB_DSN smemories@...:24306) passed: config, db, httpx ok; no leftover smem_test_* databases. AC4: empty SMEM_DB_DSN, binary exits rc 1 immediately in SMEM_ENV=dev, prod, unset ('config error: SMEM_DB_DSN is required') and in SMEM_ENV=test ('SMEM_DB_DSN: DSN is empty'), never dials 3306; a whitespace-only DSN -> 'DSN is not valid'. AC5: README Development section documents the per-checkout project name, MYSQL_PORT/MINIO_PORT/MINIO_CONSOLE_PORT (and matching the port in SMEM_DB_DSN and SMEM_TEST_DB_DSN), the legacy 'smemories' name and how to retire it, and that 'make up' re-applies the smem_test_% grants; SMEM_DB_DSN doc updated. Mechanical: make lint (golangci-lint v2.8.0 '0 issues.') build test rc 0; make test-integration green on the isolated stack. NON-BLOCKING OBSERVATIONS: (1) a nonexistent database name as the NON-privileged smemories account returns MySQL Error 1044 (not 1049), because MySQL checks the privilege first (reproduced: DSN .../nosuchdb -> retries the full 10 s, then fails with the 1044 text). AC2 names only 1045 and 1049 so it is met; adding 1044 (access denied to database) to permanent() would make a typo in the database name, and the missing smem_test_% grant fault, fail at once too. (2) a bare 'docker compose ...' (outside make) in any checkout still resolves to the compose file's 'name: smemories' and therefore the SHARED project (I saw this with a read-only 'docker compose ps' from qa-t030-b, it listed smemories-*); only 'make' sets the derived name. A bare 'docker compose down -v' in a checkout would wipe the shared stack; consider mentioning in the README that compose commands must go through make or COMPOSE_PROJECT_NAME. (3) the grants in SHOW GRANTS appear as smem\\_test\\_% (doubled backslashes); that is only the display of the escaped pattern, the grant works (tests ran), so the earlier 'mis-escaped grant' suspicion on the shared MySQL was a false alarm: its real fault is the revoke the dev reported.
- 2026-10-07 11:12Z · leader · merged 7c8b302 (squash). Review: per-checkout compose project names, an existing smemories stack keeps its name, make up re-applies the test grants, permanent DB errors fail at once without leaking the password, empty DSN rejected everywhere. For owner to check: your running shared stack is still the old smemories project; see the note in the README before running make up from the main checkout (its default ports are taken by that stack). Follow-up in T-032.
- 2026-10-07 14:03Z · leader · accepted by owner (chat, 2026-10-07: 'Accept all')

### T-031 — Auth hardening for go-live: edge rate limits, shared limiter, session purge, stored-hash caps
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** security
- **Milestone:** M2
- **Depends-on:** T-023
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-07 06:30Z by leader
- **Comments-seen:** 1

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/05-auth-hardening-for-go-live-edge-rate-limits.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

Items from the T-006 review that only matter at scale or in production: (1) the in-memory rate limiter is per process and keyed by unbounded client-chosen values; put a coarse limit at the edge (CloudFront/WAF) and move the counters to a sh

#### Comments
- 2026-10-07 07:07Z · leader · From the T-006 review: also require at least one visible character in a display name (a name made only of U+200D passes today), preferably through one shared helper that T-008 and later text fields reuse.

### T-032 — db.Open: treat MySQL 1044 as permanent; README warning about bare docker compose
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** tech-debt
- **Milestone:** M1
- **Depends-on:** T-030
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-07 11:12Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/08-db-open-treat-mysql-1044-as-permanent-readme.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

db.Open: treat MySQL 1044 as permanent; README warning about bare docker compose

#### Comments

### T-033 — CI: do not cancel in-progress runs on develop and main
- **Status:** MERGED
- **Priority:** P1
- **Type:** infra
- **Milestone:** M1
- **Depends-on:** T-004
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-033-ci-do-not-cancel-in-progress-runs-on-dev
- **PR:** https://github.com/danyaa666/smemories/pull/22
- **Updated:** 2026-10-08 08:34Z by leader
- **Comments-seen:** 7

**Spec — read this first, it is the source of truth:** `.team/epics/E01-foundation/09-ci-do-not-cancel-in-progress-runs-on-develop.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E01-foundation · **PRD:** `.team/epics/E01-foundation/PRD.md`

Evidence from the T-006/T-030 merges: the workflow's concurrency group has cancel-in-progress: true for every event, so a push to develop cancels the still-running CI of the previous push (the leader's board-sync commits do this right after each merge).

#### Comments
- 2026-10-08 02:24Z · leader · Merge-commit CI runs on develop keep getting cancelled by board-sync pushes; ready.
- 2026-10-08 02:47Z · leader · Leader: raised to P1. Merge commits on develop (d209266 T-011) were cancelled by later board-sync pushes, so develop is not getting a full CI pass per merge. Do this first.
- 2026-10-08 02:47Z · dev · ci.yml: cancel-in-progress only for pull_request; push events group by sha so each merge commit runs fully; mysql digest-pinned; docs/ci.md sentence. Test: PR CI green; push twice to develop and check both runs finish. Needs owner approval (risk high).
- 2026-10-08 03:19Z · qa · QA T-033 PASS (one gap, see 4). Evidence: 1) PR #22 CI run 37719629443 on head: go, go-integration, security, web all green; go-integration pulled the digest-pinned mysql image and passed. 2) actionlint (rhysd/actionlint:latest via docker) on ci.yml: no syntax/expression errors; only 2 pre-existing shellcheck SC2034 warnings (unused loop var i) in the wait loops at ci.yml:89/106, not from this change. 3) Pin check: docker buildx imagetools inspect mysql:8.4 digest = sha256:6ea90827...b242, identical to the pinned digest (multi-arch index digest, fine). Concurrency block read: pull_request -> group ci-<refs/pull/N/merge>, cancel true (unchanged behaviour for PRs); push (develop/main) -> group ci-<sha>, cancel false; github.ref is non-empty on PRs so the && || ternary cannot fall to sha there. Only triggers are pull_request and push [develop, main], no other event can hit the sha branch oddly. Old behaviour confirmed in gh run list --branch develop: 8 of the last 15 develop push runs were conclusion=cancelled (e.g. 7360aa7, 4db0533, 8b027a2, d209266), which is exactly the defect. 4) NOT verified live: a real double push to develop (I may not push develop, and creating a scratch GitHub repo to replay the workflow was denied by the permission system). So the 'two pushes within a minute both finish' AC rests on the expression + GitHub semantics, not on an observed run. Suggest the leader checks the first two back-to-back develop pushes after the merge (gh run list --branch develop: none cancelled). 5) docs/ci.md matches the workflow (sentence says PR runs cancel, develop/main never; no other doc mentions cancel/concurrency/mysql tag). Non-blocking: that docs sentence is one very long line (docs/ci.md:4); the workflow comment says 'a queued run' dropped in a shared group, correct per GitHub semantics (one pending per group). Risk high: needs owner approval before merge.
- 2026-10-08 03:21Z · leader · Leader review OK on head 74346f4 (QA_PASS on the same head, CI 4/4 green, actionlint clean apart from two old shellcheck notes). Concurrency group is the PR ref for pull_request events (cancel in progress) and the commit SHA for pushes (never cancelled), the documented idiom; mysql image pinned to the current manifest digest; docs/ci.md matches; no permission, secret or action change. Not verified live (QA could not push to develop): the first back-to-back develop pushes after the merge must show no cancelled run; I will check with gh run list. Note for later: the digest pin means MySQL patch updates are manual; docs/ci.md should say how to refresh it (add when T-004's follow-ups are next touched). Awaiting owner approval.
- 2026-10-08 08:34Z · human · owner approved merge
- 2026-10-08 08:34Z · leader · merged 96059a6 (squash), owner-approved in chat 2026-10-08, QA_PASS on head 74346f4. Pushes to develop and main now get one concurrency group per commit (never cancelled); PRs still cancel superseded runs; mysql image pinned by digest. To verify live: the next back-to-back develop pushes must show no cancelled run (gh run list --branch develop).

### T-034 — Public note submission (text and photos)
- **Status:** IN_QA
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-012, T-009, T-036, T-043
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** qa
- **Branch:** task/t-034-public-note-submission-text-and-photos
- **PR:** 30
- **Updated:** 2026-10-08 09:07Z by qa
- **Comments-seen:** 2

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/02-public-note-submission.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Public endpoint where a friend with a collection link submits a note with text, emoji and up to three photos; stored as pending.

#### Comments
- 2026-10-08 08:44Z · leader · Leader: T-043 is merged (0032e6f), you are unblocked. First commit: change the Vietnamese label of field how_we_met from 'Chúng ta quen nhau thế nào' to 'Chúng mình quen nhau thế nào' in internal/notefields (owner-approved wording). Spec updated with the template-driven answers design; read it again.
- 2026-10-08 09:03Z · dev · PR #30 head e90e331. How to test: make lint build test test-integration (all green); newman from postman/ (notes collection, 71 req / 135 assertions, run twice). Decision for leader: AC7 numbers (10/hr, 40/day per IP) contradict the later leader note (class behind one IP); I used 100/hr + 300/day per IP and 60/hr per collection. Verifier sees headers only. Migration 0010; TestMigratePageSizeLetter now rolls back two migrations. Not covered by tests: real 40 MiB multipart over a socket (httptest only).

### T-035 — T-010 follow-ups: template tests iterate templates.List()
- **Status:** MERGED
- **Priority:** P3
- **Type:** tech-debt
- **Milestone:** M1
- **Depends-on:** T-010
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-035-t-010-follow-ups-template-tests-iterate-
- **PR:** 26
- **Updated:** 2026-10-08 03:48Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E05-templates-export/05-t-010-follow-ups-template-tests-iterate.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E05-templates-export · **PRD:** `.team/epics/E05-templates-export/PRD.md`

From the T-010 QA notes: adding a template exactly as docs/templates.md describes fails TestListHasBuiltIns and needs a Go edit to TestSamples.

#### Comments
- 2026-10-08 02:24Z · leader · T-010 is accepted: small follow-up, ready.
- 2026-10-08 03:32Z · dev · Test-only: TestListHasBuiltIns and TestSamples iterate templates.List(); built-ins asserted as minimum. Test: go test ./internal/pdf ./internal/templates.
- 2026-10-08 03:48Z · qa · Head 15d0d97, PR #26 CI green (go, go-integration, security, web). Test-only diff (internal/pdf/book_test.go, internal/templates/templates_test.go). Evidence: go test ./internal/pdf ./internal/templates -count=1 ok; go vet and gofmt clean. Mutation check: dropped a 3rd template (zzz.json, copy of classic) into internal/templates/embed with no Go edit -> TestListHasBuiltIns, TestSamples and the whole pdf/templates packages still pass; file removed afterwards. Built-ins still asserted as minimum (classic and modern must be present; pages==6 and truncation warning pinned for them only). Non-blocking: TestRenderBothTemplatesBothSizes still hardcodes the two ids (outside this task's scope).
- 2026-10-08 03:48Z · leader · merged a01a884 (squash), QA_PASS on the same head 15d0d97, CI green. Review: test-only; TestListHasBuiltIns and TestSamples iterate templates.List(), classic and modern stay a minimum with their exact page counts pinned; QA added a third template file without any Go edit and both tests passed. For owner to check: nothing to run.

### T-036 — Bound the memory of image processing (caps, concurrency, memory limit)
- **Status:** MERGED
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-009
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-036-bound-the-memory-of-image-processing-cap
- **PR:** https://github.com/danyaa666/smemories/pull/23
- **Updated:** 2026-10-08 03:21Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E03-yearbooks/04-bound-the-memory-of-image-processing.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E03-yearbooks · **PRD:** `.team/epics/E03-yearbooks/PRD.md`

Make the worst-case memory of the T-009 image pipeline a measured, enforced number (header-based caps, lower default concurrency, memory limit knob), before T-034 exposes it to anonymous uploads.

#### Comments
- 2026-10-08 01:59Z · leader · T-009 is merged: ready for dev. It must land before T-034.
- 2026-10-08 02:56Z · dev · Root cause: x/image Scale float64 scratch (dst w x src h x4 = 480MB at 50MP) -> banded resize. Header caps (PNG/WebP 25MP, JPEG 50MP, decoded est. 128MiB incl. JPEG SOF subsampling/progressive), concurrency default 2, SMEM_MEMORY_LIMIT_MIB, docs/media.md. Test: TestLimitsFromHeader, TestJPEGSamples, TestResizeBandsMatchOneShot; measure with TestMeasurePeak (see docs/media.md). Dev: 4x16-bit PNG 4096^2 at defaults = 970MB RSS. Re-run media Postman twice.
- 2026-10-08 03:19Z · qa · QA T-036 PASS. Method: built the PR head, ran the real API (fresh process per measurement; peak RSS sampled with ps every 20ms) against MySQL+MinIO from make up; images generated by me (Go/PIL/cwebp, all under 10 MiB upload cap). NOTE .env.example in the main checkout is still the old one (concurrency 4): I loaded the PR's .env.example and also tested with the variables truly unset. AC6 (macOS arm64, defaults = concurrency 2): 4 parallel 16-bit PNG 4096x4096: peak 925-929 MiB (970-974 MB) in 3 runs, mixed 4 (16-bit PNG at cap, 25 MP 8-bit PNG, 50 MP 10000x5000 JPEG, 25 MP lossless WebP): 862/812/886 MiB. All <= 1.2 GiB (1229 MiB). Single 16-bit PNG: 478 MiB. Control with concurrency 4: 1735-1842 MiB (so the lowering is what matters). 4x each alone: 50MP JPEG 700 MiB, 6000x7456 4:4:4 JPEG at the cap 1008 MiB (highest seen), 4200x4200 progressive 4:2:0 JPEG 565 MiB, 12000x4000 JPEG 595 MiB, lossless WebP 25MP 843 MiB, lossy WebP 24MP 507 MiB, 5000x5000 8-bit PNG 850 MiB. Linux (target OS): same linux/arm64 binary in a debian container with --memory=1536m, cgroup memory.peak: 4x 16-bit PNG 905 MiB, mixed 4 855 MiB, 8 parallel mixed incl. 4:4:4 and progressive JPEG 873 MiB, no OOM kill; with SMEM_MEMORY_LIMIT_MIB=1100 4x16-bit PNG 909 MiB. AC2 caps just below/above via the API: 16-bit PNG 4096x4096 201 / 4097x4096 400; 8-bit PNG 5000x5000 201 / 5001x5000 400; JPEG 10000x5000 201 / 10001x5000 400; lossless WebP 5000x5000 201 / 5001x5000 400; 4:4:4 JPEG 6000x7456 (134208000 B) 201 / 6000x7457 400; progressive 4:2:0 4200x4200 201 / 4300x4300 400; each over-cap answer in ~8 ms before any decode. AC3: SMEM_MEDIA_MAX_CONCURRENT 0/abc/-2 and SMEM_MEMORY_LIMIT_MIB 10/63/abc/-1/1.5/1048577 all fail at startup naming the variable; limit 64, 256, 700 with the 4 mixed uploads: all 201, process alive, RSS 481/479/744 MiB (limit works, nothing crashes or is refused); startup logs 'memory limit set'. .env.example and README document both. AC1/AC4: docs/media.md exists; its numbers reproduce (970 MB for 4x 16-bit PNG; single 490 vs my 501 MB). AC5: go mod tidy leaves no diff, aws-sdk-go-v2, /credentials, /service/s3 are in the direct require block. Postman: media collection run twice via newman 6 (API restarted between runs; I did the pair twice): 44 requests, 77 assertions, 0 failures each time. Mechanical: go build, go vet, gofmt, golangci-lint (0 issues), go test -race ./... ok, go test -race -count=1 -tags integration ./... all packages ok; PR CI green (run 37720320278). Non-blocking: (a) the media Postman collection has no entry for the new caps (E11 only covers the 60000x60000 header); a tiny PNG header claiming 5001x5000 would exercise the 25 MP PNG cap cheaply; (b) the main checkout's .env.example still says concurrency 4 until merge, fine; (c) RSS on macOS never returns to baseline after a run (Go retains pages), so measure each scenario on a fresh process, as docs/media.md implies; (d) 6000x7456 4:4:4 baseline JPEG is the heaviest accepted input at ~1.0 GiB for 4 parallel, only ~18% under the 1.2 GiB target, so keep an eye on it if limits are raised.
- 2026-10-08 03:21Z · leader · merged 7772eb7 (squash), QA_PASS on the same head c6d78ce, CI green. Review: the 480 MB scaler buffer is gone (banded resize, same picture), header-based caps by colour model and JPEG subsampling, concurrency default 2, SMEM_MEMORY_LIMIT_MIB validated (0 or 64..1048576), 4 worst-case uploads peak at about 925 MiB (was 3.2 GB). For owner to check: PNG/WebP over 25 MP, 16-bit PNG over 4096x4096 and large progressive JPEGs are now refused with 400 invalid_image; phone JPEGs up to 50 MP still pass. Follow-up: stale OpenAPI sentence (noted in T-034 spec).

### T-037 — US Letter page size, end to end
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** —
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** yes
- **Assignee:** —
- **Branch:** task/t-037-us-letter-page-size-end-to-end
- **PR:** 27
- **Updated:** 2026-10-08 08:44Z by leader
- **Comments-seen:** 5

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/01-us-letter-page-size-end-to-end.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Add Letter (215.9 x 279.4 mm) as a third yearbook page size: migration, API, OpenAPI, PDF page geometry, template reference page.

#### Comments
- 2026-10-08 03:38Z · dev · Letter end to end: migration 0009 (up/down tested on A5/A4/Letter books), API+OpenAPI+schema.d.ts, templates.Dims/reference/ForPageSize, pdf MediaBox 612x792, docs, Postman yearbooks (steps 21-24 + 3 edge cases, newman x2 green). Test: go test -tags integration ./...; newman. Gap: npm check:api not run locally (CI).
- 2026-10-08 03:48Z · qa · Head 9546d3b = PR #27 head; CI green on it (go, go-integration, security, web).
  AC1 migration: real smemories-migrate binary on scratch DB smem_test_qa37 (dropped after): up, down (enum A5,A4), insert A5+A4 books, up (enum A5,A4,Letter), insert Letter book, down -> Letter row became A5, A4/A5 untouched, enum restored to ('A5','A4'); up again -> enum with Letter, rows intact; status shows 0009 applied. Dev's TestMigratePageSizeLetter also passes.
  AC2 API (real API on :18080, curl): create Letter/A4/A5 = 201; missing page_size defaults A5; 'letter','LETTER','Legal','',' Letter','Letter ' = 400 invalid_page_size on create AND patch; number -> 400 invalid_body. PATCH A4->Letter->A4->Letter->A5 all 200 and persisted; GET/list show Letter; patching title keeps size. openapi.yaml + schema.d.ts updated. npm run check:api (main checkout web/node_modules, PR-head openapi.yaml and schema.d.ts copied to a scratch dir, nothing in main touched): 'api schema ok'; negative control with a stale schema fails as it should. web has no page_size usage, so nothing else breaks.
  Postman: newman run postman/yearbooks.postman_collection.json (baseUrl :18080), run 1: 92 requests, 189 assertions, 0 failed; API restarted; run 2: 92/189/0 failed. Collection has steps 21-24 (Letter create/patch) and edge cases Legal, 'letter', 'LETTER'(patch). Note: no create-with-'LETTER' entry, only patch (covered manually above, non-blocking).
  AC3 PDF: go test TestPageBoxes ok; independent check with pdfinfo on full 6-page PDFs rendered at each size: every page A5 419.53x595.28, A4 595.28x841.89, Letter 612x792 (pdfinfo says 'letter').
  AC4 validator: dev table tests pass (A5 ref+Letter fails, Letter ref+A5/A4 fail, bad reference, coordinate outside Letter page). My probes via templates.Parse: A5 ref+A4 ok, Letter ref+Letter ok, A5 ref+[A4,Letter] fails naming template classic, 'Legal' unknown size, reference 'letter' rejected, reference number rejected, element x=100,w=120 on Letter ref rejected as outside 215.9x279.4; classic/modern unchanged and pass.
  AC5: TestForPageSize ok; Letter yields neither classic nor modern; A5/A4 yield both; A3 none; Render with Letter on classic/modern errors 'does not support page size' (TestRenderRefusesUnsupportedSize).
  AC6: docs/templates.md documents reference, the three sizes with MediaBox table, and the aspect rule.
  Mechanical: go build, go vet (+integration tag), gofmt, golangci-lint 0 issues, go test -race ./... and go test -race -tags integration ./... all ok.
  Non-blocking observations: (a) reference 'letter' (lower case) yields a second misleading error 'different aspect ratio than the letter reference page' besides the correct 'invalid reference' one; (b) templates.Dims is an exported mutable map; (c) duplicate entries in page_sizes are accepted (pre-existing); (d) explicit reference empty string is accepted as the A5 default. RISK:HIGH: owner approves the merge.
- 2026-10-08 03:49Z · leader · Leader review OK on head 9546d3b (QA_PASS on the same head, CI 4/4 green; the PR merges cleanly into develop after T-035). Migration 0009 is additive (enum value appended, metadata-only in MySQL) and its Down turns Letter books into A5 before restoring the enum; QA ran up, down, up on real data. Validation is a fixed list, the aspect-ratio rule (1% tolerance) keeps Letter apart from A5/A4, ForPageSize and the render guard stop a template from being used on a size it does not declare, MediaBox is 612x792 pt for Letter. Non-blocking, moved to the T-038 spec: exported mutable Dims map, double error for an invalid reference; Postman has no create-with-LETTER entry (patch is covered, create tested by hand). Awaiting owner approval.
- 2026-10-08 08:43Z · human · owner approved merge
- 2026-10-08 08:44Z · leader · merged 6044ea3 (squash), owner-approved ('OK' to Q-013), QA_PASS on head 9546d3b, CI green. Review: migration 0009 additive (Letter appended to the enum, Down turns Letter books into A5), aspect-ratio rule keeps Letter apart from A5/A4, ForPageSize and the render guard prevent a template on a size it does not declare, MediaBox 612x792 pt. For owner to check: nothing to run yet (no UI for Letter until T-016).

### T-038 — Template format v2: backgrounds, static text, rotation, ellipse, font families
- **Status:** MERGED
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-035
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-038-template-format-v2-backgrounds-static-te
- **PR:** 28
- **Updated:** 2026-10-08 09:14Z by leader
- **Comments-seen:** 6

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/02-template-format-v2-backgrounds-static-text.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Extend the template JSON and renderer with page background images, localised static text, rotation, ellipse photo masks and several embedded font families.

#### Comments
- 2026-10-08 04:05Z · dev · PR #28, stacked on T-037 (#27, merged into the branch; T-038 is commit 6e810cf). All AC + the two T-037 review notes (PageDims, single invalid-reference error). Test: go test -race ./internal/templates ./internal/pdf ./internal/pdf/fonts; budget: go test ./internal/pdf -run '^$' -bench LetterBook -benchtime 1x. Gaps: PNG backgrounds checked for placement/order only (JPEG byte-identical probe); 8 MiB total rule unreachable with 4 page kinds, unit-tested on the validator.
- 2026-10-08 08:42Z · qa · PR #28 head 826da58 (dev 6e810cf + my test-only commit), CI green on that head (go, go-integration, security, web). Mechanical: make build, make lint, gofmt, go vet clean; go test -race ./... pass; templates/pdf/fonts/yearbook -race -count=2 pass (no flakes).
  AC1 background: dev tables + my probes. Wrong ratio, <150/>400 DPI, over-size file (limit 1.5 MiB; my 1.8 MiB 400-DPI RGBA rejected, 1.4 MiB accepted), JPEG named .png accepted by content, '..', other template folder, sub folder, abs path, backslash, bad ext, two per page, in flow, with box/slot/rotate all rejected with template+page+file in the error. Draw order: pdftoppm of my render shows paper, then background, then shapes/text on top; pixel probes on PNG and JPEG backgrounds match asset (green/red/blue at probes, full page incl. corners).
  AC2 static text: validator table (both langs, 500/501 chars, control chars, slot+text, neither, label, 3rd language) pass; render en and vi both correct (read back from PDF and rasterised), lang fr/empty falls back to en; very long text in 20x6 mm box gives one text_truncated without slot, no crash.
  AC3 rotation: +/-45 accepted, 45.01/-46/90 rejected; box-outside-page rejected before rotation; rendered +45 is clockwise (visual), -45 counter-clockwise; q/Q balanced; clip rotates with image.
  AC4 ellipse: circle and non-square accepted, radius rejected; rendered ellipse clip visible, rotated ellipse OK.
  AC5 fonts: unknown family/role, font!=fonts.body, no body all rejected; display role renders Vietnamese (tone marks) and emoji; registry coverage test + Register panics on no-Vietnamese family (Emoji font) and bad TTF; display family only embedded when used.
  AC6/AC7 classic.pdf and modern.pdf regenerated with -write-samples: shasum identical to origin/develop (byte-identical); docs/templates.md documents all fields, limits and examples; shared tests iterate List().
  Budget: 24 pages, 30 photos, Letter, backgrounds: benchmark 14 ms, 52 MB peak heap, 7.3 MB PDF. My worst case, 4 distinct 3400x4400 (400 DPI) RGBA PNG backgrounds of 1.4-1.5 MiB each, same book: 0.72 s, 295-358 MB peak heap, 324 MB RSS: inside 60 s / 512 MB but the closest to the limit; RGB or JPEG backgrounds are far cheaper (docs already warn).
  Added test: internal/pdf/background_formats_test.go (rgb, nrgba, gray, gray16, rgba64, palette PNG backgrounds validate and render).
  Non-blocking: (1) an Adam7-interlaced PNG passes the validator (DecodeConfig only) but Render fails with 'interlacing not supported in PNG buffer'; build tests that render every template would catch a real asset, but rejecting interlace (IHDR byte 12 != 0) in templates/validate.go would honour 'bad asset fails the tests, not the request'. (2) Static text is not checked for glyph coverage at validation; a missing glyph only shows as a missing_glyph warning at render. (3) T-037 parts: no regression seen.
- 2026-10-08 08:44Z · leader · Leader: T-037 is merged to develop (6044ea3, squash) so PR #28 now conflicts in docs/templates.md, internal/pdf/render.go, internal/templates/spec.go and internal/templates/validate.go (your branch carries T-037's original commits plus your T-038-on-top edits such as PageDims). Merge origin/develop into the task branch (no rebase of a pushed branch), resolve keeping the T-038 versions of the lines you changed on top of T-037 and develop's version of everything else, re-run make lint build test test-integration, regenerate the samples (classic.pdf and modern.pdf must stay byte-identical), push, READY_FOR_QA with the merge-commit SHA. Also in this round (QA finding, small): the validator must reject an interlaced (Adam7) PNG background, because Render fails on it; add a test with an interlaced fixture generated in the test. Static-text glyph coverage at validation stays out of scope.
- 2026-10-08 08:48Z · dev · Merge commit b356781 (origin/develop merged, conflicts in docs/templates.md, pdf/render.go, templates/spec.go+validate.go resolved to the T-038 lines on top of T-037 incl. PageDims; kept render.go slices import). QA finding 1: validator rejects interlaced PNG (IHDR interlace byte), test TestBackgroundRejected case 'interlaced PNG' with a fixture built in the test, documented in docs/templates.md. classic.pdf/modern.pdf regenerated: unchanged. make lint build test test-integration all green.
- 2026-10-08 09:07Z · qa · Re-verify of merge result b356781 (origin/develop merged, interlace rejection added). CI on b356781: go, go-integration, security, web all pass. Local in worktree: make build, make lint (exit 0), gofmt, go vet, go test -race ./... pass; templates/pdf/fonts/yearbook -race -count=2 pass; make test-integration pass. New rule: validator reads IHDR interlace byte (offset 28) and rejects with 'interlaced PNG' error (TestBackgroundRejected case 'interlaced PNG', fixture built in test; offset checked by hand: 8 sig + 8 chunk header + 12 into IHDR data = interlace byte); documented in docs/templates.md line 89. Samples: go test ./internal/pdf -run TestSamples -write-samples, git status clean, shasum classic.pdf 6a63214e..., modern.pdf cedd1c22... identical to origin/develop. My earlier test internal/pdf/background_formats_test.go still passes; all earlier AC evidence (AC1-AC7) stands, merge changed nothing in those paths other than the interlace check. Non-blocking: interlace test uses a flagged-but-not-really-Adam7 fixture (only the IHDR byte matters to the check; Go cannot encode Adam7, so fine); static-text glyph coverage out of scope per leader.
- 2026-10-08 09:14Z · leader · merged eb6abef (squash), QA_PASS on head b356781 after one rework (merge with develop, interlaced PNG rejection), CI green. Review: background drawn right after the paper fill and registered once per asset, rotation and clipping closed by deferred ends, font families loaded per template with a Vietnamese coverage gate, validator checks backgrounds by content (type, 1.5 MiB, ratio, DPI 150..400, no interlacing, 8 MiB per template), static text needs both languages, classic and modern samples byte-identical, 24-page Letter book with full-page backgrounds: 14 ms and 52 MB heap (worst case 0.72 s and 358 MB). For owner to check: docs/templates.md section on format v2.

### T-039 — Design import tool (dev only): canvas page to template draft
- **Status:** TODO
- **Priority:** P2
- **Type:** tech-debt
- **Milestone:** M1
- **Depends-on:** T-038
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 09:14Z by leader
- **Comments-seen:** 1

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/03-design-import-tool-dev-only.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Dev-only Node/Playwright command that renders decoration backgrounds and reads slot and text boxes from a Claude Design canvas into a draft template.

#### Comments
- 2026-10-08 09:14Z · leader · T-038 is merged: the import tool can target the v2 format. Spec: .team/epics/E08-designer-templates/03-design-import-tool-dev-only.md.

### T-040 — Template memory-book from design temp1 (pilot 1)
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-037, T-038, T-039, T-035, T-044
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 03:19Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/04-template-memory-book-from-temp1.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Pilot 1: the pastel Memory Book design as a system template with four page kinds in English and Vietnamese.

#### Comments

### T-041 — Template navy-classic from design temp2 (pilot 2)
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-037, T-038, T-039, T-035, T-044
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 03:19Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/05-template-navy-classic-from-temp2.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Pilot 2: the navy and gold classic design as a system template with four page kinds in English and Vietnamese.

#### Comments

### T-042 — Plan the rollout of designs temp3 to temp8 after the pilots
- **Status:** BACKLOG
- **Priority:** P3
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-040, T-041
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 03:08Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/06-roll-out-designs-temp3-to-temp8.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

Planning placeholder: after the owner reviews the pilots, split into one task per remaining design.

#### Comments

### T-043 — Note field catalogue (internal/notefields): closed set of form fields with EN/VI labels and validation
- **Status:** MERGED
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-043-note-field-catalogue-internal-notefields
- **PR:** https://github.com/danyaa666/smemories/pull/24
- **Updated:** 2026-10-08 08:44Z by leader
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E04-friends-notes/06-note-field-catalogue.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E04-friends-notes · **PRD:** `.team/epics/E04-friends-notes/PRD.md`

Closed catalogue of note form fields (name, relationship, message, how we met, ...) with limits, EN/VI labels and a validator, so forms and notes can be template-driven (D-21).

#### Comments
- 2026-10-08 03:24Z · dev · Catalogue, Validate, Info, Default, docs/note-fields.md; added textx.CleanMultiline (Clean unchanged). make lint build test green. Owner must review the VI label table in the PR description before merge. Test: go test -race ./internal/notefields ./internal/textx. Dup/unknown refs are errors.
- 2026-10-08 03:42Z · qa · AC1 catalogue ids/kinds/limits: TestCatalogueGuard (dev) + my read of catalogue.go -> OK. AC2 EN/VI labels+hints present, NFC, PR table matches code; owner must still review VI table before merge (not mine to approve). AC3 Validate: my probes (internal/notefields/qa_probe_test.go, pushed, CI green on PR head): NFD Vie+U+0302+U+0323 -> composed NFC; limit and limit+1 counted in characters (60 'Đ' ok, 61 rejected; 500/501 wish); CRLF -> LF counts as 1 char; rejected in name and message and at edges: U+202A-202E, U+2066-2069, LRM/RLM, ALM, ZWSP, ZWNJ, WJ, BOM, soft hyphen, U+180E, invisible times, tag char U+E0041, U+0085/2028/2029 inside, DEL, ESC, NUL, invalid UTF-8 (\xff, overlong, surrogate, truncated); accepted unchanged: ZWJ emoji and family emoji, skin tone, VS15/VS16, NBSP inside, Thai combining; whitespace-only (space, \n, NBSP, U+3000, CRLF mix) -> missing_answer for required, dropped for optional; short-text inner \n/\r\n -> invalid_answer; 10 MB ASCII in name/message/wish, 10 MB NFD and 10 MB NUL -> invalid_answer, all five in well under 1 s (byte guard before NFC); unknown answer ids reported in sorted order before missing; input map not mutated; Default()/Info() results do not alias the catalogue; Info(nil) is non-nil. Duplicate refs and unknown/empty/wrong-case ref ids -> error (unknown ids are FieldError unknown_field, errors.Is ErrUnknownField). AC4 Default() test + mine. AC5 Info JSON fixed by TestInfoJSON. AC6 guard tests present. AC7 docs/note-fields.md read: fields + add-a-field rule, matches catalogue. Commands: go test -race -count=3 ./internal/notefields ./internal/textx; go vet; gofmt -l (clean); golangci-lint run on both pkgs (0 issues); go test -race ./... all ok; gh pr checks 24 all pass. Non-blocking observations: (a) spec-mandated byte guard 4*limit before NFC rejects extreme decomposed text (60 x e+U+0302+U+0323 = 300 bytes > 240) even though it is 60 chars after NFC; 48 of them pass; documented in the PR. (b) duplicate-ref error is a plain error, not *FieldError, so errors.As callers (T-034/T-044) must treat any non-FieldError as a template bug. (c) a trailing newline in a short-text value is trimmed, not rejected (OK per trimming rule). (d) a literal U+FFFD in text is rejected (same rule as invalid UTF-8) and tag characters (subdivision-flag emoji) are rejected as Cf per spec.
- 2026-10-08 03:43Z · leader · Leader review OK on head 9b9b08a (QA_PASS; the head includes QA's probe test commit; CI 4/4 green). Pure package with no input surface of its own: the catalogue ids are fixed by a guard test, Validate resolves refs first (unknown or duplicate ref = template bug), rejects unknown answer ids before anything else, applies textx (NFC, no control or format characters except ZWJ and variation selectors; long text allows only \n), and bounds work with a 4-bytes-per-character pre-check. Non-blocking: the byte pre-check rejects extreme all-decomposed text (more than 4 bytes per character), normal Vietnamese in NFD is far below it; a duplicate ref returns a plain error rather than a FieldError (intended). Waiting only for the owner's review of the Vietnamese label table (AC2), then I merge.
- 2026-10-08 08:44Z · leader · merged 0032e6f (squash), owner OK on the Vietnamese labels (Q-012), QA_PASS on head 9b9b08a, CI green. Review: closed catalogue with fixed ids, Validate rejects unknown ids first and bounds work by bytes, textx rules (NFC, no control or format chars except ZWJ and variation selectors). Owner said OK to my recommendation, which included one wording change: 'Chúng ta quen nhau thế nào' becomes 'Chúng mình quen nhau thế nào' (done in T-034, first commit). For owner to check: docs/note-fields.md.

### T-044 — Templates declare note fields (format v2.1): note_fields, note_field slot, NoteFields helper
- **Status:** BACKLOG
- **Priority:** P2
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-038, T-043
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 03:19Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E08-designer-templates/07-templates-declare-note-fields.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E08-designer-templates · **PRD:** `.team/epics/E08-designer-templates/PRD.md`

A template lists which note fields its friend pages ask for and where each answer prints; the API reads the list for the public form.

#### Comments

### T-045 — Register accepts a locale; verification email in the student's language
- **Status:** QA_PASS
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-015
- **Risk:** high
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** leader
- **Branch:** task/t-045-register-accepts-a-locale-verification-e
- **PR:** https://github.com/danyaa666/smemories/pull/29
- **Updated:** 2026-10-08 08:51Z by qa
- **Comments-seen:** 4

**Spec — read this first, it is the source of truth:** `.team/epics/E02-auth/06-register-accepts-locale.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E02-auth · **PRD:** `.team/epics/E02-auth/PRD.md`

Let the register request carry the UI language so Vietnamese students get the verification email in Vietnamese and a correct stored locale.

#### Comments
- 2026-10-08 03:54Z · leader · T-015 merged: ready.
- 2026-10-08 08:42Z · dev · Register accepts optional locale en|vi (else 400 invalid_locale); stored + verification mail follows it; web form sends i18n language; openapi/schema/Postman updated (newman x2 green, step 1 now registers with vi). Tests: TestRegisterLocale (integration), auth.test.tsx. Integration tests need make up + migrate.
- 2026-10-08 08:51Z · qa · PR head 8a9cafb; CI green (go, go-integration, web, security). Evidence (all produced by me, real API on MySQL, own DB):
  AC1 curl against running API: absent/null/en -> 201 locale en; vi -> 201 locale vi; 400 invalid_locale for VI, En, fr, '', 1, 1.5, true, false, [], {}, 'vi ', 'vi-VN'; malformed JSON -> invalid_body. DB users.locale matches; GET /v1/me with session cookie shows en/vi. Bad locales create nothing (no user row, no Set-Cookie). Rate limit: 14 bad-locale calls then 5 valid registers still all 201, the 6th valid is 429, and a bad locale after that is still 400 invalid_locale (locale checked before the limiter, as dev claimed). JSON-escaped 'vi' (\u0076\u0069) is accepted as vi, correct.
  AC2 API log: locale vi -> 'Subject: Xac nhan dia chi email SMemories cua ban' (Vietnamese) with /verify-email?token= link; en/absent/null -> 'Confirm your SMemories email address'. Forgot-password for a vi user -> Vietnamese reset subject. Go test TestRegisterLocale asserts vi subject+link.
  AC3 real Chrome (headless, CDP, vite dev proxy to API): UI en via switcher -> POST /api/v1/auth/register body locale:en, /v1/me en; UI vi via switcher (h1 'Tao tai khoan') -> body locale:vi, /v1/me vi; browser Accept-Language vi-VN with no click -> locale vi; default browser en -> locale en. Vitest has en and vi request-body tests.
  AC4 openapi/schema/postman updated; npm run check:api ok. newman auth collection (Full flow + Edge cases) run twice back to back with an API restart between: 48 requests/116 assertions, 0 failed both times. Full collection incl. Google folder with dummy SMEM_GOOGLE_* config, twice with restart: 55 req/140 assertions, 0 failed. (Without Google env the Google folder fails by design, 404.)
  Mechanical: go build/vet, go test -race ./... , integration tests for internal/auth and internal/db (-tags integration, TestRegisterLocale x3), gofmt, web vitest 61/61, eslint+prettier, tsc, lint:i18n, check:api: all green.
  Google sign-in unaffected: no google/oidc file in the diff; all Google integration tests pass (incl. TestGoogleNameAndLocaleFallbacks). Real Google round trip not run (no credentials), covered by the fake-provider tests.
  Non-blocking: unknown extra JSON fields and a differently-cased key (LOCALE) are accepted by the decoder (pre-existing Go encoding/json behaviour, value still validated). Risk high: owner approves the merge.
- 2026-10-08 08:52Z · leader · Leader review OK on head 8a9cafb (QA_PASS on the same head, CI 4/4 green). A two-value allow-list (en, vi), anything else including non-strings is invalid_locale before any rate-limit token or hash is spent, nothing is created on error, the stored locale drives the verification and reset emails, the web form sends i18n.resolvedLanguage limited to en/vi. Non-blocking: unknown JSON fields and a differently-cased key are accepted by the shared decoder (pre-existing behaviour). Awaiting owner approval.

### T-046 — List a yearbook's photos (API and photo library in the web UI)
- **Status:** BACKLOG
- **Priority:** P1
- **Type:** feature
- **Milestone:** M1
- **Depends-on:** T-016
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** —
- **PR:** —
- **Updated:** 2026-10-08 09:14Z by leader
- **Comments-seen:** 0

**Spec — read this first, it is the source of truth:** `.team/epics/E03-yearbooks/05-list-a-yearbooks-photos.md`
(read it from the repo root, i.e. the main checkout, where the leader keeps it current; not from a task worktree).
**Epic:** E03-yearbooks · **PRD:** `.team/epics/E03-yearbooks/PRD.md`

Owner-only list endpoint for a yearbook's photos and the web photo library that uses it, so uploaded photos survive a reload.

#### Comments

<!-- tasks:end -->

## 7. Change log

- 2026-10-06 — Board initialised by `/team-init`. Owner decisions D-01…D-09 recorded; M0 and M1 specified; M2/M3 sketched in BACKLOG.
- 2026-10-07 — First dev run: T-001 and T-005 reached READY_FOR_QA (PRs #1, #2); T-005 verdict GO `codeberg.org/go-pdf/fpdf`. Q-001 and Q-002 answered `A` on the board, recorded as D-10 and D-11. T-001's READY_FOR_QA update had been lost from the board and was restored from the event log.
- 2026-10-07 — T-001 merged to develop (c4c25a8): module path, httpx router/middleware, config, /healthz, OpenAPI + Postman starters. QA found T-005 (PDF spike) needs one fix (fpdf cover image not full-bleed); sent back. Review follow-ups collected in T-028, which now blocks T-004 and T-012.
- 2026-10-07 — T-002 merged to develop (61dba35): docker-compose MySQL 8.4 + MinIO, goose migrations, /readyz, dbtest harness. T-005 (PDF spike, GO codeberg.org/go-pdf/fpdf) passed QA and my review but conflicts with T-002 on go.mod/go.sum/Makefile; sent back for a conflict-only merge. QA minor findings collected in T-030; frozen MinIO image tracked in T-029 (L-08).
- 2026-10-07 — T-005 merged to develop (b1eaffa): ADR 0002, verdict GO codeberg.org/go-pdf/fpdf, recorded as D-12; ADR rules made binding for T-010. QA passed T-003 (web scaffold, PR #4).
- 2026-10-07 — T-028 merged (a0d9429): access log uses the route pattern, make lint scope fixed. T-006 (auth core, PR #6) sent back before QA to add Unicode normalisation (L-09) and merge develop. T-003 (web scaffold) waits for a conflict-only merge.
- 2026-10-07 — T-010 (PDF renderer, PR #16) passed QA and my review and awaits owner approval; T-008 (yearbook API, PR #17) is in QA; T-007 starts next. L-11 records that sqlc is not used. QA and dev both worked within the usage gate; the freshness guard left T-007 unclaimed once more.
- 2026-10-07 — Owner answers in chat: T-010 approved and merged (357e78a); Q-005 decided (D-13, no CAPTCHA); all merged tasks T-001..T-006, T-008, T-028, T-030 accepted (DONE). Branch protection and replacing the old shared dev stack were authorised and follow.
- 2026-10-07 — Branch protection enabled on develop and main (D-14).
- 2026-10-07 — Owner approved the migration: all 35 task specs moved out of the README into epic files (`.team/epics/E01..E07`, one PRD per epic); board blocks keep status, dependencies, comments and a stub that links the spec. README 200 KB -> 156 KB. Skills updated earlier to write new work this way.
- 2026-10-08 — T-010 accepted (DONE). T-007 reviewed, owner-approved and merged (186ac52). T-009 passed QA and review; awaiting owner approval, merges after T-007 (migration 0006). Follow-up T-036 (memory bound of image processing) created and made a prerequisite of T-034.
- 2026-10-08 — T-009 (photo upload, d46aec5), T-011 (Google sign-in, d209266) and T-012 (collection links) merged, each owner-approved after QA and leader review; T-007 and T-009 and T-011 await owner acceptance. T-010 accepted. Promoted T-015, T-033, T-035 to TODO; T-036 (memory bound) is queued ahead of T-034.
- 2026-10-08 — Owner asked for a Canva-style editor. Decisions D-19 (staged editing), D-20 (each owner edits their own copy), D-21 (template-driven note fields, now). New epic E09 (sketch), tasks T-043 (field catalogue, P1, before T-034) and T-044 (templates declare fields); T-034's spec was rewritten for answers by field id. Epic E08 (designer templates, D-15..D-18, L-12) created earlier today with T-037..T-042.
