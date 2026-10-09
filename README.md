# SMemories

**Keep the memories, in a book you can hold.**

## Overview

SMemories is a web application that lets students and classes design, collect content for, and export their own yearbooks. A whole class can build one together. A student can also make a personal yearbook alone. Friends can leave messages and photos through a simple shareable form, so nobody has to chase screenshots across chat groups. When the book is ready, SMemories exports a print-ready PDF.

## The problem

- Class yearbooks are usually made by one overworked student or teacher, who collects photos and messages by hand.
- Content arrives scattered across chat apps, forms, and email.
- Most design tools are too complex for students, and professional yearbook services are too expensive.
- Students who want a personal keepsake have no simple option.

## Target users

| Role | What they do |
|---|---|
| **Class admin** (teacher or class monitor) | Creates the class yearbook, invites students, picks the template, and approves the final book. |
| **Student** | Fills in their own profile page, collects notes from friends, and can make a personal yearbook. |
| **Contributor** (friend, teacher, or family member) | Leaves a message and photos through a link. Needs no account. |

## Core features

### 1. Two ways to create a yearbook
- **Class yearbook:** the admin creates a class space and invites students. Each student contributes their own page. The system compiles all pages into one book.
- **Personal yearbook:** a student builds their own book without a class, with the same tools.

### 2. Friends' notes collection
- The owner generates a **shareable link** to a form, like a Google Form.
- Contributors submit a **message, one or more photos**, their name, and an optional nickname or relationship, with no sign-up.
- The owner can **review, approve, hide, or reorder** submissions before they go into the book.
- The owner can set a deadline and send reminders.
- Approved notes can be placed on a student's page, or in a shared "Messages" section.

### 3. Templates and layout
- A library of ready-made **templates** (themes, cover styles, page layouts, fonts, colour palettes).
- Templates cover common page types: cover, class introduction, teacher messages, student profiles, group photos, event timeline, "class superlatives", and a notes/autograph page.
- Users can customize colours, fonts, and photo placement, and can add, remove, or reorder pages.

### 4. Yearbook information
Structured fields for the content that goes into the book:
- Class: name, school, year, motto, class photo, teachers.
- Student: name, photo, nickname, birthday (optional), quote, hobbies, future plans.
- Extras: events, memorable moments, awards, and fun polls.

### 5. Preview and PDF export
- Live **page-by-page preview** while editing.
- **Print-to-PDF** export at print quality (A4 or A5 and print-safe margins, with bleed as a later option).
- Download the PDF to share digitally or take it to any print shop.

### 6. Sharing and privacy
- Books are **private by default**. Sharing happens through invite or link only.
- The owner controls who can view, edit, or contribute.
- Contributors cannot see each other's notes unless the owner allows it.

## Typical flow

1. The admin or student creates a yearbook and picks a template.
2. They fill in the class or student information.
3. They send the notes form link to friends, teachers, or classmates.
4. Contributions arrive. The owner reviews and approves them.
5. The owner arranges the pages, previews the book, and exports it as PDF.

## Out of scope for v1 (possible later)

- Ordering physical printed books through the app
- Video or audio messages (for example via QR codes in the book)
- AI-assisted layout or photo selection
- Mobile apps

## Decisions

| Topic | Decision |
|---|---|
| Audience | University and college students (18+) first. School-age (under 18) support is a later milestone. |
| Accounts | Owners and students sign in with email + password or Google. Contributors never need an account. |
| Moderation | Owners approve or hide every note before it can be printed. |
| Languages | English and Vietnamese from the first screen. |
| First release | Personal yearbook, end to end. Class yearbooks come after go-live. |
| Print scope | PDF export only. Print it anywhere. |

## Tech stack

- **API:** Go (standard library HTTP), MySQL 8.4 (`utf8mb4`), S3-compatible object storage for photos.
- **Web:** Vite, React, TypeScript, react-i18next.
- **PDF:** pure-Go PDF library, declarative JSON templates, preview shown with pdf.js.
- **Hosting:** AWS (Fargate, RDS MySQL, S3, CloudFront), planned for milestone M2.
- **Local development:** Docker Compose runs MySQL and MinIO; no cloud credentials are needed.

## Development

Prerequisites: Go 1.26 (the `go` directive in `go.mod`), `make`, Docker with Compose, and golangci-lint v2 (2.8.0, the version CI pins; `make lint` runs it with `.golangci.yml`, which also covers the integration-tagged files; CI runs the same, see [docs/ci.md](docs/ci.md)).

