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
| `make up` / `make down` | Start (and wait for) / stop MySQL 8.4 and MinIO; creates `.env` from `.env.example` first. Ports bind to `127.0.0.1`. `make up` also (re)applies the `smem_test_%` grants, so it works on older MySQL volumes |
| `make migrate` / `make migrate-down` | Apply all migrations / roll back the last one |
| `make test-integration` | Tests tagged `integration` against the local MySQL (run `make up migrate` first; each test gets its own throwaway database) |

Run the API locally:

```sh
make up migrate               # creates .env, starts MySQL + MinIO, applies migrations
make run                      # make loads .env for you
curl -i localhost:8080/healthz   # {"status":"ok"}
curl -i localhost:8080/readyz    # {"status":"ready"}; 503 not_ready while MySQL is down
```

Several checkouts on one machine: `make up` names the compose project after the checkout directory
(`COMPOSE_PROJECT_NAME`, lower-cased; set it in your shell or `.env` to choose another name), so each checkout has its own
containers and volumes and `make down` in one leaves the others running. The host ports are fixed per machine, so give each
checkout free ones in its `.env`: `MYSQL_PORT`, `MINIO_PORT`, `MINIO_CONSOLE_PORT` (and match the port in `SMEM_DB_DSN` and
`SMEM_TEST_DB_DSN`). A checkout whose stack already runs under the old fixed name `smemories` keeps that name; to retire an
old stack run `COMPOSE_PROJECT_NAME=smemories docker compose down` once from the directory that started it.

MinIO console: <http://127.0.0.1:9001> (credentials in `.env`). MinIO stopped publishing Docker images, so
the compose file uses a frozen Bitnami build; it is for local development only.

Configuration is read from environment variables (see `.env.example`): `SMEM_HTTP_ADDR` (default `:8080`),
`SMEM_ENV` (`dev|test|prod`, default `dev`), `SMEM_LOG_LEVEL` (`debug|info|warn|error`, default `info`),
`SMEM_DB_DSN` (`user:pass@tcp(host:port)/db`; required, in every `SMEM_ENV`, for the API binary, which exits with a clear message when it is empty), and the pool settings
`SMEM_DB_MAX_OPEN` (20), `SMEM_DB_MAX_IDLE` (5), `SMEM_DB_CONN_MAX_LIFETIME` (5m).
Auth (see `.env.example`): `SMEM_ALLOWED_ORIGINS` (comma-separated browser origins allowed to send cookie-carrying
state-changing requests; required in prod, default `http://localhost:5173`), `SMEM_TRUST_PROXY` (take the client IP from the
last `X-Forwarded-For` hop; only behind a trusted proxy), `SMEM_AUTH_MAX_CONCURRENT_HASHES` (4), `SMEM_AUTH_ARGON_MEMORY_KIB`
(19456), `SMEM_AUTH_ARGON_TIME` (2), `SMEM_AUTH_ARGON_PARALLELISM` (1), and the rate limits `SMEM_RATE_REGISTER_PER_HOUR` (5),
`SMEM_RATE_LOGIN_FAILS_PER_EMAIL` (10) and `SMEM_RATE_LOGIN_FAILS_PER_IP` (100). The session cookie is `Secure` unless `SMEM_ENV=dev`.
An invalid value stops the process with a message naming the variable. The API contract is
`api/openapi.yaml`; the Postman collections live in `postman/` (`newman run postman/platform.postman_collection.json`
against a running API).

## Project management

Work is planned and tracked on the team board: [`.team/README.md`](.team/README.md) (vision, roadmap, decisions, tasks).
