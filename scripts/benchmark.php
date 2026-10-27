#!/usr/bin/env phpscript
<?php

// benchmark.php is the load sweep as one command. It brings each test
// environment up, drives wrk at every endpoint, and prints two tables: one row
// per endpoint, and one row per environment with the endpoints collapsed.
//
// Run it from the repository root, the directory the testdata paths resolve
// against:
//
//	scripts/benchmark.php
//	scripts/benchmark.php --targets goserver,phpscript-service --concurrency 1
//	scripts/benchmark.php --segments 3 --duration 2s
//
// --endpoints replaces the API's four routes with name=path pairs, for a
// measurement whose subject is one file, and --jit is what the php-vanilla
// stacks set opcache.jit to, php's own `disable` by default:
//
//	scripts/benchmark.php --targets php-vanilla-apache --jit tracing \
//		--endpoints bare=/_bare.php,hello=/hello?name=sprint
//
// A summary row collapses the endpoints of its own run, and a list mixing one
// cheap path with one expensive path leaves that row describing neither. Read
// the per-endpoint table for those.
//
// A runtime takes the name of its directory, with the concurrency appended:
// `phpscript-service-4` is testdata/phpscript-service answering four requests
// at once. php-vanilla is one directory served two ways, and it carries the web
// server in its name.
//
// What this prints is a point estimate. scripts/bench-http.sh
// stays the harness of record: the drift column that catches a box doing
// something else, the memory section, the per-segment appendix, the manifest of
// toolchain and commands, and the /slow abort check are all there and none of
// them is here.

$FLAGS = array(
	"targets" => "goserver,phpscript-http-bridge,phpscript-service,php-vanilla-apache,php-vanilla-nginx",
	"concurrency" => "1,4",
	"segments" => "6",
	"duration" => "3s",
	"warmup" => "2s",
	"workers" => "4",
	"limit" => "7200",
	"endpoints" => "",
	"jit" => "disable",
);

// fail reports a condition the run cannot continue past and stops. Whatever is
// up at that moment comes down through teardown, which runs on every exit path.
function fail($message) {
	fwrite(STDERR, "benchmark: " . $message . "\n");
	exit(1);
}

// pid_file and profile_file record what is up, and teardown reaches it from any
// exit path. `.gitignore` holds bin/, and it resolves the same for both sides of every
// call here: a relative path reads inside the runtime's root, and the shell a
// command runs through starts in that same directory.
function pid_file() {
	return "bin/benchmark.pid";
}

function profile_file() {
	return "bin/benchmark.profile";
}

// teardown takes down whatever the work files name as up. A server left behind holds
// its port and keeps its workers on the cores the next measurement needs, and a
// shutdown function registers this and every exit path reaches it, the
// guarantee `trap stop_server EXIT` carries in the bash harness.
function teardown() {
	if (is_file(pid_file())) {
		$pid = trim(file_get_contents(pid_file()));
		if ($pid !== "") {
			exec("kill " . $pid . " 2>/dev/null");
			usleep(300000);
		}

		unlink(pid_file());
	}

	if (is_file(profile_file())) {
		$profile = trim(file_get_contents(profile_file()));
		if ($profile !== "") {
			exec("docker compose -f testdata/php-vanilla/compose.yml --profile " . $profile . " down >/dev/null 2>&1");
		}

		unlink(profile_file());
	}
}

// environments describes each test environment: its directory, how it
// starts, and where its address comes from once it has one.
function environments() {
	return array(
		"goserver" => array(
			"kind" => "logged",
			"command" => "bin/testserver-go",
			"pattern" => "|serving on http://([^ ]+) |",
		),
		"phpscript-http-bridge" => array(
			"kind" => "logged",
			"command" => "phpscript run testdata/phpscript-http-bridge/server.php",
			"pattern" => "|serving on http://([^ ]+) |",
		),
		"phpscript-service" => array(
			"kind" => "logged",
			"command" => "phpscript server -f bin/benchmark-service.yml testdata/phpscript-service",
			"pattern" => "|url=http://([^ \n]+)|",
		),
		"php-vanilla-apache" => array(
			"kind" => "compose",
			"profile" => "apache",
			"port" => "8098",
			"containers" => "phpscript-php-vanilla-apache",
		),
		"php-vanilla-nginx" => array(
			"kind" => "compose",
			"profile" => "nginx",
			"port" => "8099",
			"containers" => "phpscript-php-vanilla-nginx phpscript-php-vanilla-fpm",
		),
	);
}

