#!/usr/bin/env bash
#
# The load sweep for testdata/testserver.php and its Go twin.
#
# It measures Path B only: HTTP\Mux on runner.Pool, parsed once, handlers
# resolved once at $mux->handle(), workers forked once at listen(), response
# streamed. `phpscript server` is Path A and builds a runtime per request; a
# number from one never belongs in a table with a number from the other. The
# per-request allocations are priced by BenchmarkTestServerRoute, which is the
# other half of this harness. docs/agents/performance.md is the comparison.
#
#	scripts/bench-http.sh before
#	BENCH_SEGMENTS=6 scripts/bench-http.sh after
#
# It writes bench-http-<side>.txt and bench-manifest-<side>.txt at the repo
# root, both gitignored by the bench*.txt line.
#
# wrk rather than hey, which the first run of this harness settled: hey reports
# every latency as four decimal places of a second, so its finest column is 0.1
# ms and a handler here answers in under 0.1 ms. Every percentile it printed was
# the same number. wrk reports microseconds, and what it does not report is a
# 95th percentile - it prints 50, 75, 90 and 99 - so the 90th is collected in
# place of it.
#
# It takes /tmp/phpscript-measure.lock itself rather than asking a caller to,
# because the build and the measurement are one unit: a concurrent `go install`
# on a shared box overwrites the binary mid-sweep. It builds a private
# bin/phpscript and verifies it is what PATH finds, for the same reason.
#
# PHPSCRIPT_MEASURE_LOCK=held says the caller already holds it, for a session
# that sweeps and then runs the Go benchmarks without letting go in between. The
# lock is per open file description, so taking it again from here would wait on
# the caller's own hold and never return.
#
# One server per side for the whole sweep. Restarting between segments is where
# the provenance of a number gets lost, so the drift guard below is what catches
# a segment the box disturbed instead.
set -euo pipefail

cd "$(dirname "$0")/.."
root="$PWD"

side="${1:-before}"

# A segment is one wrk run. Several of them per route rather than one long one,
# because the drift guard needs two ends to compare and a single run reports one
# distribution with no way to tell when inside it the box was busy.
segments="${BENCH_SEGMENTS:-4}"
duration="${BENCH_DURATION:-3s}"
warmup="${BENCH_WARMUP:-2s}"
# Concurrency 1 is the delta. 4 is the queueing table and is never the headline:
# the PHP side answers TESTSERVER_WORKERS requests at once behind a queue of 64
# that is hardcoded in the script's config(), so the comparison stops being
# honest above the worker count.
concurrencies="${BENCH_CONCURRENCY:-1 4}"
targets="${BENCH_TARGETS:-go php}"
workers="${TESTSERVER_WORKERS:-4}"
# Running out of time is a fatal on the PHP side: the shutdown callback fires,
# the server stops listening and the process leaves mid-sweep. This has to
# outlast the whole sweep, not one segment.
limit="${TESTSERVER_LIMIT:-1800}"

export GOFLAGS=""
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"
export GOGC="${GOGC:-100}"

out="$root/bench-http-$side.txt"
manifest="$root/bench-manifest-$side.txt"
work="$(mktemp -d)"
server_pid=""
server_addr=""

# The routes, and what each one prices. /slow is twenty usleep calls by
# construction and prices the deadline machinery rather than the request path,
# so it gets the correctness check at the end and no latency row. /info is
# phpinfo() and nobody serves that shape.
routes=(
	"hello|/hello?name=sprint|"
	"users|/users/42|"
	"echo|/echo|scripts/bench-http-post.lua"
	"index|/|"
)

if [ "${PHPSCRIPT_MEASURE_LOCK:-}" != "held" ]; then
	exec 9>/tmp/phpscript-measure.lock
	flock -w 3600 9
	export PHPSCRIPT_MEASURE_LOCK=held
fi

CGO_ENABLED=0 go build -o "$root/bin/phpscript" .
CGO_ENABLED=0 go build -o "$root/bin/testserver-go" testdata/testserver.go
PATH="$root/bin:$PATH"
export PATH
command -v phpscript | grep -q "^$root/bin/" || {
	echo "phpscript on PATH is not the private build: $(command -v phpscript)" >&2
	exit 1
}