| Command | What it does |
|---|---|
| `make build` | Builds the API to `bin/smemories-api` and the web app to `web/dist` |
| `make test` | Runs the Go tests with the race detector, then the web tests (vitest) |
| `make lint` | Fails (listing the files) if a tracked or new Go file is not gofmt-clean, runs `go vet` (with and without the `integration` tag), then the web checks (eslint, prettier, `tsc`, i18n key parity, stale API types) |
| `make web-install` | `npm ci` in `web/` (Node 22, see `web/.nvmrc`); the other web targets do it on demand. Dev server: `cd web && npm run dev` (proxies `/api/*` to `localhost:8080`) |
| `make run` | Runs the API from source (needs the stack below: `SMEM_DB_DSN` is required) |
| `make up` / `make down` | Start (and wait for) / stop MySQL 8.4, Valkey 8 (Redis protocol) and MinIO; creates `.env` from `.env.example` first. Ports bind to `127.0.0.1`. `make up` also (re)applies the `smem_test_%` grants, so it works on older MySQL volumes |
| `make migrate` / `make migrate-down` | Apply all migrations / roll back the last one |
| `make test-integration` | Tests tagged `integration` against the local MySQL and Valkey (run `make up migrate` first; each test gets its own throwaway database) |

Run the API locally:

```sh
make up migrate               # creates .env, starts MySQL + MinIO, applies migrations
make run                      # make loads .env for you
curl -i localhost:8080/healthz   # {"status":"ok"}
curl -i localhost:8080/readyz    # {"status":"ready"}; 503 not_ready while MySQL or Redis is down
```

Several checkouts on one machine: `make up` names the compose project after the checkout directory
(`COMPOSE_PROJECT_NAME`, lower-cased; set it in your shell or `.env` to choose another name), so each checkout has its own
containers and volumes and `make down` in one leaves the others running. The host ports are fixed per machine, so give each
checkout free ones in its `.env`: `MYSQL_PORT`, `REDIS_PORT`, `MINIO_PORT`, `MINIO_CONSOLE_PORT` (and match the port in `SMEM_DB_DSN`,
`SMEM_TEST_DB_DSN`, `SMEM_REDIS_URL` and `SMEM_TEST_REDIS_URL`). A checkout whose stack already runs under the old fixed name `smemories` keeps that name; to retire an
old stack run `COMPOSE_PROJECT_NAME=smemories docker compose down` once from the directory that started it.

MinIO console: <http://127.0.0.1:9001> (credentials in `.env`). MinIO stopped publishing Docker images, so
the compose file uses a frozen Bitnami build; it is for local development only.

