#!/usr/bin/env bash
#
# test-lifecycle drives a real phpscript server through the two operations a
# running one supports: testing a configuration and reloading it.
#
# What it covers that the Go tests cannot:
#
#   the pidfile      that the file names the process actually running, which
#                    in a Go test would be the test binary comparing its own
#                    pid to itself.
#   -s reload        the signal crossing a process boundary, rather than a
#                    Reload call made in the same process.
#   the refusal      that a SIGHUP carrying a configuration the server cannot
#                    use leaves the old one serving, and that the server is
#                    still answering afterwards.
#   the stop         that SIGTERM drains rather than killing, and takes the
#                    pidfile with it.
#
# Readiness is the pidfile rather than a successful request. The platform
# writes it after the reload handler is armed, and a SIGHUP arriving before
# that takes its default disposition and kills the process.
#
# Usage: scripts/test-lifecycle.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
pid=""

log() { printf '%s\n' "$*"; }
die() {
	printf 'FAIL: %s\n' "$*" >&2
	[ -s "$work/server.log" ] && { echo '--- server log ---' >&2; cat "$work/server.log" >&2; }
	exit 1
}

cleanup() {
	[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && kill -TERM "$pid" 2>/dev/null
	[ -n "$pid" ] && wait "$pid" 2>/dev/null
	rm -rf "$work"
}
trap cleanup EXIT

# A port nothing else in the pipeline uses. 0 would let the kernel pick, but
# then the port has to be scraped back out of the log, and a fixed one keeps
# what the script asserts readable.
port=8791
url="http://127.0.0.1:$port"
pidfile="$work/run.pid"

log "building"
go build -o "$work/phpscript" "$root" || die "build"

site() {
	mkdir -p "$work/site-$1/public"
	printf '<?php echo "%s";' "$1" > "$work/site-$1/public/index.php"
	printf 'telemetry:\n  enabled: false\n' > "$work/site-$1/phpscript.yml"
}
site a
site b

config() {
	{
		printf 'telemetry:\n  enabled: false\n'
		printf 'server:\n  addr: "127.0.0.1:%s"\n  pid_file: "%s"\n' "$port" "$pidfile"
		printf 'virtualhost:\n'
		for name in "$@"; do
			printf '  - domain: %s.localhost\n    root: %s\n' "$name" "$work/site-$name"
		done
	} > "$work/config.yml"
}

get() { curl -s -o /dev/null -w '%{http_code}' -H "Host: $1.localhost" "$url/"; }
body() { curl -s -H "Host: $1.localhost" "$url/"; }

# 1. A configuration is testable before anything runs.
config a
"$work/phpscript" -f "$work/config.yml" -t > "$work/t.out" 2>&1 || die "-t rejected a good configuration: $(cat "$work/t.out")"
grep -q 'config.yml: ok' "$work/t.out" || die "-t printed $(cat "$work/t.out")"
log "ok: -t accepts a good configuration"

# 2. The server starts and records its pid.
"$work/phpscript" -f "$work/config.yml" server > "$work/server.log" 2>&1 &
pid=$!

for _ in $(seq 60); do
	[ -s "$pidfile" ] && break
	sleep 0.5
done
[ -s "$pidfile" ] || die "no pidfile after 30s"

recorded="$(cat "$pidfile")"
[ "$recorded" = "$pid" ] || die "pidfile holds $recorded, the server is $pid"
log "ok: the pidfile names the running server"

[ "$(body a)" = "a" ] || die "a.localhost did not answer"
[ "$(get b)" = "404" ] || die "b.localhost answered before it was configured"
log "ok: the configured site serves and an unconfigured one does not"

# 3. A test of a broken configuration does not touch the running server.
config a b
printf '  - domain: c.localhost\n    root: %s/absent\n' "$work" >> "$work/config.yml"

if "$work/phpscript" -f "$work/config.yml" -t > "$work/t.out" 2>&1; then
	die "-t accepted a virtual host with no root"
fi
grep -q 'config.yml: failed' "$work/t.out" || die "-t printed $(cat "$work/t.out")"
[ "$(body a)" = "a" ] || die "-t disturbed the running server"
log "ok: -t rejects a missing root and leaves the server alone"

# 4. A reload of that same broken configuration is refused, and the server
#    keeps serving. This is the signal path, not the command path: a
#    kill -HUP gets the same answer.
kill -HUP "$pid"
sleep 2

kill -0 "$pid" 2>/dev/null || die "a refused reload killed the server"
[ "$(body a)" = "a" ] || die "a.localhost stopped answering after a refused reload"
[ "$(cat "$pidfile")" = "$pid" ] || die "the pidfile changed across a refused reload"
grep -q 'reload refused, still serving' "$work/server.log" || die "the log does not report the refusal"
log "ok: a reload that cannot be applied leaves the old server serving"

# 5. A good reload applies the edit, in the same process.
config a b
"$work/phpscript" -f "$work/config.yml" -t > /dev/null 2>&1 || die "-t rejected the fixed configuration"
"$work/phpscript" -f "$work/config.yml" -s reload || die "-s reload failed"

for _ in $(seq 20); do
	[ "$(get b)" = "200" ] && break
	sleep 0.5
done
[ "$(body b)" = "b" ] || die "the reloaded site never answered"
[ "$(body a)" = "a" ] || die "the site that was already there stopped answering"
[ "$(cat "$pidfile")" = "$pid" ] || die "the reload replaced the process instead of the platform"
log "ok: -s reload applies an edit without replacing the process"

# 6. The errors -s gives.
if "$work/phpscript" -f "$work/config.yml" -s restart > "$work/s.out" 2>&1; then
	die "-s accepted an unknown verb"
fi
grep -q 'want reload' "$work/s.out" || die "-s restart printed $(cat "$work/s.out")"
log "ok: -s refuses a verb it does not have"

# 7. SIGTERM drains and takes the pidfile with it.
kill -TERM "$pid"
for _ in $(seq 20); do
	kill -0 "$pid" 2>/dev/null || break
	sleep 0.5
done
kill -0 "$pid" 2>/dev/null && die "the server ignored SIGTERM"
wait "$pid" 2>/dev/null || true
pid=""

[ -e "$pidfile" ] && die "the pidfile outlived the server"
log "ok: SIGTERM stops the server and removes the pidfile"

log "lifecycle: all checks passed"
