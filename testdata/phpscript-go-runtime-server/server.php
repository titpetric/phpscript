<?php

// A server on the go runtime namespace: phpscript.yml sets runtime: go, so no
// file writes `use Runtime\Go;`, and the source imports go's packages and uses
// go's request and response types.
//
//	phpscript -f testdata/phpscript-go-runtime-server/phpscript.yml \
//	    run testdata/phpscript-go-runtime-server/server.php
//
// TESTSERVER_ADDR and TESTSERVER_WORKERS override the address and how many
// requests the server answers at once. Ctrl-C stops it; SIGHUP reads the file
// again. There is no time limit under this runtime: the process runs until it
// is signalled.

import "net/http";
import "encoding/json";
import "os";
import "strconv";
import "time";
import "fmt";

class Server {
	function env_int($name, $fallback) {
		$raw = os.Getenv($name);
		if ($raw === "") {
			return $fallback;
		}
		return strconv.Atoi($raw);
	}

	function config() {
		$addr = os.Getenv("TESTSERVER_ADDR");
		if ($addr === "") {
			$addr = "127.0.0.1:8099";
		}

		$workers = $this->env_int("TESTSERVER_WORKERS", 4);
		if ($workers < 1) {
			$workers = 4;
		}

		$queue = $this->env_int("TESTSERVER_QUEUE", 64);
		if ($queue < 1) {
			$queue = 64;
		}

		return array(
			"addr" => $addr,
			"workers" => $workers,
			"queue" => $queue,
		);
	}

	// $this->index is the method read without parentheses, a callable bound to
	// this object, and handle() registers it.
	function mount() {
		$mux = new http.ServeMux;

		// '{$}' anchors the root; a bare 'GET /' is a ServeMux subtree.
		$mux->Handle('GET /{$}', $this->index);
		$mux->Handle("GET /hello", $this->hello);
		$mux->Handle("GET /users/{id}", $this->showUser);
		$mux->Handle("POST /echo", $this->echoRequest);
		$mux->Handle("GET /slow", $this->slow);
		$mux->Handle("GET /info", $this->info);
		return $mux;
	}

	function index($w, $r) {
		$w->Header()->Set("Content-Type", "text/plain; charset=utf-8");
		$w->Write("phpscript go runtime test server\n\n");
		$w->Write("GET  /             this page ('{\$}' anchors it)\n");
		$w->Write("GET  /hello        a greeting\n");
		$w->Write("GET  /users/{id}   reads a path value\n");
		$w->Write("POST /echo         reports the request it was given\n");
		$w->Write("GET  /slow         holds the connection, and notices if you leave\n");
		$w->Write("GET  /info         reports the runtime\n");
	}

	function hello($w, $r) {
		$w->Header()->Set("Content-Type", "text/plain; charset=utf-8");
		$name = $r->URL->Query()->Get("name");
		if ($name === "") {
			$name = "world";
		}
		$w->Write("hello " . $name . "\n");
	}

	function showUser($w, $r) {
		$w->Header()->Set("Content-Type", "application/json");
		$body = json.Marshal(array("id" => $r->PathValue("id"), "path" => $r->URL->Path));
		$w->Write($body . "\n");
	}

	function echoRequest($w, $r) {
		$w->Header()->Set("Content-Type", "application/json");
		$body = json.Marshal(array(
			"method" => $r->Method,
			"path" => $r->URL->Path,
			"query" => $r->URL->RawQuery,
			"user_agent" => $r->UserAgent(),
			"name" => $r->FormValue("name"),
		));
		$w->Write($body . "\n");
	}

	// The ticks finish either way: the work is the transaction that already
	// committed, and only the response nobody waits for is dropped.
	function slow($w, $r) {
		http.IgnoreUserAbort();

		$ticks = 0;
		while ($ticks < 20) {
			time.Sleep("100ms");
			$ticks++;
			if (http.Aborted($r)) {
				break;
			}
		}

		if (http.Aborted($r)) {
			fmt.Fprintf(os.Stderr, "aborted after %d ticks, response skipped\n", $ticks);
			return;
		}

		$w->Header()->Set("Content-Type", "text/plain; charset=utf-8");
		$w->Write("waited " . $ticks . " ticks, still connected\n");
	}

	function info($w, $r) {
		$w->Header()->Set("Content-Type", "application/json");
		$body = json.Marshal(array(
			"sapi" => "go",
			"method" => $r->Method,
			"path" => $r->URL->Path,
			"host" => $r->Host,
		));
		$w->Write($body . "\n");
	}

	function run() {
		$config = $this->config();

		$http = new http.Server($config["addr"], $this->mount(), $config["workers"], $config["queue"]);
		$bound = $http->Listen();

		echo "serving on http://" . $bound . " until the process is signalled\n";

		// Returns when the host context ends, which is Ctrl-C, SIGTERM or a
		// reload. The close after it lets the requests in flight finish.
		$http->Wait();
		$http->Close();
	}
}

$server = new Server;

$server->run();