// endpoints is the API every environment answers, route for route and byte for
// byte, and one column compares against another. A --endpoints list replaces it
// with paths of the caller's choosing, each named for its column.
function endpoints($list) {
	if ($list !== "") {
		$chosen = array();
		foreach (explode(",", $list) as $pair) {
			$parts = explode("=", $pair, 2);
			if (count($parts) !== 2) {
				fail("--endpoints takes name=path pairs, read: " . $pair);
			}

			array_push($chosen, array(
				"name" => $parts[0],
				"path" => $parts[1],
				"lua" => "",
			));
		}

		return $chosen;
	}

	return array(
		array(
			"name" => "hello",
			"path" => "/hello?name=sprint",
			"lua" => "",
		),
		array(
			"name" => "users",
			"path" => "/users/42",
			"lua" => "",
		),
		array(
			"name" => "echo",
			"path" => "/echo",
			"lua" => "scripts/bench-http-post.lua",
		),
		array(
			"name" => "index",
			"path" => "/",
			"lua" => "",
		),
	);
}

// flags reads --key value and --key=value over the defaults.
function flags($argv, $defaults) {
	$settings = $defaults;
	$count = count($argv);
	for ($i = 1; $i < $count; $i++) {
		$argument = $argv[$i];
		if (!str_starts_with($argument, "--")) {
			fail("unexpected argument: " . $argument);
		}

		$name = substr($argument, 2);
		$value = "";
		if (str_contains($name, "=")) {
			$parts = explode("=", $name, 2);
			$name = $parts[0];
			$value = $parts[1];
		} else {
			$i++;
			if ($i >= $count) {
				fail("--" . $name . " takes a value");
			}

			$value = $argv[$i];
		}

		if (!array_key_exists($name, $defaults)) {
			fail("unknown flag: --" . $name);
		}

		$settings[$name] = $value;
	}

	return $settings;
}

// micros reads one wrk latency. wrk picks the unit per line, and each line's
// suffix decides the scale.
function micros($token) {
	if (str_ends_with($token, "us")) {
		return substr($token, 0, strlen($token) - 2) + 0;
	}

	if (str_ends_with($token, "ms")) {
		return (substr($token, 0, strlen($token) - 2) + 0) * 1000;
	}

	if (str_ends_with($token, "s")) {
		return (substr($token, 0, strlen($token) - 1) + 0) * 1e+06;
	}

	return $token + 0;
}

// segment runs wrk once and reduces its report to the six numbers a row
// carries, latencies in microseconds.
function segment($url, $threads, $conc, $duration, $lua) {
	$command = "nice -n -20 wrk --latency -t " . $threads . " -c " . $conc . " --timeout 30s -d " . $duration;
	if ($lua !== "") {
		$command = $command . " -s " . $lua;
	}

	$report = shell_exec($command . " " . escapeshellarg($url) . " 2>&1");
	if ($report === null) {
		fail("wrk wrote nothing for " . $url);
	}

	$row = array(
		"p50" => 0,
		"p90" => 0,
		"p99" => 0,
		"rps" => 0,
		"non2xx" => 0,
		"errors" => 0,
	);
	$distribution = false;
	foreach (explode("\n", $report) as $line) {
		$fields = explode(" ", trim(preg_replace("/\s+/", " ", $line)));
		if (str_contains($line, "Latency Distribution")) {
			$distribution = true;
			continue;
		}

		if ($distribution && $fields[0] === "50%") {
			$row["p50"] = micros($fields[1]);
		}

		if ($distribution && $fields[0] === "90%") {
			$row["p90"] = micros($fields[1]);
		}

		if ($distribution && $fields[0] === "99%") {
			$row["p99"] = micros($fields[1]);
			$distribution = false;
		}

		if ($fields[0] === "Requests/sec:") {
			$row["rps"] = $fields[1] + 0;
		}

		if (str_contains($line, "Non-2xx or 3xx responses:")) {
			$row["non2xx"] = $fields[count($fields) - 1] + 0;
		}

		// "Socket errors: connect 0, read 0, write 0, timeout 0", printed only
		// when one of the four is not zero. Every numeric field is one of them.
		if (str_contains($line, "Socket errors:")) {
			foreach ($fields as $field) {
				if (preg_match("/^[0-9]+/", $field) === 1) {
					$row["errors"] = $row["errors"] + ($field + 0);
				}
			}
		}
	}

	return $row;
}