Configuration is read from environment variables (see `.env.example`): `SMEM_HTTP_ADDR` (default `:8080`),
`SMEM_ENV` (`dev|test|prod`, default `dev`), `SMEM_LOG_LEVEL` (`debug|info|warn|error`, default `info`),
`SMEM_DB_DSN` (`user:pass@tcp(host:port)/db`; required, in every `SMEM_ENV`, for the API binary, which exits with a clear message when it is empty), and the pool settings
`SMEM_DB_MAX_OPEN` (20), `SMEM_DB_MAX_IDLE` (5), `SMEM_DB_CONN_MAX_LIFETIME` (5m).
Request deadline: `SMEM_HTTP_REQUEST_TIMEOUT` (30s, minimum 1s) cancels the context of every request unless a route sets its own.
Redis (`docs/redis.md`): `SMEM_REDIS_URL` (`redis://[:password@]host:port/db`; required in every `SMEM_ENV`, copy it from `.env.example` into an older `.env`), `SMEM_REDIS_PASSWORD`, `SMEM_REDIS_DIAL_TIMEOUT` (2s), `SMEM_REDIS_READ_TIMEOUT` and `SMEM_REDIS_WRITE_TIMEOUT` (1s), `SMEM_REDIS_POOL_SIZE` (10).
Auth (see `.env.example`): `SMEM_ALLOWED_ORIGINS` (comma-separated browser origins allowed to send cookie-carrying
state-changing requests; required in prod, default `http://localhost:5173`), `SMEM_PUBLIC_BASE_URL` (web app URL for emailed links; required in prod, default `http://localhost:5173`; in dev/test emails are printed to stdout by the log mailer, and prod refuses to start until a real mailer exists), `SMEM_TRUST_PROXY` (take the client IP from the
last `X-Forwarded-For` hop; only behind a trusted proxy), `SMEM_AUTH_MAX_CONCURRENT_HASHES` (4), `SMEM_AUTH_ARGON_MEMORY_KIB`
(19456), `SMEM_AUTH_ARGON_TIME` (2), `SMEM_AUTH_ARGON_PARALLELISM` (1), and the rate limits `SMEM_RATE_REGISTER_PER_HOUR` (5),
`SMEM_RATE_LOGIN_FAILS_PER_EMAIL` (10) and `SMEM_RATE_LOGIN_FAILS_PER_IP` (100). The session cookie is `Secure` unless `SMEM_ENV=dev`.
Photos (see `.env.example`): `SMEM_S3_ENDPOINT` (empty for AWS S3, `http://127.0.0.1:9000` for the local MinIO), `SMEM_S3_REGION`,
`SMEM_S3_BUCKET` (required in prod, default `smemories-dev`), `SMEM_S3_ACCESS_KEY`, `SMEM_S3_SECRET_KEY`, `SMEM_S3_PATH_STYLE` (true
for MinIO), `SMEM_MEDIA_MAX_BYTES` (10 MiB upload cap) and `SMEM_MEDIA_MAX_CONCURRENT` (2 images processed at once; memory budget in `docs/media.md`) and `SMEM_MEMORY_LIMIT_MIB` (soft Go memory limit, 0 = unset; about 75% of the container memory) and the public note upload settings `SMEM_PUBLIC_UPLOAD_MAX_CONNS` (48; submissions in progress at once, each spooled to disk while it arrives), `SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP` (8; of which one client IP may have) and `SMEM_UPLOAD_TMP_DIR` (spool directory, default the OS temp dir; worst case 48 x 32 MiB of disk, see `docs/media.md`).
An invalid value stops the process with a message naming the variable. The API contract is
`api/openapi.yaml`; the Postman collections live in `postman/` (`newman run postman/platform.postman_collection.json`
against a running API; the media collection uploads files, so run it from `postman/` with `--working-dir .`; the notes collection needs verified test accounts: see its description and `make verify-newman-users`).

### Google sign-in setup

Google sign-in is optional: with `SMEM_GOOGLE_CLIENT_ID` unset, `GET /v1/auth/google/start` and `/callback` answer `404 not_found`
and everything else works as usual. To turn it on:

1. In the [Google Cloud Console](https://console.cloud.google.com/apis/credentials) create a project (or pick one), configure the
   OAuth consent screen (scopes `openid`, `email`, `profile`), then **Create credentials > OAuth client ID > Web application**.
2. Add the authorised redirect URI `<public base URL>/api/v1/auth/google/callback`: for local development
   `http://localhost:5173/api/v1/auth/google/callback` (the Vite dev server proxies `/api` to the API), in production
   `https://<your domain>/api/v1/auth/google/callback`. It must match exactly.
3. Set these variables (see `.env.example`); the API refuses to start if one is missing:

   | Variable | Value |
   |---|---|
   | `SMEM_GOOGLE_CLIENT_ID` | the client ID from step 1 (setting it turns the feature on) |
   | `SMEM_GOOGLE_CLIENT_SECRET` | the client secret; keep it out of git |
   | `SMEM_PUBLIC_BASE_URL` | where the web app lives, e.g. `http://localhost:5173`; the redirect URI is built from it |
   | `SMEM_OIDC_COOKIE_KEY` | at least 32 random bytes that sign the short-lived `smem_oidc` cookie, e.g. `openssl rand -base64 48` |
   | `SMEM_GOOGLE_ISSUER` | optional, default `https://accounts.google.com` (tests point it at a fake provider) |

The web app starts the flow by navigating the browser to `/api/v1/auth/google/start?return_to=/path`. Failures come back as
`/login?error=<code>` with one of `oidc_state`, `oidc_denied`, `oidc_failed`, `email_unverified`. A Google account whose email matches
a local account is linked to it; if that local account never verified its email it loses its password and sessions first
(pre-hijacking defence). Social-only accounts have no password; "Forgot password" sets one.

## Engineering standards

Code follows the backend skills in `.agents/skills/` as adapted for this project: the API contract ([docs/api-contract.md](docs/api-contract.md)), database conventions
([docs/db-conventions.md](docs/db-conventions.md)) and Go conventions ([docs/go-conventions.md](docs/go-conventions.md)). Existing code is being converted (epic E10 on the board);
until a domain is converted its endpoints still live under `/v1`.

## Project management

Work is planned and tracked on the team board: [`.team/README.md`](.team/README.md) (vision, roadmap, decisions, tasks).