{
	echo "side: $side"
	echo "commit: $(git rev-parse HEAD)"
	echo "dirty: $(test -z "$(git status --porcelain)" && echo no || echo yes)"
	echo "go: $(go version)"
	echo "CGO_ENABLED: 0"
	echo "GOFLAGS: '${GOFLAGS}'"
	echo "GOMEMLIMIT: $GOMEMLIMIT"
	echo "GOGC: $GOGC"
	echo "cores: $(nproc)"
	echo "GOMAXPROCS: unset, so both servers take every core"
	echo "binary: $(command -v phpscript)"
	echo "go twin: $root/bin/testserver-go"
	echo "load generator: $(wrk --version 2>&1 | head -1)"
	echo "date: $(date -Is)"
	echo "uname: $(uname -a)"
	echo "segments per route: $segments"
	echo "segment duration: $duration"
	echo "warmup per route: $warmup"
	echo "concurrencies: $concurrencies"
	echo "TESTSERVER_WORKERS: $workers"
	echo "TESTSERVER_LIMIT: $limit"
	echo "commands:"
	echo "  CGO_ENABLED=0 go build -o bin/phpscript ."
	echo "  CGO_ENABLED=0 go build -o bin/testserver-go testdata/testserver.go"
	echo "  TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=$workers TESTSERVER_LIMIT=$limit phpscript run testdata/testserver.php"
	echo "  TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=$workers TESTSERVER_LIMIT=$limit bin/testserver-go"
	echo "  wrk --latency -t <threads> -c <c> -d $duration [-s scripts/bench-http-post.lua] <url>"
} >"$manifest"

: >"$out"

# start_server brings one side up on a port the kernel picks and answers its
# address. 127.0.0.1:0 rather than the default 8099 because several sprints
# share the box and a fixed port is a collision.
start_server() {
	local target="$1" log="$2"
	case "$target" in
	php) TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS="$workers" TESTSERVER_LIMIT="$limit" \
		phpscript run testdata/testserver.php >"$log" 2>&1 & ;;
	go) TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS="$workers" TESTSERVER_LIMIT="$limit" \
		"$root/bin/testserver-go" >"$log" 2>&1 & ;;
	*)
		echo "unknown target $target" >&2
		exit 1
		;;
	esac
	server_pid=$!
	local addr=""
	for _ in $(seq 1 200); do
		addr="$(sed -n 's|^serving on http://\([^ ]*\) .*|\1|p' "$log" | head -1)"
		[ -n "$addr" ] && break
		sleep 0.05
	done
	if [ -z "$addr" ]; then
		echo "$target server never printed an address" >&2
		cat "$log" >&2
		exit 1
	fi
	server_addr="$addr"
}

stop_server() {
	[ -n "${server_pid:-}" ] || return 0
	kill "$server_pid" 2>/dev/null || true
	wait "$server_pid" 2>/dev/null || true
	server_pid=""
}

# A server left behind outlives the sprint: TESTSERVER_LIMIT is half an hour and
# the PHP side runs until it is up.
trap 'stop_server; rm -rf "$work"' EXIT

# segment reduces one wrk run to "p50 p90 p99 rps non2xx errors", latencies in
# microseconds. wrk writes them with a unit suffix and picks the unit per line,
# so the suffix is what is read rather than assumed.
segment() {
	awk '
		function micros(v) {
			if (v ~ /us$/) return substr(v, 1, length(v) - 2) + 0
			if (v ~ /ms$/) return (substr(v, 1, length(v) - 2) + 0) * 1000
			if (v ~ /s$/) return (substr(v, 1, length(v) - 1) + 0) * 1000000
			return v + 0
		}
		/Latency Distribution/ { dist = 1; next }
		dist && $1 == "50%" { p50 = micros($2) }
		dist && $1 == "90%" { p90 = micros($2) }
		dist && $1 == "99%" { p99 = micros($2); dist = 0 }
		/^Requests\/sec:/ { rps = $2 + 0 }
		/Non-2xx or 3xx responses:/ { bad = $NF + 0 }
		# "Socket errors: connect 0, read 0, write 0, timeout 0", printed only when
		# one of the four is not zero. Every numeric field is one of them.
		/Socket errors:/ {
			for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]/) errs += $i + 0
		}
		END { printf "%.1f %.1f %.1f %.1f %d %d\n", p50, p90, p99, rps, bad + 0, errs + 0 }
	' "$1"
}

