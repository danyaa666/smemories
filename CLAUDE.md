# SMemories: team notes

Board and decisions: [.team/README.md](.team/README.md). Product description: [README.md](README.md).

## Usage gate in the Claude desktop app (owner-approved 2026-10-07)

The desktop app's session never runs the statusLine, so `~/.claude/team-usage.json` is not filled automatically
and `usage_gate.py` would sleep forever. **Leader only:** before the usage gate in every `/team-tick`
(Step 0), and again right before dispatching dev/qa:

1. Call the read-only `get_usage` tool (`mcp__ccd_session_mgmt__get_usage`).
2. If `plan.status` is `ok` and both the "5-hour limit" and "Weekly" windows are present, run
   `python3 .team/refresh_usage.py <5h percentUsed> <5h resetsAt> <weekly percentUsed> <weekly resetsAt>`
   using exactly the numbers that call returned.
3. Then run the normal gate: `usage_gate.py --wait-fresh`. Obey it literally.

Rules:
- If `get_usage` fails or is not `ok`, do **not** write the cache. The gate then sleeps (fails closed).
- Never invent, round up or reuse old numbers. Never change `limits` or `usage.on_unknown` in `.team/config.json`.
- The cache is stale after 5 minutes, so refresh right before dispatching; dev and qa only run `usage_gate.py` and never refresh.
- Not needed in a terminal session (`bin/team start`): there the statusLine in `.claude/settings.json` fills the cache.
