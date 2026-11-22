#!/usr/bin/env bash
# perf.sh BIN FILE MAX_MS [RUNS]
# Measures kmd end-to-end (process start → read → parse → render → exit),
# exercising every feature: images are decoded (-i blocks) and OSC 8 links
# emitted even though stdout is not a terminal.
#
# Each kmd run is paired with a /bin/true run in the same instant, so the
# report separates the cost of kmd from the cost of exec'ing *any* program
# on a loaded machine. Gates:
#   - p50 wall time of kmd            < MAX_MS
#   - p95 of (kmd − paired /bin/true) < MAX_MS   (robust to machine load)
# Timing uses bash's $EPOCHREALTIME: no extra processes are spawned.
set -euo pipefail

bin=$1 file=$2 max_ms=$3 runs=${4:-200}
args=(--color=always --hyperlinks=always --images=blocks --no-pager -w 100 "$file")
export COLORTERM=truecolor

"$bin" "${args[@]}" >/dev/null # warm-up (page cache)

kmd=() base=() over=()
for ((i = 0; i < runs; i++)); do
	t0=${EPOCHREALTIME/[.,]/}
	/bin/true
	t1=${EPOCHREALTIME/[.,]/}
	"$bin" "${args[@]}" >/dev/null
	t2=${EPOCHREALTIME/[.,]/}
	b=$((10#$t1 - 10#$t0)) k=$((10#$t2 - 10#$t1))
	base+=("$b") kmd+=("$k") over+=($((k > b ? k - b : 0)))
done

pct() { # pct P values... → P-th percentile
	local p=$1
	shift
	local -a s
	mapfile -t s < <(printf '%s\n' "$@" | sort -n)
	echo "${s[$((${#s[@]} * p / 100))]}"
}
ms() { printf '%d.%03d' $(($1 / 1000)) $(($1 % 1000)); }

k50=$(pct 50 "${kmd[@]}") k95=$(pct 95 "${kmd[@]}")
b50=$(pct 50 "${base[@]}") b95=$(pct 95 "${base[@]}")
o50=$(pct 50 "${over[@]}") o95=$(pct 95 "${over[@]}")

printf '  %d runs, %s (%s bytes, all features)\n' "$runs" "$file" "$(wc -c <"$file")"
printf '  kmd        p50 %s ms · p95 %s ms\n' "$(ms "$k50")" "$(ms "$k95")"
printf '  /bin/true  p50 %s ms · p95 %s ms   (cost of exec on this machine right now)\n' "$(ms "$b50")" "$(ms "$b95")"
printf '  kmd − true p50 %s ms · p95 %s ms   (limit: p50 and overhead p95 < %d ms)\n' "$(ms "$o50")" "$(ms "$o95")" "$max_ms"

fail=0
((k50 < max_ms * 1000)) || { echo "  FAIL: kmd p50 above ${max_ms} ms" >&2; fail=1; }
((o95 < max_ms * 1000)) || { echo "  FAIL: kmd overhead p95 above ${max_ms} ms" >&2; fail=1; }
exit $fail
