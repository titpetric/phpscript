#!/usr/bin/env bash
#
# The load sweep for one API written five ways, one directory each under
# testdata/, driven by one load generator so the difference reads off directly.
#
# BENCH_TARGETS names the sides. The default pair needs no docker:
#
#	goserver         testdata/goserver, net/http, the floor: no VM
#	bridge           testdata/phpscript-http-bridge, Path B
#	service          testdata/phpscript-service, Path A
#	vanilla-apache   testdata/php-vanilla on apache with mod_php, Slim
#	vanilla-nginx    testdata/php-vanilla on nginx with php-fpm, Slim
#
#	BENCH_TARGETS="goserver bridge service vanilla-apache vanilla-nginx" scripts/bench-http.sh after
#
# This script brings every side up and tears it down. Each one shares the
# host's network namespace, answers on the same worker count and runs at
# nice -n -20. What differs between them is the stack.
#
# One table per side, because two of them are different execution models.
# `bridge` is Path B: the script is the server, parsed once, handlers resolved
# at $mux->handle(), workers forked once at listen(). `service` is Path A:
# annotated endpoints under `phpscript server`, a pooled runtime reset between
# requests, with the superglobals decoded for each one. A number from one never
# belongs in a table with a number from the other. The table header names the
# model. BenchmarkTestServerRoute prices the per-request allocations, the other
# half of this harness. docs/agents/performance.md is the comparison.
#
#	scripts/bench-http.sh before
#	BENCH_SEGMENTS=6 scripts/bench-http.sh after
#
# It writes bench-http-<side>.txt and bench-manifest-<side>.txt at the
# repository root, both gitignored by the bench*.txt line.
#
# wrk and not hey, which the first run of this harness settled: hey reports
# every latency as four decimal places of a second, which leaves its finest
# column coarser than any latency this harness measures. It prints the same
# number for every percentile. wrk reports microseconds. The one percentile wrk
# never prints is the 95th - it prints 50, 75, 90 and 99 - and the 90th stands
# in for it.
#
# It takes /tmp/phpscript-measure.lock itself, and no caller has to: the build
# and the measurement are one unit, and a concurrent `go install` on a shared
# box overwrites the binary mid-sweep. It builds a private bin/phpscript and
# checks that PATH finds that one, for the same reason.
#
# Each step takes the lock; the run never holds it. Two sweeps cannot share the
# build or one route's segment set, which is half a minute of work. A whole
# sweep is twenty minutes, and holding the lock for all twenty serialised two
# other sprints behind the first run of this script. A server stays up across
# the gaps, idle. TESTSERVER_LIMIT has to outlast the waiting as well as the
# measuring.
#
# PHPSCRIPT_MEASURE_LOCK=held says the caller already holds it, for a session
# that sweeps and then runs the Go benchmarks without letting go. The lock is
# per open file description. Taking it again from here would wait on the
# caller's own hold and never return.
#
# One server per side for the whole sweep. Restarting between segments loses
# the provenance of a number. The drift guard below catches a segment the box
# disturbed.
#
# Both servers and the generator run at `nice -n -20`, symmetrically, the way
# `atkins bench:pinned` does and for the same reason. Without it the first run
# of this sweep put millisecond tails on a Go server answering in tens of
# microseconds and failed its own drift guard twice: the lock keeps other
# agents from measuring, not from running, and the kernel deschedules a
# default-priority server on a shared box for milliseconds at a time.
set -euo pipefail

cd "$(dirname "$0")/.."
root="$PWD"

side="${1:-before}"

