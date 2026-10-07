# T-003 — Web scaffold: Vite + React + TypeScript + EN/VI i18n

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M0 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Description
Create the web app skeleton: Vite + React + TypeScript with English/Vietnamese i18n from the first screen, a typed API client generated from `api/openapi.yaml`, tests, and make targets. Decisions: D-02, D-05, L-02, L-03, L-07.

#### Scope
- In: `web/` app; i18n with `en.json`/`vi.json`; language switcher; i18n parity check; typed client + error mapping; home page with API status badge; not-found page; dev proxy; make targets.
- Out (do not do): auth pages, any product screen, state management beyond TanStack Query setup, CSS framework (plain CSS modules or one small global stylesheet), web fonts (use the system font stack, which covers Vietnamese), CI (T-004).

#### Acceptance criteria
- [ ] AC1 — `web/` is a Vite + React + TypeScript app (`strict`, `noUncheckedIndexedAccess`). `npm ci && npm run build` passes from a clean clone. Node version pinned in `web/.nvmrc` (22 LTS) and `engines`.
- [ ] AC2 — `npm run lint` (eslint, zero warnings), `npm run typecheck` (`tsc --noEmit`), `npm test` (vitest + Testing Library) pass; Prettier config committed.
- [ ] AC3 — i18n via react-i18next with `web/src/locales/en.json` and `vi.json`. A language switcher (EN | VI) is an accessible button group using `aria-pressed`. The choice persists in `localStorage` (reads and writes wrapped in try/catch; the app works without storage), default from `navigator.language` (`vi*` → `vi`, else `en`), and `<html lang>` follows the active language.
- [ ] AC4 — `npm run lint:i18n` exits non-zero and names the key when a key exists in only one locale or a value is empty; a unit test proves it fails on a fixture pair and passes on the real files.
- [ ] AC5 — Home route `/` shows the app name, a translated tagline, the language switcher and an API status badge that calls `GET /api/healthz` through the typed client: "API: ok" / "API: unreachable" (translated). An unknown route renders a translated not-found page.
- [ ] AC6 — `npm run gen:api` generates `web/src/api/schema.d.ts` from `../api/openapi.yaml` with `openapi-typescript`; the file is committed and `npm run check:api` fails if it is stale. A thin fetch wrapper sends/receives JSON and maps the error envelope to a typed `ApiError { status, code, message, requestId }`.
- [ ] AC7 — Vite dev server proxies `/api/*` to `http://localhost:8080/*` (prefix stripped, see L-07).
- [ ] AC8 — Makefile gains `web-install`, `web-build`, `web-test`, `web-lint`; `make build`, `make test`, `make lint` now include the web steps.
- [ ] AC9 — Accessibility basics: `header`/`main` landmarks, visible focus ring, text contrast ≥ 4.5:1, language switcher usable by keyboard; covered by role-based Testing Library queries.

#### Design
Files: `web/package.json`, `web/vite.config.ts`, `web/tsconfig*.json`, `web/eslint.config.js`, `web/src/{main.tsx,App.tsx,i18n.ts}`, `web/src/locales/{en,vi}.json`, `web/src/api/{client.ts,schema.d.ts}`, `web/src/pages/{Home,NotFound}.tsx`, `web/scripts/check-i18n.mjs`, tests next to the code. Query client and router are created in `main.tsx` so later tasks only add routes.

#### Risk
`low`

#### Security & performance notes
Never render API-supplied strings as HTML. Do not put tokens in `localStorage` (none exist yet; keep it that way — sessions will be HttpOnly cookies). Keep the initial bundle small: no UI kit.

#### Test plan
- Dev: tests for switcher behaviour (click, persistence, storage throwing, `lang` attribute), parity script (pass and fail fixtures), API badge (200, network error, 500 envelope), not-found page.
- QA should probe: delete a key from `vi.json` and confirm `lint:i18n` fails; browser language `vi-VN` on first load; block `localStorage` in a private window; keyboard-only use of the switcher; API down state.
