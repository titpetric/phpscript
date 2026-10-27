<?php

// A PHP program that is itself the HTTP server: HTTP\Mux is net/http's
// ServeMux with PHP callables as its handlers, and HTTP\Server::listen forks
// the workers.
//
// A handler starts on a clean stack holding $w and $r. The runtime decodes no
// superglobal, nothing carries over from the last request, and two requests
// answered at the same moment run on a runtime each, and neither can see the
// other.
//
//	phpscript run testdata/phpscript-http-bridge/server.php
//
// TESTSERVER_ADDR, TESTSERVER_WORKERS and TESTSERVER_LIMIT override the
// address, how many requests the server answers at once, and how long the script
// runs. Ctrl-C stops it; SIGHUP ends the generation running and reads the file
// again.
class Server {
	function config() {
		$addr = getenv("TESTSERVER_ADDR");
		if ($addr === false || $addr === "") {
			$addr = "127.0.0.1:8099";
		}

		$workers = intval(getenv("TESTSERVER_WORKERS"));
		if ($workers < 1) {
			$workers = 4;
		}

		$limit = intval(getenv("TESTSERVER_LIMIT"));
		if ($limit < 1) {
			$limit = 10;
		}

		return array(
			"addr" => $addr,
			"workers" => $workers,
			"queue" => 64,
			"limit" => $limit,
		);
	}

	// $this->index is the method below read without parentheses, which is a
	// callable bound to this object, and that is the whole of the
	// registration.
	function mount() {
		$mux = new HTTP\Mux;

		// '{$}' anchors the root. A bare 'GET /' is a net/http subtree and
		// would answer for every path no other route claimed.
		$mux->handle('GET /{$}', $this->index);
		$mux->handle("GET /hello", $this->hello);
		$mux->handle("GET /users/{id}", $this->showUser);
		$mux->handle("POST /echo", $this->echoRequest);
		$mux->handle("GET /slow", $this->slow);
		$mux->handle("GET /info", $this->info);
		return $mux;
	}

	function index(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		$w->header()->set("Content-Type", "text/plain; charset=utf-8");
		// echo reaches the response: the runtime answering the request writes
		// there for the length of the call.
		echo "phpscript test server\n\n";
		echo "GET  /             this page ('{\$}' anchors it; a bare\n";
		echo "                   'GET /' is a net/http subtree and would\n";
		echo "                   answer for every path nothing else claimed)\n";
		echo "GET  /hello        a greeting\n";
		echo "GET  /users/{id}   reads a path value\n";
		echo "POST /echo         reports the request it was given\n";
		echo "GET  /slow         holds the connection, and notices if you leave\n";
		echo "GET  /info         phpinfo()\n";
	}

	function hello(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		$w->header()->set("Content-Type", "text/plain; charset=utf-8");
		// query() is net/http's url.Values, and get() answers the first value
		// under a name, or "" for a name that is not there. form_value() is
		// the same answer over the query and a form body together.
		$name = $r->url->query()->get("name");
		if ($name === "") {
			$name = "world";
		}

		$w->write("hello " . $name . "\n");
	}

	// The encoder writes straight to the response with no string of the
	// document between the two, and it converts a PHP array the way
	// json_encode() does.
	function showUser(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		$w->header()->set("Content-Type", "application/json");
		(new JSON\Encoder($w))->encode(array("id" => $r->path_value("id"), "path" => $r->url->path));
	}

	// form_value reads the query and the body, parsing the body on the first
	// call, and parse_form() is not needed first.
	function echoRequest(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		$w->header()->set("Content-Type", "application/json");
		(new JSON\Encoder($w))->encode(array(
			"method" => $r->method,
			"path" => $r->url->path,
			"query" => $r->url->rawquery,
			"user_agent" => $r->user_agent(),
			"name" => $r->form_value("name"),
		));
	}

	// What connection_aborted() is for. The work finishes, because in an
	// application that work is the transaction that already committed; the
	// part this drops is the part nobody remains to read. ignore_user_abort(true)
	// keeps the handler running long enough to make that choice, and without
	// it the disconnect ends the handler where it next looks.
	function slow(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		ignore_user_abort(true);

		$ticks = 0;
		while ($ticks < 20) {
			usleep(100000);
			$ticks++;
			if (connection_aborted()) {
				break;
			}
		}

		if (connection_aborted()) {
			// This writes nothing to the response: nobody remains to read it.
			// error_log puts the line on the request's trace and through the
			// host's error handler.
			error_log("aborted after " . $ticks . " ticks, response skipped");
			return;
		}

		$w->header()->set("Content-Type", "text/plain; charset=utf-8");
		$w->write("waited " . $ticks . " ticks, still connected\n");
	}

	function info(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
		$w->header()->set("Content-Type", "text/markdown; charset=utf-8");
		phpinfo();
	}

	function run() {
		$config = $this->config();

		set_time_limit($config["limit"]);

		// The third argument is how many requests the server answers at once and the
		// fourth is how deep the queue behind them is; omitted they are the
		// number of cores and 1024. Workers are the parallelism and the queue
		// is the backpressure: a request that finds every worker busy waits
		// its turn.
		$http = new HTTP\Server($config["addr"], $this->mount(), $config["workers"], $config["queue"]);

		$http->listen();

		// Stopping the server is the script's own business. The shutdown pass
		// runs with the clock off, and this still runs when the time limit is
		// what ended the script.
		register_shutdown_function(function() use ($http) {
			$http->shutdown();
			echo "server stopped\n";
		});

		echo "serving on http://" . $http->addr() . " until the time limit is up\n";

		// Returns when the script runs out of time, and stands last in the
		// file: the limit is a fatal, as php's is, and anything after it
		// would leave the process exiting non-zero. Ending here lets the
		// shutdown callback have the last word and the process exit 0.
		$http->wait();
	}
}

$server = new Server;

$server->run();
