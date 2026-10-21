#!/usr/bin/env bash
# system-health/check.sh: read-only host health check.
# No writes, no network, nothing is killed or cleaned. Prints key=value lines, then one SUMMARY line.
# Exit code: 0 = OK, 1 = WARN, 2 = CRIT. Override any threshold with the HEALTH_* variables below.
set -u

LOAD_WARN=${HEALTH_LOAD_PER_CORE_WARN:-1.0};   LOAD_CRIT=${HEALTH_LOAD_PER_CORE_CRIT:-2.0}   # 1-minute load per core
MEM_WARN=${HEALTH_MEM_AVAIL_PCT_WARN:-15};     MEM_CRIT=${HEALTH_MEM_AVAIL_PCT_CRIT:-5}      # MemAvailable, % of total (warn at or below)
CG_WARN=${HEALTH_CGROUP_USED_PCT_WARN:-80};    CG_CRIT=${HEALTH_CGROUP_USED_PCT_CRIT:-92}    # cgroup memory used, % of a finite limit
DISK_WARN=${HEALTH_DISK_USED_PCT_WARN:-80};    DISK_CRIT=${HEALTH_DISK_USED_PCT_CRIT:-92}    # filesystem used %
AVAIL_WARN=${HEALTH_DISK_AVAIL_GB_WARN:-5};    AVAIL_CRIT=${HEALTH_DISK_AVAIL_GB_CRIT:-1}    # free GB (warn at or below)
INODE_WARN=${HEALTH_INODE_USED_PCT_WARN:-80};  INODE_CRIT=${HEALTH_INODE_USED_PCT_CRIT:-92}  # inode used %
ZOMBIE_WARN=${HEALTH_ZOMBIES_WARN:-5}
RESTART_SECS=${HEALTH_RECENT_RESTART_SECS:-900}   # uptime below this is reported as a NOTE
DISK_PATH=${HEALTH_DISK_PATH:-/}

sev=0
problems=()
notes=()
flag() { # flag LEVEL(1=WARN,2=CRIT) MESSAGE
  [ "$1" -gt "$sev" ] && sev=$1
  problems+=("$([ "$1" = 2 ] && echo CRIT || echo WARN): $2")
}
ge() { awk -v a="$1" -v b="$2" 'BEGIN{exit !(a+0 >= b+0)}'; }   # a >= b (floats ok)
le() { awk -v a="$1" -v b="$2" 'BEGIN{exit !(a+0 <= b+0)}'; }   # a <= b (floats ok)

# ---- CPU
cores=$(nproc 2>/dev/null || echo 1)
read -r l1 l5 l15 _ < /proc/loadavg
lpc=$(awk -v l="$l1" -v c="$cores" 'BEGIN{printf "%.2f", l/c}')
echo "cpu_cores=$cores load_1_5_15=$l1/$l5/$l15 load_per_core_1m=$lpc"
if ge "$lpc" "$LOAD_CRIT"; then flag 2 "load per core $lpc >= $LOAD_CRIT"
elif ge "$lpc" "$LOAD_WARN"; then flag 1 "load per core $lpc >= $LOAD_WARN"; fi

# ---- Memory
mt=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
ma=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
st=$(awk '/^SwapTotal:/{print $2}' /proc/meminfo)
sf=$(awk '/^SwapFree:/{print $2}' /proc/meminfo)
apct=$(awk -v a="$ma" -v t="$mt" 'BEGIN{printf "%.1f", 100*a/t}')
echo "mem_total_mb=$((mt/1024)) mem_available_mb=$((ma/1024)) mem_available_pct=$apct swap_used_mb=$(((st-sf)/1024))"
if le "$apct" "$MEM_CRIT"; then flag 2 "available memory ${apct}% <= ${MEM_CRIT}%"
elif le "$apct" "$MEM_WARN"; then flag 1 "available memory ${apct}% <= ${MEM_WARN}%"; fi

# ---- cgroup memory (only meaningful when the limit is finite)
cg_limit=""; cg_used=""
if [ -r /sys/fs/cgroup/memory.max ]; then
  cg_limit=$(cat /sys/fs/cgroup/memory.max); cg_used=$(cat /sys/fs/cgroup/memory.current 2>/dev/null)
elif [ -r /sys/fs/cgroup/memory/memory.limit_in_bytes ]; then
  cg_limit=$(cat /sys/fs/cgroup/memory/memory.limit_in_bytes); cg_used=$(cat /sys/fs/cgroup/memory/memory.usage_in_bytes 2>/dev/null)