# A segment is one wrk run. A route takes a set of them: the drift guard needs
# two ends to compare, and a single long run reports one distribution with no
# way to tell which part of it the box spent busy.
segments="${BENCH_SEGMENTS:-6}"
duration="${BENCH_DURATION:-3s}"
warmup="${BENCH_WARMUP:-2s}"
# Concurrency 1 is the delta. 4 is the queueing table and is never the headline:
# the PHP side answers TESTSERVER_WORKERS requests at once behind the queue of
# 64 the script's `config()` sets. Above the worker count the comparison stops
# being honest.
concurrencies="${BENCH_CONCURRENCY:-1 4}"
# The five sides, one directory each under testdata/. The default pair is the
# one that needs no docker; the rest are opt-in.
#
#	goserver         testdata/goserver, net/http, the floor
#	bridge           testdata/phpscript-http-bridge, Path B
#	service          testdata/phpscript-service, Path A
#	vanilla-apache   testdata/php-vanilla on apache with mod_php
#	vanilla-nginx    testdata/php-vanilla on nginx with php-fpm
targets="${BENCH_TARGETS:-goserver bridge}"
workers="${TESTSERVER_WORKERS:-4}"
# The vanilla profiles listen here and answer this many requests at once, which
# is TESTSERVER_WORKERS on the other three. compose.yml reads all three.
vanilla_port="${VANILLA_PORT:-8098}"
vanilla_nginx_port="${VANILLA_NGINX_PORT:-8099}"
vanilla_workers="${VANILLA_WORKERS:-$workers}"
compose_dir="$root/testdata/php-vanilla"
# Which profile is up. The teardown and the memory probe read it to reach the
# right containers.
vanilla_profile=""
# Running out of time is a fatal on the PHP side: the shutdown callback fires,
# the server stops listening and the process leaves mid-sweep. This outlasts
# the whole sweep and the waiting between its steps, never one segment.
limit="${TESTSERVER_LIMIT:-7200}"

export GOFLAGS=""
export GOMEMLIMIT="${GOMEMLIMIT:-2GiB}"
export GOGC="${GOGC:-100}"

out="$root/bench-http-$side.txt"
manifest="$root/bench-manifest-$side.txt"
work="$(mktemp -d)"
segout="$work/segments.txt"
memout="$work/memory.txt"
server_pid=""
server_addr=""
server_kind=""
vanilla_logs_pid=""

# The routes, and what each one prices. /slow is twenty usleep calls by
# construction and prices the deadline machinery, which leaves the request path
# unmeasured: it takes the correctness check at the end and no latency row.
# /info is phpinfo() and nobody serves that shape.
routes=(
	"hello|/hello?name=sprint|"
	"users|/users/42|"
	"echo|/echo|scripts/bench-http-post.lua"
	"index|/|"
)

lock_held="${PHPSCRIPT_MEASURE_LOCK:-}"
if [ "$lock_held" != "held" ]; then
	exec 9>/tmp/phpscript-measure.lock
fi

# with_lock runs one step under the lock. A caller already holding it runs
# through unchanged: the lock is per open file description, and taking it twice
# would wait on that caller's own hold and never return.
with_lock() {
	if [ "$lock_held" = "held" ]; then
		"$@"
		return $?
	fi
	flock -w 3600 9 || return 1
	local status=0
	"$@" || status=$?
	flock -u 9
	return $status
}

build_binaries() {
	CGO_ENABLED=0 go build -o "$root/bin/phpscript" .
	CGO_ENABLED=0 go build -o "$root/bin/testserver-go" testdata/goserver/main.go
}

with_lock build_binaries

# The vanilla images once, ahead of the sweep. The loop below calls start_server
# per concurrency and again for the abort check, and a build in that function
# would put docker's work inside the measurement window. This builds both
# profiles whichever one the sweep reads, because the two share public/index.php
# and a stale image for either is a column answering for code this run never
# built.
case " $targets " in
*" vanilla-"*)
	VANILLA_PORT="$vanilla_port" VANILLA_NGINX_PORT="$vanilla_nginx_port" VANILLA_WORKERS="$vanilla_workers" \
		docker compose -f "$compose_dir/compose.yml" --profile apache --profile nginx build >/dev/null
	;;
