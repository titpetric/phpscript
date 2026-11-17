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
import "time";
import "fmt";

class Server {
	// An unset counter reads as int("")'s zero, and the check after it names the default.
	function config(): array {
		$addr = getenv("TESTSERVER_ADDR");
		if ($addr === "") {
			$addr = "127.0.0.1:8099";
		}

		$workers = int(getenv("TESTSERVER_WORKERS"));
		if ($workers < 1) {
			$workers = numcpu();
		}
		if ($workers < 2) {
			$workers = 2;
		}

		$queue = int(getenv("TESTSERVER_QUEUE"));
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
	function mount(): http.ServeMux {
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

	// The route table is one template call: ob_start() captures the echoed
	// fragment and the page echoes what it returned.
	function index(http.ResponseWriter $w, *http.Request $r): void {
		$w->Header()->Set("Content-Type", "text/html; charset=utf-8");

		$routes = array(
			array("path" => "/", "what" => "this page, the root anchor"),
			array("path" => "/hello", "what" => "a greeting, with an optional name"),
			array("path" => "/users/{id}", "what" => "reads a path value"),
			array("path" => "/echo", "what" => "reports the request it was given"),
			array("path" => "/slow", "what" => "holds the connection, and notices if you leave"),
			array("path" => "/info", "what" => "reports the runtime"),
		);

		ob_start();
		?>
		<ul>
		<?php foreach ($routes as $route) { ?>
			<li><code><?php echo $route["path"]; ?></code> - <?php echo $route["what"]; ?></li>
		<?php } ?>
		</ul>
		<?php
		$table = ob_get_clean();
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
		$w->Header()->Set("Content-Type", "text/plain; charset=utf-8");
		$name = $r->URL->Query()->Get("name");
		if ($name === "") {
			$name = "world";
		}
		echo "hello " . $name . "\n";
	}

	function showUser(http.ResponseWriter $w, *http.Request $r): void {
		$w->Header()->Set("Content-Type", "application/json");
		json.NewEncoder($w)->Encode(array("id" => $r->PathValue("id"), "path" => $r->URL->Path));
	}

	function echoRequest(http.ResponseWriter $w, *http.Request $r): void {
		$w->Header()->Set("Content-Type", "application/json");
		json.NewEncoder($w)->Encode(array(
			"method" => $r->Method,
			"path" => $r->URL->Path,
			"query" => $r->URL->RawQuery,
			"user_agent" => $r->UserAgent(),
			"name" => $r->FormValue("name"),
		));
	}

	// The ticks finish either way: the work is the transaction that already
	// committed, and only the response nobody waits for is dropped.
	function slow(http.ResponseWriter $w, *http.Request $r): void {
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
		echo "waited " . $ticks . " ticks, still connected\n";
	}

	function info(http.ResponseWriter $w, *http.Request $r): void {
		$w->Header()->Set("Content-Type", "application/json");
		json.NewEncoder($w)->Encode(array(
			"sapi" => "go",
			"method" => $r->Method,
			"path" => $r->URL->Path,
			"host" => $r->Host,
		));
	}

	function run(): void {
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
