# Architecture: smemories (commit a2226aa, branch develop)

Facts below come from four read-only reconnaissance reports; hunters must re-read cited source before relying on a line reference.

## 1. Product, principals, protected resources
SMemories lets students and classes design, collect content for, and export yearbooks (print via browser `window.print()`; README.md:7). No admin or role concept exists.
- **Anonymous client:** register, login, forgot/reset password, Google OIDC start/callback, `POST /api/print-feedback/create`, `/healthz`, `/readyz`.
- **Link-token holder (friend, no account):** `GET /api/public-collect/get` and multipart `POST /api/public-collect/submit-note` (<=3 photos, 32 MiB cap). The bearer token travels in the query string (internal/notes/handler.go:66-68).
- **Registered, unverified user:** CRUD on own yearbooks and photos. **Verified owner:** note-collection links, moderation, book-data.
- **Operators:** `smemories-migrate`, `smemories-media-backfill` CLIs; optional unauthenticated `/metrics` listener.
Protected resources: accounts and sessions, owners' yearbooks/photos/notes, email-code and reset flows, per-owner storage quota, S3 objects, availability of the shared API.

## 2. Comparable baseline
No source-grounded comparable; omitted.

## 3. Stack and deployment
Go 1.26 API on stdlib `net/http` mux (cmd/smemories-api/main.go, internal/httpx/router.go), MySQL 8.4, Redis/Valkey (sessions, email codes, rate limits), S3/MinIO (photos), SES mailer, Google OIDC. React 19/Vite SPA (web/). Dockerfile builds three binaries into a distroless non-root image. Planned AWS ECS/ALB/S3/CloudFront; **no IaC, ALB, WAF, edge-header, task-role or bucket-policy source is in the repo** (docs/deploy.md; T-023 undone). `SMEM_ENV` defaults to `dev` (internal/config/config.go:116); dev selects a public OTP key constant, log mailer, non-Secure cookie, skips prod checks (internal/config/prod.go). Prod requires SES mail, a mandatory `SMEM_OTP_KEY`, refuses `SMEM_DEV_FIXED_OTP`. Registered dev shortcut `DEV-SHORTCUT(otp)` (`123123`, docs/dev-shortcuts.md; `.env.example` ships it; CI e2e sets it). `scripts/check-no-dev-shortcuts.sh` does not exist yet.
**Execution limits:** no Go module cache, no node_modules, no DB/MinIO, no packaged sandbox, no promotion helper. No target code runs in this audit. Never run `go`, `npm`, `make` or `docker`: `go` with GOTOOLCHAIN=auto downloads a toolchain.

## 4. Entry surfaces and key paths
- Middleware chain (internal/httpx/router.go:40-49): request id, access log, metrics, security headers, recover, timeout, 1 MiB body cap, client IP. `X-Forwarded-For` last hop used only if `SMEM_TRUST_PROXY` (internal/httpx/middleware.go:238-252). Rate-limit keys are the raw IP string (no IPv6 grouping).
- Auth (internal/auth/): Argon2id passwords, 6-digit email codes (HMAC in Redis, 5 tries), 30-day sliding sessions with no absolute cap, Google OIDC with signed `smem_oidc` cookie and `return_to` check (google.go:298-310), account linking strips an unverified local account's password/sessions (store.go:244-255).
- Yearbook/media/notes/bookdata/templateapi handlers: `/api/...` JSON endpoints guarded by `RequireUser` and CSRF `Guard`. Ids are checked canonical-ULID in notes batch endpoints but only by length elsewhere; the caller's id spelling reaches S3 keys and the yearbook-delete prefix (internal/media/service.go:86-150, internal/yearbook/service.go:128). `public_id` columns are case-insensitive.
- Uploads: owner path buffers up to `SMEM_MEDIA_MAX_BYTES` in memory (internal/media/handler.go:175-232); public submit spools to `os.CreateTemp` with pacing and slot limits (internal/notes/submit.go:57-247); both pass `media.process` (internal/media/image.go) for sniff, dimension limits, decode, re-encode to three variants.
- Stored text (note answers, display names) is rendered by React as text; email templates in internal/auth/emails; `textx.Clean`. No `dangerouslySetInnerHTML`; `i18n.ts:48` sets `escapeValue: false`.
- Cursor tokens: AES-GCM key derived from `SMEM_OTP_KEY` (httpx/cursor.go; main.go:157), the same secret that keys email codes.
- CI (.github/workflows/ci.yml): `pull_request`, `contents: read`, SHA-pinned actions; `.github/actions/go-newest-patch/action.yml` selects a toolchain from go.dev; Makefile does `include .env`.

## 5. Trust boundaries and strongest source-visible control
- Anonymous -> session: login limiter pair/IP fail-closed, Argon2id, session rotation on login (auth/service.go).
- Cookie session -> state change: Origin/Referer check **only when the session cookie is present** (auth/csrf.go:22-52); SameSite=Lax; no CORS.
- Owner scoping: every yearbook/media/notes query joins `yearbook_tab.owner_id`.
- Link token -> public form: 192-bit token stored as SHA-256; per-IP limiters; honeypot; CAPTCHA verifier is `allowAll` (notes/verifier.go:17).
- Limiter policy: fail-closed for login/code guessing, fail-open for register, forgot-password, public routes, uploads.
- Deployment facts source cannot establish: real `SMEM_ENV`, `SMEM_TRUST_PROXY` vs exposure, Redis/MySQL network and TLS, S3 bucket policy, edge CSP/HSTS, whether access logs capture `?token=`, GitHub branch protection.

## 6. Starting paths
cmd/, internal/{auth,notes,media,yearbook,httpx,config,storage,mailer,ratelimit,redis,db,bookdata,templateapi,printfeedback,textx,templates,pdf}, migrations/, web/src/, Dockerfile, docker-compose.yml, Makefile, scripts/, .github/, .env.example, docs/dev-shortcuts.md, api/openapi.yaml. `.team/` and `.agents/` are process material, not targets.

## 7. Prior coverage
No compatible prior `coverage-ledger.json` or `findings.json`; no carried confirmations or exclusions. A non-skill write-up (docs/security-audit-2026-10-20.md) exists and is not coverage evidence. Quick profile: partial coverage only.

## 8. Companion selection
Selected: WEB-PROTOCOL-AND-AUTH (HTTP API, sessions, CSRF, OIDC, recovery, forwarded headers), CLIENT-SIDE (React SPA, browser storage, navigation), DATA-ISOLATION-AND-LIFECYCLE (owner-scoped data, blobs, deletes, derived copies, logs), RESOURCE-EXHAUSTION-AND-AVAILABILITY (anonymous 32 MiB multipart, image decode, limiters), SUPPLY-CHAIN-AND-RELEASE (CI, Dockerfile, dependencies), CLOUD-AND-DEPLOYMENT (env-selected security posture, container, metrics listener, S3, secrets). Excluded: AI-AND-LLM (no model/tool surface), MEMORY-SAFETY-AND-BINARY (Go, CGO off, no `unsafe`), PROTOCOLS-RPC-AND-MESSAGING (no RPC, broker or webhook), DESKTOP-MOBILE-AND-LOCAL-IPC (none).
