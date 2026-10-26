# SMemories: team notes

Board and decisions: [.team/README.md](.team/README.md). Product description: [README.md](README.md).

## Usage gate in the Claude desktop app (owner-approved 2026-10-07, one-command form 2026-10-15)

The desktop app's session never runs the statusLine, so `~/.claude/team-usage.json` is not filled automatically
and `usage_gate.py` would sleep forever. **Leader only:** before the usage gate in every `/team-tick`
(Step 0), and again right before dispatching dev/qa:

1. Call the read-only `get_usage` tool (`mcp__ccd_session_mgmt__get_usage`).
2. If `plan.status` is `ok` and both the "5-hour limit" and "Weekly" windows are present, run **one** command with exactly the
   numbers that call returned (a window with no `resetsAt`, i.e. one that has just reset and not started again, gets `-`;
   accepted only together with 0% used):
   `python3 .team/gate.py <5h percentUsed> <5h resetsAt> <weekly percentUsed> <weekly resetsAt>`
   It does what the two old steps did (`refresh_usage.py`, then `usage_gate.py --wait-fresh`), validates first and fails closed,
   and prints one line: `GO ...` (exit 0) or `SLEEP <reason> | resume HH:MMZ in N min ...` (exit 10). Obey it literally.
   (The two separate scripts still work.)

Rules:
- If `get_usage` fails or is not `ok`, do **not** write the cache. The gate then sleeps (fails closed).
- Never invent, round up or reuse old numbers. Never change `limits` or `usage.on_unknown` in `.team/config.json`.
- The cache is stale after 5 minutes, so refresh right before dispatching; dev and qa only run `usage_gate.py` and never refresh.
- Not needed in a terminal session (`bin/team start`): there the statusLine in `.claude/settings.json` fills the cache.

## Token budget (owner request, 2026-10-15)

The heartbeat fires every 30 minutes and the leader's context is large, so every wake-up is expensive even when it does nothing.
**Leader only:**
1. **A sleeping tick is exactly 2 tool calls:** `get_usage`, then `python3 .team/gate.py ...`. On `SLEEP` reply with the one line and
   stop: no board read, no plan, no other tool.
2. **Sleep until the reset, not in 30-minute steps.** When the SLEEP line says `resume ... in N min` with N > 45, replace the
   recurring heartbeat by a one-shot: `CronDelete` the recurring job (id in `.team/state/heartbeat.json`), `CronCreate` with
   `recurring: false`, the `cron "..."` shown in the SLEEP line and prompt `/team-tick`, and record `mode: "one-shot"` and the new id in
   `heartbeat.json`. On the first tick that gets `GO`: `CronList`; if only the one-shot (or nothing) exists, create the recurring job
   (`7,37 * * * *`, prompt `/team-tick`) and record it. Never leave the session with neither a recurring nor a pending one-shot job,
   and tell the owner once that the heartbeat is in one-shot mode.
3. **An awake tick starts with `python3 .team/orient.py`** (about 25 lines: summary, newest unread comment per task cut to one line,
   questions, what dev and qa would pick, open PRs with head, mergeable and CI) instead of separate summary, unread, questions, next and
   `gh` calls. Read a task in full (`board.py get T-xxx`) only when it needs a decision, and cap long output (`| cut -c1-400`, `| tail -n 20`).
4. **Batch Bash:** acknowledge, regenerate the index, commit and push in one call. Commit with a path limit (`git commit -- .team docs`)
   because the owner may have unrelated staged files.
5. **Dispatch only what `orient.py` lists as PICK or REWORK**, after a fresh `get_usage` + `gate.py`. Do not spawn dev for a task whose
   PR conflicts with develop (send it back with a note), and every dispatch prompt ends with: "Final report to the leader: at most 8 lines
   (status, PR and head, what to verify, blockers); details go on the board."
6. **Never read agent output files or transcripts**; the board and the PR are the report. Do not re-read specs you wrote this session.
7. **Context:** when `get_usage` shows `context.percentUsed` >= 45, tell the owner once per day in one line: "leader context is N%:
   `/compact` would cut every tick's cost; all state is on the board".

## CI scope (owner decision D-37, 2026-10-26)

From T-132 on, CI runs `go`, `web`, `security` and `image` on every push and pull request. **`go-integration` and `e2e` (journey and print-check) are paused**:
they run only by hand (`gh workflow run ci.yml --ref <branch>`) and show as *skipped* on a PR. Because of that:
- **Dev** runs `make test-integration` for the packages it touched, and the e2e/print-check commands from `docs/e2e.md` when it changes the web book, print or auth flows, before it sets READY_FOR_QA.
- **QA** runs the full `make test-integration` and the e2e and print-check commands locally on the PR head, and writes the result lines (pass/fail counts, head SHA) in its board comment. A QA_PASS without them is not valid for risk-high tasks or anything touching permissions, migrations, concurrency or print.
- **Leader** does not merge a high-risk task whose QA comment lacks those lines, and may start the manual CI run (`gh workflow run`) on a branch it is unsure about.
- Where a spec says "CI green" for integration, race-integration, e2e or print-check, read it as "QA ran it locally and reported it" until the jobs are switched back on (delete the two `if:` lines in `ci.yml`).

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


## Engineering standards (owner rule, 2026-10-09)

The backend skills in `.agents/skills/` (`be-golang` with its `references/`, `be-api-design`, `be-rldb`, `be-architect`, `be-td`) are the team's standard, as adapted by
[docs/api-contract.md](docs/api-contract.md), [docs/db-conventions.md](docs/db-conventions.md) and [docs/go-conventions.md](docs/go-conventions.md) (decisions D-25..D-27).
- **Read the skills from the main checkout** (`.agents/skills/`, not a task worktree) and the three docs before any Go, SQL or API change; the docs list the few places where we deliberately differ (plain `database/sql`, stdlib mux, `internal/apperr` instead of `go-common`, ULID public ids).
- New endpoints are `/api/<namespace>/<action>` with the `{status,data}` envelope; new tables are `_tab` tables with BIGINT ms timestamps and no foreign keys; handlers never call stores; no `fmt.Errorf`/`errors.New` in application code. Existing code is converted by epic E10 (T-063..T-073); do not "fix" other domains in an unrelated PR.
- Reviewers (leader, QA) check the PR against those documents' checklists.

## Finding tasks by epic (owner request, 2026-10-09)

`board.py` only understands plain ids (`T-063`: board headings, `Depends-on`, branches `task/t-063-...`, worktrees), so the id itself cannot carry the epic.
Instead every board title starts with the epic code (`T-063 — [E10] Foundation: ...`, searchable with `grep "\[E10\]"` or `board.py list`), every spec's first line is
`# E10_T-063 — ...`, and [.team/TASKS.md](.team/TASKS.md) lists all tasks grouped by epic with labels like `E10_T-063`.
**Leader only:** new tasks get the `[E##]` prefix in `add-task --title`; before each board commit run `python3 .team/epic_index.py` (add `--titles` to prefix any task that lacks it).