esac
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
	echo "GOMAXPROCS: unset: both servers take every core"
	echo "binary: $(command -v phpscript)"
	echo "go floor: $root/bin/testserver-go"
	echo "load generator: $(wrk --version 2>&1 | head -1)"
	echo "priority: nice -n -20, server and generator alike"
	echo "date: $(date -Is)"
	echo "uname: $(uname -a)"
	echo "segments per route: $segments"
	echo "segment duration: $duration"
	echo "warmup per route: $warmup"
	echo "concurrencies: $concurrencies"
	echo "TESTSERVER_WORKERS: $workers"
	echo "TESTSERVER_LIMIT: $limit"
	echo "targets: $targets"
	case " $targets " in
	*" vanilla-"*)
		echo "VANILLA_PORT: $vanilla_port"
		echo "VANILLA_NGINX_PORT: $vanilla_nginx_port"
		echo "VANILLA_WORKERS: $vanilla_workers"
		echo "vanilla php: $(docker run --rm phpscript/php-vanilla-apache:local php -r 'echo PHP_VERSION;' 2>/dev/null)"
		echo "vanilla slim: $(jq -r '.packages[] | select(.name == "slim/slim") | .version' "$compose_dir/composer.lock" 2>/dev/null)"
		echo "vanilla opcache: on, validate_timestamps off, route cache on, autoloader classmap-authoritative"
		echo "vanilla network: host: no published port and no NAT hop"
		;;
	esac
	echo "commands:"
	echo "  CGO_ENABLED=0 go build -o bin/phpscript ."
	echo "  CGO_ENABLED=0 go build -o bin/testserver-go testdata/goserver/main.go"
	echo "  TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=$workers TESTSERVER_LIMIT=$limit phpscript run testdata/phpscript-http-bridge/server.php"
	echo "  TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=$workers TESTSERVER_LIMIT=$limit bin/testserver-go"
	echo "  phpscript server -f <addr 127.0.0.1:0> testdata/phpscript-service"
	case " $targets " in
	*" vanilla-"*)
		echo "  VANILLA_PORT=$vanilla_port VANILLA_NGINX_PORT=$vanilla_nginx_port VANILLA_WORKERS=$vanilla_workers docker compose -f testdata/php-vanilla/compose.yml --profile <apache|nginx> up -d --wait"
		;;
	esac
	echo "  nice -n -20 wrk --latency -t <threads> -c <c> -d $duration [-s scripts/bench-http-post.lua] <url>"
} >"$manifest"

: >"$out"
: >"$segout"
: >"$memout"

# start_server brings one side up on a port the kernel picks and answers its
# address. 127.0.0.1:0 and not the default 8099, because the box runs more than
# one sprint at a time and a fixed port is a collision.
# target_model names the execution model a table's numbers describe. The header
# states what the rows mean, which saves a reader the directory a side came
# from.
target_model() {
	case "$1" in
	goserver) echo "testdata/goserver on net/http, the floor: no VM, one goroutine per request." ;;
	bridge) echo "testdata/phpscript-http-bridge, Path B: the script is the server, parsed once, workers forked at listen()." ;;
	service) echo "testdata/phpscript-service, Path A: annotated endpoints under \`phpscript server\`, a pooled runtime reset between requests." ;;
	vanilla-apache) echo "testdata/php-vanilla on apache with mod_php, Slim: a bootstrap per request." ;;
	vanilla-nginx) echo "testdata/php-vanilla on nginx with php-fpm, Slim: a bootstrap per request." ;;
	*) echo "$1" ;;
	esac
}

start_server() {
	local target="$1" log="$2"
	server_kind="$target"
	case "$target" in
	bridge) TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS="$workers" TESTSERVER_LIMIT="$limit" \
		nice -n -20 phpscript run testdata/phpscript-http-bridge/server.php >"$log" 2>&1 & ;;
	goserver) TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS="$workers" TESTSERVER_LIMIT="$limit" \
		nice -n -20 "$root/bin/testserver-go" >"$log" 2>&1 & ;;
	service)
		start_service "$log"
		return
		;;
	vanilla-apache | vanilla-nginx)
		start_vanilla "$target" "$log"
		return
		;;
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