fi
if [[ "$cg_limit" =~ ^[0-9]+$ && "$cg_used" =~ ^[0-9]+$ && "$cg_limit" -lt 1152921504606846976 ]]; then
  cgpct=$(awk -v u="$cg_used" -v l="$cg_limit" 'BEGIN{printf "%.1f", 100*u/l}')
  echo "cgroup_mem_used_mb=$((cg_used/1048576)) cgroup_mem_limit_mb=$((cg_limit/1048576)) cgroup_mem_used_pct=$cgpct"
  if ge "$cgpct" "$CG_CRIT"; then flag 2 "cgroup memory ${cgpct}% >= ${CG_CRIT}%"
  elif ge "$cgpct" "$CG_WARN"; then flag 1 "cgroup memory ${cgpct}% >= ${CG_WARN}%"; fi
else
  echo "cgroup_mem=unlimited_or_unreadable"
fi

# ---- Disk (free space is the binding number: a per-session allowance can sit below size minus used)
read -r size_kb used_kb avail_kb use_pct < <(df -Pk "$DISK_PATH" 2>/dev/null | awk 'NR==2{print $2,$3,$4,$5}')
use_pct=${use_pct%\%}
avail_gb=$(awk -v k="${avail_kb:-0}" 'BEGIN{printf "%.1f", k/1048576}')
echo "disk_path=$DISK_PATH disk_size_gb=$(awk -v k="${size_kb:-0}" 'BEGIN{printf "%.0f", k/1048576}') disk_used_pct=${use_pct:-?} disk_avail_gb=$avail_gb"
if [[ "${use_pct:-}" =~ ^[0-9]+$ ]]; then
  if ge "$use_pct" "$DISK_CRIT"; then flag 2 "disk ${use_pct}% used >= ${DISK_CRIT}%"
  elif ge "$use_pct" "$DISK_WARN"; then flag 1 "disk ${use_pct}% used >= ${DISK_WARN}%"; fi
fi
if le "$avail_gb" "$AVAIL_CRIT"; then flag 2 "only ${avail_gb} GB free <= ${AVAIL_CRIT} GB"
elif le "$avail_gb" "$AVAIL_WARN"; then flag 1 "only ${avail_gb} GB free <= ${AVAIL_WARN} GB"; fi

ipct=$(df -Pi "$DISK_PATH" 2>/dev/null | awk 'NR==2{gsub("%","",$5); print $5}')
if [[ "$ipct" =~ ^[0-9]+$ ]]; then
  echo "inode_used_pct=$ipct"
  if ge "$ipct" "$INODE_CRIT"; then flag 2 "inodes ${ipct}% used >= ${INODE_CRIT}%"
  elif ge "$ipct" "$INODE_WARN"; then flag 1 "inodes ${ipct}% used >= ${INODE_WARN}%"; fi
fi

# ---- Processes
procs=$(ps -e --no-headers 2>/dev/null | wc -l)
zombies=$(ps -eo stat --no-headers 2>/dev/null | grep -c '^Z')
read -r tpid trss tpmem tcomm < <(ps -eo pid,rss,pmem,comm --sort=-rss --no-headers 2>/dev/null | head -1)
echo "processes=$procs zombies=$zombies top_mem_proc=${tcomm:-?} top_mem_pid=${tpid:-?} top_mem_rss_mb=$(( ${trss:-0} / 1024 )) top_mem_pct=${tpmem:-?}"
if [ "$zombies" -ge "$ZOMBIE_WARN" ]; then flag 1 "$zombies zombie processes >= $ZOMBIE_WARN"; fi

# ---- Uptime (a short uptime means the container or host restarted)
up=$(cut -d. -f1 /proc/uptime)
echo "uptime_s=$up"
if [ "$up" -lt "$RESTART_SECS" ]; then notes+=("restarted ${up}s ago (uptime below ${RESTART_SECS}s)"); fi

# ---- Result
case $sev in 0) status=OK;; 1) status=WARN;; *) status=CRIT;; esac
echo "checked_at=$(date -u +%FT%TZ)"
echo "STATUS=$status"
for ((i = 0; i < ${#problems[@]}; i++)); do echo "PROBLEM: ${problems[$i]}"; done
for ((i = 0; i < ${#notes[@]}; i++)); do echo "NOTE: ${notes[$i]}"; done
echo "SUMMARY: $status | load/core $lpc | mem avail ${apct}% | disk ${use_pct:-?}% used, ${avail_gb} GB free | procs $procs (zombies $zombies) | up ${up}s"
exit "$sev"
