# SMemories: team notes

Board and decisions: [.team/README.md](.team/README.md). Product description: [README.md](README.md).

## Usage gate in the Claude desktop app (owner-approved 2026-10-07)

The desktop app's session never runs the statusLine, so `~/.claude/team-usage.json` is not filled automatically
and `usage_gate.py` would sleep forever. **Leader only:** before the usage gate in every `/team-tick`
(Step 0), and again right before dispatching dev/qa:

1. Call the read-only `get_usage` tool (`mcp__ccd_session_mgmt__get_usage`).
2. If `plan.status` is `ok` and both the "5-hour limit" and "Weekly" windows are present, run
   `python3 .team/refresh_usage.py <5h percentUsed> <5h resetsAt> <weekly percentUsed> <weekly resetsAt>`
   using exactly the numbers that call returned. If a window has no `resetsAt` (a window that has just reset and not
   started again), pass `-` for it; the helper accepts that only together with 0% used.
3. Then run the normal gate: `usage_gate.py --wait-fresh`. Obey it literally.

Rules:
- If `get_usage` fails or is not `ok`, do **not** write the cache. The gate then sleeps (fails closed).
- Never invent, round up or reuse old numbers. Never change `limits` or `usage.on_unknown` in `.team/config.json`.
- The cache is stale after 5 minutes, so refresh right before dispatching; dev and qa only run `usage_gate.py` and never refresh.
- Not needed in a terminal session (`bin/team start`): there the statusLine in `.claude/settings.json` fills the cache.

## Local stack (since 2026-10-07)

The dev MySQL and MinIO stack runs as the compose project `awesomeproject1`, started with `make up` from the main checkout
(`make migrate` applies the schema). The old shared `smemories` project was retired; its data volumes `smemories_mysql-data` and
`smemories_minio-data` were kept and are unused. The `smemories` account has the `smem_test_%` grant (`make up` re-applies it), so
`make test-integration` works with the default DSN: **no root-account workaround is needed any more**. Every compose experiment still
uses its own project name and ports; never run `docker compose down -v` against the main checkout's project.

## Where specs live

Each epic has a folder `.team/epics/E##-slug/` with a `PRD.md` and one spec file per task. A task's board block (`board.py get T-xxx`) is a short
stub that links its spec: **read the spec from the repo root (the main checkout) before you start**, and again if a comment says it was updated.
Specs and PRDs are leader-owned: never change anything under `.team/` in a pull request.

## Dev-only shortcuts (owner rule, 2026-10-08)

A dev-only shortcut is code that exists only to make development and testing easier and would be a backdoor in production. The first one is the fixed email code `123123` (`SMEM_DEV_FIXED_OTP`, T-048).
Rules for every agent:
- A shortcut must be switched on by an environment variable that is unset by default, honoured only when `SMEM_ENV` is `dev` or `test`, and the API must **refuse to start** if the variable is set in any other environment.
- Tag every line of it with the comment `DEV-SHORTCUT(<name>)` and register it in `docs/dev-shortcuts.md` (what, where, guard, how to remove). Never ship a shortcut that is not registered.
- **Delete all shortcuts before going to production.** T-050 does it and adds `scripts/check-no-dev-shortcuts.sh` to CI and to the deploy pipeline; it is a prerequisite of the first deploy (T-023). The leader checks `docs/dev-shortcuts.md` at every go-live review, and nobody sets a shortcut variable in a production configuration or in infrastructure code.
- Never log, print or commit a real code, key or token, and never reuse the dev code value for anything else.