// median picks the 50th percentile the way the bash harness picks it, and a row
// here and a row there hold the same number.
function median($values) {
	$count = count($values);
	if ($count === 0) {
		return 0;
	}

	sort($values);
	$index = intval(0.5 * $count + 0.5);
	if ($index < 1) {
		$index = 1;
	}

	if ($index > $count) {
		$index = $count;
	}

	return $values[$index - 1];
}

// collapse reduces a set of segments to one row: the median of each latency
// column and of the request rate, and the sum of what went wrong.
function collapse($segments) {
	$p50 = array();
	$p90 = array();
	$p99 = array();
	$rps = array();
	$non2xx = 0;
	$errors = 0;
	foreach ($segments as $one) {
		array_push($p50, $one["p50"]);
		array_push($p90, $one["p90"]);
		array_push($p99, $one["p99"]);
		array_push($rps, $one["rps"]);
		$non2xx = $non2xx + $one["non2xx"];
		$errors = $errors + $one["errors"];
	}

	return array(
		"p50" => median($p50),
		"p90" => median($p90),
		"p99" => median($p99),
		"rps" => median($rps),
		"non2xx" => $non2xx,
		"errors" => $errors,
	);
}

// address polls a server's log until it reports where it listens. A kernel-
// picked port keeps two runs on one box from colliding, and the log is the only
// place the port exists.
function address($log, $pattern) {
	for ($i = 0; $i < 200; $i++) {
		if (is_file($log)) {
			$found = array();
			if (preg_match($pattern, file_get_contents($log), $found) === 1) {
				return $found[1];
			}
		}

		usleep(50000);
	}

	return "";
}

// start_logged runs one of the three servers this repository builds, in the
// background, and answers the address it printed.
function start_logged($environment, $settings, $log) {
	if ($environment["command"] === "bin/testserver-go") {
		exec("CGO_ENABLED=0 go build -o bin/testserver-go testdata/goserver/main.go", $output, $code);
		if ($code !== 0) {
			fail("the Go floor did not build: " . implode("\n", $output));
		}
	}

	if (str_contains($environment["command"], "phpscript server")) {
		file_put_contents("bin/benchmark-service.yml", "runner:\n  work_dir: \".\"\nroutes:\n  enabled: true\nserver:\n  addr: \"127.0.0.1:0\"\n  quiet: false\n");
	}

	$environments = "TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=" . $settings["workers"] . " TESTSERVER_LIMIT=" . $settings["limit"];
	$pid = trim(shell_exec($environments . " nice -n -20 " . $environment["command"] . " >" . $log . " 2>&1 & echo $!"));

	// Recorded before the wait below, because a server that starts and never
	// reports an address is the one most worth taking down.
	file_put_contents(pid_file(), $pid);

	$addr = address($log, $environment["pattern"]);
	if ($addr === "") {
		fail("no address from " . $environment["command"] . ":\n" . file_get_contents($log));
	}

	return array("addr" => $addr, "pid" => $pid);
}

// start_compose brings up one php-vanilla profile and renices every process in
// it. The other environments run at nice -n -20, and a default-priority server
// on a shared box loses the CPU for milliseconds at a time.
function start_compose($environment, $settings) {
	$prefix = "VANILLA_PORT=8098 VANILLA_NGINX_PORT=8099 VANILLA_WORKERS=" . $settings["workers"] . " VANILLA_JIT=" . $settings["jit"] . " ";
	$compose = $prefix . "docker compose -f testdata/php-vanilla/compose.yml --profile " . $environment["profile"];

	// Recorded before the stack comes up, and a profile that half starts is
	// still taken down.
	file_put_contents(profile_file(), $environment["profile"]);

	exec($compose . " up -d --wait --wait-timeout 180 2>&1", $output, $code);
	if ($code !== 0) {
		fail("the " . $environment["profile"] . " stack did not come up: " . implode("\n", $output));
	}

	foreach (explode(" ", $environment["containers"]) as $container) {
		exec("docker top " . $container . " -eo pid 2>/dev/null | tail -n +2", $pids, $ignored);
		foreach ($pids as $pid) {
			exec("renice -n -20 -p " . trim($pid) . " >/dev/null 2>&1");
		}
	}

	return array("addr" => "127.0.0.1:" . $environment["port"], "pid" => "");
}