# summarise reduces a route's segments to one row: the median of each column,
# and the drift between the first segment and the last.
#
# The drift column is the guard. A segment set whose two ends disagree was
# measured against a box doing something else, and is re-measured rather than
# published.
summarise() {
	awk '
		function pct(arr, count, p,   idx) {
			idx = int(p * count / 100.0 + 0.5)
			if (idx < 1) idx = 1
			if (idx > count) idx = count
			return arr[idx]
		}
		{
			n++
			p50[n] = $1; p90[n] = $2; p99[n] = $3; rps[n] = $4
			bad += $5; errs += $6
			if (n == 1) firstP50 = $1
			lastP50 = $1
		}
		END {
			if (n == 0) { print "0 0 0 0 0 0 0"; exit }
			drift = (firstP50 > 0) ? (lastP50 - firstP50) / firstP50 * 100.0 : 0
			for (i = 1; i <= n; i++) { s50[i] = p50[i]; s90[i] = p90[i]; s99[i] = p99[i]; srps[i] = rps[i] }
			asort(s50, s50, "@val_num_asc")
			asort(s90, s90, "@val_num_asc")
			asort(s99, s99, "@val_num_asc")
			asort(srps, srps, "@val_num_asc")
			printf "%.1f %.1f %.1f %.0f %d %d %d %+.1f\n", \
				pct(s50, n, 50), pct(s90, n, 50), pct(s99, n, 50), pct(srps, n, 50), \
				n, bad + 0, errs + 0, drift
		}
	' "$1"
}

for conc in $concurrencies; do
	# wrk wants no more threads than connections, and one thread per connection is
	# what keeps the generator off the cores the server is answering on.
	threads="$conc"
	[ "$threads" -gt 2 ] && threads=2

	{
		echo "## concurrency $conc, wrk threads $threads"
		echo
		printf '%-8s %-6s %9s %9s %9s %9s %5s %7s %7s %8s\n' \
			route side "p50 us" "p90 us" "p99 us" "req/s" segs non2xx errors "drift %"
	} >>"$out"

	for target in $targets; do
		log="$work/$target-$conc.log"
		start_server "$target" "$log"
		for spec in "${routes[@]}"; do
			IFS='|' read -r name path script <<<"$spec"
			url="http://$server_addr$path"
			wrk_args=(--latency -t "$threads" -c "$conc" --timeout 30s)
			if [ -n "$script" ]; then
				wrk_args+=(-s "$root/$script")
			fi
			wrk "${wrk_args[@]}" -d "$warmup" "$url" >/dev/null 2>&1 || true
			rows="$work/$side-$target-$conc-$name.rows"
			: >"$rows"
			for seg in $(seq 1 "$segments"); do
				raw="$work/$side-$target-$conc-$name-$seg.wrk"
				wrk "${wrk_args[@]}" -d "$duration" "$url" >"$raw" 2>&1
				segment "$raw" >>"$rows"
			done
			# summarise answers eight fields; the split is what fills the row.
			# shellcheck disable=SC2046
			printf '%-8s %-6s %9s %9s %9s %9s %5s %7s %7s %8s\n' \
				"$name" "$target" $(summarise "$rows") >>"$out"
		done
		stop_server
	done
	echo >>"$out"
done

# /slow is not a latency row. What it is is the one place the deadline machinery
# is observable from outside: the client leaves, ignore_user_abort(true) keeps
# the handler running, and connection_aborted() is what lets it skip a response
# nobody is left to read. A change to the request path that breaks this breaks it
# silently, so the sweep checks it rather than timing it.
{
	echo "## /slow abort check"
	echo
} >>"$out"
for target in $targets; do
	log="$work/$target-slow.log"
	start_server "$target" "$log"
	curl -sS --max-time 1 "http://$server_addr/slow" >/dev/null 2>&1 || true
	sleep 2.5
	if grep -q "aborted after" "$log"; then
		echo "$target: connection_aborted fired, response skipped" >>"$out"
	else
		echo "$target: FAIL, no abort recorded" >>"$out"
	fi
	stop_server
done

echo >>"$out"
cat "$manifest" >>"$out"

cat "$out"
