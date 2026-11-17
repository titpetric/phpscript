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

import "net/http"
import "encoding/json"
import "os"
import "time"
import "fmt"

class Server {
	// An unset counter reads as int("")'s zero, and the check after it names the default.
	function config(): array {
		$addr = getenv("TESTSERVER_ADDR") ?? "127.0.0.1:8099"
		$workers = int(getenv("TESTSERVER_WORKERS"))
		if ($workers < 1) {
			$workers = numcpu()
		}
		if ($workers < 2) {
			$workers = 2
		}

		$queue = int(getenv("TESTSERVER_QUEUE"))
		if ($queue < 1) {
			$queue = 64
		}

		return {
			"addr": $addr,
			"workers": $workers,
			"queue": $queue,
		}
	}

	// $this->index is the method read without parentheses, a callable bound to
	// this object, and handle() registers it.
	function mount(): http.ServeMux {
		$mux = new http.ServeMux

		// '{$}' anchors the root; a bare 'GET /' is a ServeMux subtree.
		$mux->handle('GET /{$}', $this->index)
		$mux->handle("GET /hello", $this->hello)
		$mux->handle("GET /users/{id}", $this->showUser)
		$mux->handle("POST /echo", $this->echoRequest)
		$mux->handle("GET /slow", $this->slow)
		$mux->handle("GET /info", $this->info)
		return $mux
	}

	// The route table is one template call: ob_start() captures the echoed
	// fragment and the page echoes what it returned.
	function index(http.ResponseWriter $w, *http.Request $r): void {
		header("Content-Type: text/html; charset=utf-8")

		$routes = [
			{"path": "/", "what": "this page, the root anchor"},
			{"path": "/hello", "what": "a greeting, with an optional name"},
			{"path": "/users/{id}", "what": "reads a path value"},
			{"path": "/echo", "what": "reports the request it was given"},
			{"path": "/slow", "what": "holds the connection, and notices if you leave"},
			{"path": "/info", "what": "reports the runtime"},
		]

		ob_start()
		?>
		<ul>
		<?php foreach ($routes as $route) { ?>
			<li><code><?php echo $route["path"]; ?></code> - <?php echo $route["what"]; ?></li>
		<?php } ?>
		</ul>
		<?php
		$table = ob_get_clean()
		?>
		<!doctype html>
		<html lang="en">
		<head>
			<meta charset="utf-8">
			<title>phpscript go runtime test server</title>
		</head>
		<body>
			<h1>phpscript go runtime test server</h1>
			<p>Every route is a method on one class, mounted on a <code>net/http</code> mux. The root pattern is <code>GET /{$}</code>.</p>
		<?php echo $table; ?>
		</body>
		</html>
		<?php
	}

	function hello(http.ResponseWriter $w, *http.Request $r): void {
		header("Content-Type: text/plain; charset=utf-8")
		$name = $r->url->query()->get("name")
		if ($name == "") {
			$name = "world"
		}
		echo "hello " . $name . "\n"
	}

	function showUser(http.ResponseWriter $w, *http.Request $r): void {
		header("Content-Type: application/json")
		json.NewEncoder($w)->encode({"id": $r->pathValue("id"), "path": $r->url->path})
	}

	function echoRequest(http.ResponseWriter $w, *http.Request $r): void {
		header("Content-Type: application/json")
		json.NewEncoder($w)->encode({
			"method": $r->method,
			"path": $r->url->path,
			"query": $r->url->rawQuery,
			"user_agent": $r->userAgent(),
			"name": $r->formValue("name"),
		})
	}

	// The ticks finish either way: the work is the transaction that already
	// committed, and only the response nobody waits for is dropped.
	function slow(http.ResponseWriter $w, *http.Request $r): void {
		http.IgnoreUserAbort()

		$ticks = 0
		while ($ticks < 20) {
			time.Sleep("100ms")
			$ticks++
			if (http.Aborted($r)) {
				break
			}
		}

		if (http.Aborted($r)) {
			fmt.Fprintf(os.Stderr, "aborted after %d ticks, response skipped\n", $ticks)
			return
		}

		header("Content-Type: text/plain; charset=utf-8")
		echo "waited " . $ticks . " ticks, still connected\n"
	}

	function info(http.ResponseWriter $w, *http.Request $r): void {
		header("Content-Type: application/json")
		json.NewEncoder($w)->encode({
			"sapi": "go",
			"method": $r->method,
			"path": $r->url->path,
			"host": $r->host,
		})
	}

	function run(): void {
		$config = $this->config()
		$http = new http.Server($config["addr"], $this->mount(), $config["workers"], $config["queue"])
		$bound = $http->listen()

		echo "serving on http://" . $bound . " until the process is signalled\n"

		// Returns when the host context ends, which is Ctrl-C, SIGTERM or a
		// reload. The close after it lets the requests in flight finish.
		$http->wait()
		$http->close()
	}
}

$server = new Server
$server->run()