// table prints rows under headers, every column as wide as its widest cell.
// The first columns name a thing and read left, the rest are numbers and read
// right, and a digit lines up under a digit.
function table($title, $headers, $named, $rows) {
	$widths = array();
	foreach ($headers as $column => $head) {
		$widths[$column] = strlen($head);
	}

	foreach ($rows as $row) {
		foreach ($row as $column => $cell) {
			if (strlen($cell) > $widths[$column]) {
				$widths[$column] = strlen($cell);
			}
		}
	}

	echo "\n", $title, "\n\n";
	$line = "";
	foreach ($headers as $column => $head) {
		$pad = $column < $named ? STR_PAD_RIGHT : STR_PAD_LEFT;
		$line = $line . str_pad($head, $widths[$column], " ", $pad) . "  ";
	}

	echo rtrim($line), "\n";
	foreach ($rows as $row) {
		$line = "";
		foreach ($row as $column => $cell) {
			$pad = $column < $named ? STR_PAD_RIGHT : STR_PAD_LEFT;
			$line = $line . str_pad($cell, $widths[$column], " ", $pad) . "  ";
		}

		echo rtrim($line), "\n";
	}
}

register_shutdown_function("teardown");

if (!is_file("go.mod")) {
	fail("run this from the repository root, where the testdata paths resolve");
}

if (trim(shell_exec("command -v wrk || true")) === "") {
	fail("wrk is not on PATH");
}

$settings = flags($argv, $FLAGS);
$known = environments();
$routes = endpoints($settings["endpoints"]);
$perEndpoint = array();
$perRuntime = array();

foreach (explode(",", $settings["targets"]) as $name) {
	if (!array_key_exists($name, $known)) {
		fail("unknown target: " . $name . ", one of " . implode(", ", array_keys($known)));
	}

	$environment = $known[$name];
	// `.gitignore` holds bin/, and it resolves the same for both sides of every call
	// here: a relative path reads inside the runtime's root, and the shell a
	// command runs through starts in that same directory.
	$log = "bin/benchmark-" . $name . ".log";

	fwrite(STDERR, "benchmark: " . $name . " up\n");
	$server = $environment["kind"] === "compose" ? start_compose($environment, $settings) : start_logged($environment, $settings, $log);

	foreach (explode(",", $settings["concurrency"]) as $conc) {
		// wrk takes no more threads than connections, and one thread saturates
		// a handler answering in tens of microseconds.
		$threads = $conc + 0 > 2 ? 2 : $conc + 0;
		$runtime = $name . "-" . $conc;
		$all = array();
		foreach ($routes as $route) {
			$url = "http://" . $server["addr"] . $route["path"];

			fwrite(STDERR, "benchmark: " . $runtime . " " . $route["name"] . "\n");
			segment($url, $threads, $conc, $settings["warmup"], $route["lua"]);
			$segments = array();
			for ($i = 0; $i < $settings["segments"] + 0; $i++) {
				array_push($segments, segment($url, $threads, $conc, $settings["duration"], $route["lua"]));
				array_push($all, $segments[count($segments) - 1]);
			}

			$row = collapse($segments);

			array_push($perEndpoint, array(
				$runtime,
				$route["name"],
				sprintf("%.1f", $row["p50"]),
				sprintf("%.1f", $row["p90"]),
				sprintf("%.1f", $row["p99"]),
				sprintf("%.0f", $row["rps"]),
				sprintf("%d", $row["non2xx"]),
				sprintf("%d", $row["errors"]),
			));
		}

		$summary = collapse($all);

		array_push($perRuntime, array(
			$runtime,
			sprintf("%.1f", $summary["p50"]),
			sprintf("%.1f", $summary["p90"]),
			sprintf("%.1f", $summary["p99"]),
			sprintf("%.0f", $summary["rps"]),
			sprintf("%d", $summary["non2xx"]),
			sprintf("%d", $summary["errors"]),
		));
	}

	teardown();
}

table("Per endpoint, each row the median of its segments.", array("runtime", "endpoint", "p50 us", "p90 us", "p99 us", "req/s", "non-2xx", "errors"), 2, $perEndpoint);
table("Per runtime, every segment of every endpoint collapsed by the same rule.", array("runtime", "p50 us", "p90 us", "p99 us", "req/s", "non-2xx", "errors"), 1, $perRuntime);