# start_service brings up `phpscript server` over the annotated endpoints, on a
# port the kernel picks.
#
# It writes its own configuration and never reads the directory's, because
# `config.yml` names a fixed port for a reader bringing it up by hand and a
# sweep needs one the kernel chose: two sweeps on one box would otherwise
# collide on it. The rest of the file holds what the directory sets, route
# scanning included.
#
# This reads the address off the server's own log line, which spells it
# `url=http://host:port` where the other two spell `serving on http://...`.
start_service() {
	local log="$1" config_file="$work/service.yml"
	{
		echo "runner:"
		echo "  work_dir: \".\""
		echo "routes:"
		echo "  enabled: true"
		echo "server:"
		echo "  addr: \"127.0.0.1:0\""
		echo "  quiet: false"
	} >"$config_file"

	nice -n -20 phpscript server -f "$config_file" "$root/testdata/phpscript-service" >"$log" 2>&1 &
	server_pid=$!

	local addr=""
	for _ in $(seq 1 200); do
		addr="$(sed -n 's|.*url=http://\([^ ]*\).*|\1|p' "$log" | head -1)"
		[ -n "$addr" ] && break
		sleep 0.05
	done
	if [ -z "$addr" ]; then
		echo "the service never printed an address" >&2
		cat "$log" >&2
		exit 1
	fi
	server_addr="$addr"
}

# start_vanilla brings the compose stack up and waits for its healthcheck.
#
# It takes a fixed port where the other two take 0, because a container sharing
# the host's network namespace has no address to report back and compose
# publishes no port the kernel picked. Host networking is what the measurement
# needs: a published port adds docker-proxy or a NAT hop, and the numbers here
# are tens of microseconds.
#
# This renices the container's processes after the healthcheck, because the
# other two sides run at nice -n -20 and the kernel deschedules a
# default-priority server on a shared box for milliseconds at a time. Apache
# forks its children at startup. They exist by the time this runs, each one
# reniced by hand.
start_vanilla() {
	local target="$1" log="$2" profile port
	profile="${target#vanilla-}"
	case "$profile" in
	apache) port="$vanilla_port" ;;
	nginx) port="$vanilla_nginx_port" ;;
	esac

	vanilla_profile="$profile"
	VANILLA_PORT="$vanilla_port" VANILLA_NGINX_PORT="$vanilla_nginx_port" VANILLA_WORKERS="$vanilla_workers" \
		docker compose -f "$compose_dir/compose.yml" --profile "$profile" up -d --wait --wait-timeout 180 >"$log" 2>&1 || {
		echo "the $target stack did not come up" >&2
		docker compose -f "$compose_dir/compose.yml" --profile "$profile" logs --no-log-prefix >&2 || true
		exit 1
	}
	# The log the abort check greps. error_log inside the handler reaches the
	# web server's stderr, which is the container's, which is this.
	VANILLA_PORT="$vanilla_port" VANILLA_NGINX_PORT="$vanilla_nginx_port" VANILLA_WORKERS="$vanilla_workers" \
		docker compose -f "$compose_dir/compose.yml" --profile "$profile" logs -f --no-log-prefix >"$log" 2>&1 &
	vanilla_logs_pid=$!

	# Every process in every container of the profile, because the other sides
	# run at nice -n -20 and the kernel deschedules a default-priority server on
	# a shared box for milliseconds at a time. The web server forks its children
	# at startup. They exist by now and this renices each by hand.
	local container pid
	for container in $(vanilla_containers "$profile"); do
		for pid in $(docker top "$container" -eo pid 2>/dev/null | tail -n +2); do
			renice -n -20 -p "$pid" >/dev/null 2>&1 || true
		done
	done
	server_addr="127.0.0.1:$port"
}

# vanilla_containers names the containers one profile runs: apache is one, and
# nginx is the front and the fpm pool behind it.
vanilla_containers() {
	case "$1" in
	apache) echo "phpscript-php-vanilla-apache" ;;
	nginx) echo "phpscript-php-vanilla-nginx phpscript-php-vanilla-fpm" ;;
	esac
}

stop_server() {
	case "${server_kind:-}" in
	vanilla-*)
		[ -n "${vanilla_logs_pid:-}" ] && kill "$vanilla_logs_pid" 2>/dev/null
		vanilla_logs_pid=""
		VANILLA_PORT="$vanilla_port" VANILLA_NGINX_PORT="$vanilla_nginx_port" VANILLA_WORKERS="$vanilla_workers" \
			docker compose -f "$compose_dir/compose.yml" --profile "$vanilla_profile" down >/dev/null 2>&1 || true
		vanilla_profile=""
		server_kind=""
		return 0
		;;
	esac
	[ -n "${server_pid:-}" ] || return 0
	kill "$server_pid" 2>/dev/null || true
	wait "$server_pid" 2>/dev/null || true
	server_pid=""
	server_kind=""
}

