#!/usr/bin/env python3
"""One call per tick: refresh the usage cache, run the gate, print ONE line.

    gate.py FIVE_PCT FIVE_RESETS_ISO SEVEN_PCT SEVEN_RESETS_ISO     (numbers from get_usage, `-` for a window with no resetsAt at 0%)

GO    -> `GO 5h 20% 7d 3%`                                         exit 0
SLEEP -> `SLEEP <reason> | resume 13:03Z in 190 min (20:03 local)`  exit 10  (resume = when every blocking window has reset, +3 min)
Bad input or no plugin -> message, exit 2 (fails closed: nothing is written, the gate is not run).
It does exactly what CLAUDE.md's two steps did (refresh_usage.py, then usage_gate.py --wait-fresh); thresholds and
usage.on_unknown are not touched.
"""
import datetime as dt
import glob
import json
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
PLUGIN = os.path.expanduser("~/.claude/plugins/cache/claude-agent-team/agent-team/*/scripts/usage_gate.py")


def newest(pattern):
    found = glob.glob(pattern)
    if not found:
        sys.exit("agent-team plugin not found under ~/.claude/plugins/cache")
    return max(found, key=lambda p: tuple(int(x) for x in p.split("/")[-3].split(".")))


def limits():
    try:
        cfg = json.load(open(os.path.join(HERE, "config.json"))).get("limits", {})
    except (OSError, ValueError):
        cfg = {}
    return float(cfg.get("session_max_pct", 60)), float(cfg.get("weekly_max_pct", 75))


def reset_ts(iso):
    return None if iso == "-" else dt.datetime.fromisoformat(iso.replace("Z", "+00:00")).timestamp()


def main(argv):
    if len(argv) != 4:
        sys.exit(__doc__.split("\n\n")[1].strip())
    r = subprocess.run([sys.executable, os.path.join(HERE, "refresh_usage.py"), *argv], capture_output=True, text=True)
    if r.returncode != 0:
        print("REFUSED " + (r.stderr or r.stdout).strip().splitlines()[-1])
        sys.exit(2)
    g = subprocess.run([sys.executable, newest(PLUGIN), "--wait-fresh"], capture_output=True, text=True)
    first = (g.stdout.strip().splitlines() or ["SLEEP reason=no output from gate"])[0]
    if first.startswith("GO"):
        print("GO 5h %s%% 7d %s%%" % (argv[0], argv[2]))
        return 0
    reason = first.split("reason=", 1)[-1].split(" window_resets_in", 1)[0]
    s_max, w_max = limits()
    blocking = []
    for pct, iso, mx in ((float(argv[0]), argv[1], s_max), (float(argv[2]), argv[3], w_max)):
        if pct >= mx:
            blocking.append(reset_ts(iso))
    if blocking and all(b is not None for b in blocking):
        resume = dt.datetime.fromtimestamp(max(blocking) + 180, dt.timezone.utc)
        mins = max(0, int((resume - dt.datetime.now(dt.timezone.utc)).total_seconds() // 60))
        local = resume.astimezone()
        print("SLEEP %s | resume %sZ in %d min (%s local, cron \"%d %d %d %d *\")" % (
            reason, resume.strftime("%H:%M"), mins, local.strftime("%H:%M"), local.minute, local.hour, local.day, local.month))
    else:
        print("SLEEP " + reason)
    return 10


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
