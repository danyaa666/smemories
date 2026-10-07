#!/usr/bin/env python3
"""Desktop-app bridge: feed the usage gate from the app's real plan limits.

    refresh_usage.py FIVE_PCT FIVE_RESETS_ISO SEVEN_PCT SEVEN_RESETS_ISO

The Claude desktop app never runs the statusLine, so ~/.claude/team-usage.json stays
empty and usage_gate.py sleeps forever. The leader reads the numbers with the app's
read-only get_usage tool (never guessed) and passes them here; this script validates
them and writes the cache through the plugin's own statusline.py, so the format is
exactly what the gate expects. Bad input writes nothing, so the gate fails closed.
Thresholds and `usage.on_unknown` are NOT touched.
"""
import datetime as dt
import glob
import json
import os
import subprocess
import sys

PLUGIN = os.path.expanduser("~/.claude/plugins/cache/claude-agent-team/agent-team/*/scripts/statusline.py")


def window(pct, iso):
    p = float(pct)
    if not 0 <= p <= 100:
        raise ValueError("percent out of range: %r" % pct)
    if iso == "-":  # the app reports no reset time for a window that has not started: only valid at 0% used
        if p != 0:
            raise ValueError("no reset time given for a window at %r%% used" % pct)
        return {"used_percentage": p, "resets_at": None}
    return {"used_percentage": p, "resets_at": dt.datetime.fromisoformat(iso.replace("Z", "+00:00")).timestamp()}


def newest_statusline():
    found = glob.glob(PLUGIN)
    if not found:
        sys.exit("agent-team plugin not found under ~/.claude/plugins/cache")
    return max(found, key=lambda p: tuple(int(x) for x in p.split("/")[-3].split(".")))


def main(argv):
    if len(argv) != 4:
        sys.exit(__doc__.split("\n\n")[1].strip())
    try:
        data = {"rate_limits": {"five_hour": window(argv[0], argv[1]), "seven_day": window(argv[2], argv[3])}}
    except ValueError as exc:  # float() and fromisoformat() both raise ValueError
        sys.exit("refusing to write cache: %s" % exc)
    subprocess.run([sys.executable, newest_statusline()], input=json.dumps(data), text=True,
                   check=True, stdout=subprocess.DEVNULL)
    print("cache refreshed: 5h %s%%, 7d %s%%" % (argv[0], argv[2]))


if __name__ == "__main__":
    main(sys.argv[1:])