# record_memory writes what the side holds in memory with the sweep's last
# request answered and nothing torn down yet.
#
# RSS for a host process, because phpscript and the Go twin each answer every
# request on one process whose workers are threads of it. The container's
# own cgroup for the vanilla side, because apache's parent and its children are
# separate processes and the cgroup is what counts them together. The two are
# not the same measurement and the note column says which is which.
record_memory() {
	local target="$1" conc="$2" kb note
	case "$target" in
	vanilla-*)
		# docker stats writes "95.2MiB / 1GiB" and picks the unit per line. This
		# reads the suffix off each line, the way segment() reads a wrk latency.
		# Every container of the profile, summed: the nginx profile is two.
		# shellcheck disable=SC2046
		kb="$(docker stats --no-stream --format '{{.MemUsage}}' $(vanilla_containers "$vanilla_profile") 2>/dev/null |
			awk '{
				value = $1 + 0
				unit = $1
				sub(/^[0-9.]+/, "", unit)
				if (unit ~ /^GiB/) value *= 1024 * 1024
				else if (unit ~ /^MiB/) value *= 1024
				else if (unit ~ /^B/) value /= 1024
				total += value
			}
			END { printf "%.0f", total }')" || kb=0
		note="container cgroups, the web server and its $vanilla_workers workers"
		;;
	*)
		# Both guarded: ps answers 1 for a process with no children, and
		# phpscript and the Go twin have no child process, their workers being
		# threads of the one process. Under pipefail that 1 reaches the
		# assignment and ends the sweep.
		kb="$({
			ps -o rss= -p "$server_pid" 2>/dev/null || true
			ps --ppid "$server_pid" -o rss= 2>/dev/null || true
		} | awk '{ total += $1 } END { printf "%.0f", total }')" || kb=0
		note="process RSS, one process answering on $workers workers"
		;;
	esac
	printf '%-10s %-16s %5s %10s  %s\n' "$side" "$target" "$conc" "${kb:-0}" "$note" >>"$memout"
}

# measure_route is the step the lock protects: one warmup and one segment set
# for one route, written as one line per segment, and no other work.
measure_route() {
	local name="$1" target="$2" url="$3" rows="$4"
	nice -n -20 wrk "${wrk_args[@]}" -d "$warmup" "$url" >/dev/null 2>&1 || true
	: >"$rows"
	local seg raw
	for seg in $(seq 1 "$segments"); do
		raw="$work/$side-$target-$conc-$name-$seg.wrk"
		nice -n -20 wrk "${wrk_args[@]}" -d "$duration" "$url" >"$raw" 2>&1
		segment "$raw" >>"$rows"
	done
}

# A server left behind outlives the sprint: TESTSERVER_LIMIT is half an hour and
# the PHP side runs until that time passes.
trap 'stop_server; rm -rf "$work"' EXIT

# segment reduces one wrk run to "p50 p90 p99 rps non2xx errors", latencies in
# microseconds. wrk writes them with a unit suffix and picks the unit per line.
# The awk program below converts whatever suffix a line spells.
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
# The drift column is the guard. A segment set whose two ends disagree measured
# a box doing something else: it goes back for a re-measurement, and where the
# box will not go quiet it publishes as the interval the segment appendix below
# holds, never as this row's point estimate.
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

