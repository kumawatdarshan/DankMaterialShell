#!/usr/bin/env bash
# Snapshot Rss/Pss/Private_Dirty for the live shell processes.
# Usage: scripts/profile-memory.sh [label]
set -euo pipefail

label="${1:-snapshot}"

snapshot() {
    local name="$1" pattern="$2" pid
    pid=$(pgrep -f "$pattern" | head -n1 || true)
    if [[ -z "$pid" ]]; then
        echo "[$label] $name: not running"
        return 0
    fi
    echo "[$label] $name (PID $pid):"
    grep -E '^(Rss|Pss|Private_Dirty):' "/proc/$pid/smaps_rollup"
}

snapshot "dms" "dms -c"
snapshot "quickshell" "quickshell -p"
