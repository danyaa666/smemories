#!/usr/bin/env python3
"""One call replaces the leader's orient step (about 6 board/git/gh calls and their long outputs).

    orient.py            -> compact snapshot, about 25 lines, never the full comment text

Prints: board summary (without the empty lines), unread comments (latest per task, one line, 170 chars max),
open questions, what dev and qa would pick, and one line per open PR (head, mergeable, CI). Read-only.
Full text of anything: `board.py --as leader get T-xxx`.
"""
import json
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
BOARD = os.path.expanduser("~/.claude/plugins/cache/claude-agent-team/agent-team/")


def board_py():
    import glob
    found = glob.glob(BOARD + "*/scripts/board.py")
    if not found:
        sys.exit("agent-team plugin not found")
    return max(found, key=lambda p: tuple(int(x) for x in p.split("/")[-3].split(".")))


def run(*cmd):
    r = subprocess.run(cmd, capture_output=True, text=True, cwd=os.path.dirname(HERE))
    return r.stdout.strip()


def board(*args):
    return run(sys.executable, board_py(), "--as", "leader", *args)


def main():
    subprocess.run(["git", "fetch", "origin", "--prune", "-q"], cwd=os.path.dirname(HERE))
    keep = ("TASKS", "DEV_QUEUE", "QA_QUEUE", "LEADER_REVIEW", "HUMAN_REVIEW", "BLOCKED", "PIPELINE", "PARKED",
            "OWNER_APPROVAL", "UNREAD_COMMENTS", "QUESTIONS", "STALE")
    for line in board("summary").splitlines():
        if line.startswith("LEASE") and "none" not in line:
            print(line)
        elif line.startswith(keep) and " none" not in line and not line.endswith("=0"):
            print(line)
    unread = board("unread").splitlines()
    task, last = None, {}
    for line in unread:
        if line.startswith("T-"):
            task = line.split(" — ")[0]
        elif line.startswith("- ") and task:
            last[task] = line  # keeps the newest comment of each task
    for t, c in last.items():
        parts = c[2:].split(" · ", 2)
        who = parts[1] if len(parts) > 2 else "?"
        text = (parts[2] if len(parts) > 2 else c).replace("\n", " ")
        print("UNREAD %s (%s): %s" % (t, who, text[:170] + ("..." if len(text) > 170 else "")))
    q = board("questions")
    if q and "no open" not in q:
        print("QUESTIONS", q[:400])
    for who in ("dev", "qa"):
        picks = [l for l in board("next", "--for", who).splitlines() if l.startswith(("PICK", "REWORK"))]
        for l in picks:
            print("%s %s" % (who.upper(), l[:150]))
    r = run("gh", "pr", "list", "--state", "open", "--json", "number,title,headRefOid,mergeable,statusCheckRollup")
    try:
        for pr in json.loads(r or "[]"):
            ci = [c.get("conclusion") or c.get("status") or "?" for c in pr.get("statusCheckRollup", [])]
            bad = [x for x in ci if x not in ("SUCCESS", "SKIPPED", "NEUTRAL")]
            print("PR #%s %s head=%s %s CI=%s" % (pr["number"], pr["title"][:50], pr["headRefOid"][:7],
                  pr["mergeable"], "green" if ci and not bad else ("none" if not ci else ",".join(bad))))
    except ValueError:
        print("PR list unavailable")
    st = run("git", "status", "-sb").splitlines()[0]
    print("GIT", st)


if __name__ == "__main__":
    main()