# One table per runtime, and not one table per concurrency with a side column.
#
# Two of the sides are different execution models: phpscript-service is Path A,
# a pooled runtime reset between requests, and phpscript-http-bridge is Path B,
# workers forked once at listen(). docs/agents/performance.md forbids a Path A number
# and a Path B number sharing a table, because the two answer different
# questions and a reader comparing rows across them is comparing nothing. The
# table header names the path. What each number describes travels with it.
#
# Target-major and not concurrency-major: one server comes up once and answers
# every concurrency before the next side starts. A side brought up twice is two
# warmups and two opcache states.
for target in $targets; do
	for conc in $concurrencies; do
		# wrk wants no more threads than connections, and one thread per
		# connection is what keeps the generator off the cores the server is
		# answering on.
		threads="$conc"
		[ "$threads" -gt 2 ] && threads=2

		log="$work/$target-$conc.log"
		start_server "$target" "$log"

		{
			echo "## $target, concurrency $conc, wrk threads $threads"
			echo
			echo "$(target_model "$target")"
			echo
			printf '%-8s %9s %9s %9s %9s %5s %7s %7s %8s\n' \
				route "p50 us" "p90 us" "p99 us" "req/s" segs non2xx errors "drift %"
		} >>"$out"

		for spec in "${routes[@]}"; do
			IFS='|' read -r name path script <<<"$spec"
			url="http://$server_addr$path"
			wrk_args=(--latency -t "$threads" -c "$conc" --timeout 30s)
			if [ -n "$script" ]; then
				wrk_args+=(-s "$root/$script")
			fi
			rows="$work/$side-$target-$conc-$name.rows"
			# One lock for the warmup and the segments of one route, the unit two
			# sweeps cannot share. The next route takes it again.
			with_lock measure_route "$name" "$target" "$url" "$rows"
			# summarise answers nine fields after the route; the split is what
			# fills the row.
			# shellcheck disable=SC2046
			printf '%-8s %9s %9s %9s %9s %5s %7s %7s %8s\n' \
				"$name" $(summarise "$rows") >>"$out"
			awk -v route="$name" -v target="$target" -v conc="$conc" \
				'{ printf "%-14s %-16s %5s %5d %9s %9s %9s %9s\n", route, target, conc, NR, $1, $2, $3, $4 }' \
				"$rows" >>"$segout"
		done
		echo >>"$out"

		# Before the teardown, with the last request answered: what the side
		# holds after a sweep is the number, not what it started with.
		record_memory "$target" "$conc"
		stop_server
	done
done

# The segments behind every row. A row is their median, and a row published as
# an interval quotes these lines in place of a point estimate.
{
	echo "## segments, one wrk run each"
	echo
	printf '%-14s %-16s %5s %5s %9s %9s %9s %9s\n' route side conc seg "p50 us" "p90 us" "p99 us" "req/s"
	cat "$segout"
	echo
} >>"$out"

# What each side holds in memory with its sweep finished. The note column tells
# the two measurements apart: a host process reports its own RSS and the
# container reports a cgroup counting apache's parent and every child it forked.
{
	echo "## memory with the sweep answered"
	echo
	printf '%-10s %-16s %5s %10s  %s\n' side target conc "rss kb" note
	cat "$memout"
	echo
} >>"$out"

# /slow is not a latency row. The deadline machinery shows from outside in this
# one place: the client leaves, ignore_user_abort(true) keeps the handler
# running, and connection_aborted() lets the handler skip a response no reader
# waits for. A change to the request path breaks this silently. The sweep
# checks it and times nothing.
{
	echo "## /slow abort check"
	echo
} >>"$out"
for target in $targets; do
	log="$work/$target-slow.log"
	start_server "$target" "$log"
	curl -sS --max-time 1 "http://$server_addr/slow" >/dev/null 2>&1 || true
	sleep 2.5
	case "$target" in
	service)
		# Not checked here, and not a failure. error_log() on a served request
		# records on the oida span and writes nowhere else, which is the
		# contract: oida is the sink, and only a run with no trace and no error
		# handler falls back to the process error stream. Reading it would mean
		# scraping /debug/oida, which is a tenant-scoped admin surface behind a
		# login in a virtual-hosting deployment, and a load harness has no
		# business reaching into one. A fixture over connection_aborted() covers
		# the endpoint's own behaviour.
		echo "$target: not checked, the abort records on the oida span" >>"$out"
		;;
	*)
		if grep -q "aborted after" "$log"; then
			echo "$target: connection_aborted fired, response skipped" >>"$out"
		else
			echo "$target: FAIL, no abort recorded" >>"$out"
		fi
		;;
	esac
	stop_server
done

echo >>"$out"
cat "$manifest" >>"$out"

cat "$out"
