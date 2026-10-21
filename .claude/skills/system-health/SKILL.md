---
name: system-health
description: Check this machine's CPU, memory, disk and process health and report a short status. Use when the user asks for a system health or resource check, or when this runs on a recurring /loop.
model: haiku
allowed-tools: Bash(bash .claude/skills/system-health/check.sh*)
---

# System health check

Read-only check of the host this session runs on. The script does all the measuring and the threshold logic. Your job is to run it and relay the result briefly.

## Steps

1. Run exactly one command from the project root: `bash .claude/skills/system-health/check.sh`
   It prints `key=value` lines, then `STATUS=OK|WARN|CRIT`, optional `PROBLEM:` and `NOTE:` lines, and a final `SUMMARY:` line. The exit code is 0 for OK, 1 for WARN, 2 for CRIT; a non-zero exit is a result, not a failure.
2. Reply using the format below. Use only numbers the script printed. Do not estimate or round up.

## Reply format

- **OK:** reply with the `SUMMARY:` line and nothing else. Add the `NOTE:` text after it if there is one (for example a recent restart).
- **WARN or CRIT:** start with the `SUMMARY:` line. Then list each `PROBLEM:` line as a bullet. Name the top memory process from `top_mem_proc`, `top_mem_pid` and `top_mem_rss_mb`. End with one suggested read-only next step, such as `ps -eo pid,pcpu,pmem,rss,comm --sort=-pcpu | head` for CPU or `du -xh --max-depth=2 / | sort -rh | head` for disk.
- **On a recurring loop:** if the previous tick in this conversation had the same status and the same problems, reply `unchanged since last check` plus the `SUMMARY:` line instead of repeating the bullets. Always say so when the status changes, in either direction.
- If the script cannot run or prints no `STATUS=` line, say so in one line with the error text. Do not guess a status.

## Rules

- Read-only. Never kill a process, delete or clean files, restart anything, change settings, install packages or use the network, even when the status is CRIT. Suggest actions; do not take them.
- Run only the command above. Do not add other checks, and do not write any file.
- Thresholds live at the top of `check.sh` and can be overridden with `HEALTH_*` environment variables. Do not change them from inside this skill.
- Keep replies short. This skill is meant to run every few minutes.
