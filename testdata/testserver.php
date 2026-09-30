<?php

// A PHP program that is the HTTP server rather than something a server runs.
//
// It is shaped the way a Go server is. config() reads the environment into one
// value and returns it. Each handler is a closure held in a property and
// registered as $this->fnName, which is what s.handleX is over there. routes()
// builds the router and returns it. run() is main(): it holds what it built in
// locals.
//
// A handler takes its state from its arguments. It starts on a clean stack
// holding $w and $r and nothing else, no superglobals are decoded, nothing
// carries over from the last request, and two requests answered at the same
// moment cannot see each other, because each runs on a runtime of its own. A
// closure copies what its use (...) clause names and nothing else, so what a
// handler can reach is what it asked for.
//
// HTTP\Server's third argument is how many requests are answered at once and
// its fourth is how deep the queue behind them is; omitted they are the number
// of cores and 1024. Workers are the parallelism, the queue is the
// backpressure: a request that finds every worker busy waits its turn rather
// than starting a runtime of its own.
//
// Ctrl-C stops the server. SIGHUP reloads it: the generation running is ended,
// its shutdown callback stops it listening, and the file is read again.
//
// set_time_limit ends the whole thing. The limit is a deadline on the context
// the interpreter checks and every binding is handed, and wait() is a binding,
// so the script runs out of time while it is parked there and the shutdown
// callback stops the server. Ten seconds, which is long enough to answer a few
// requests by hand and short enough that a forgotten one does not outlive the
// terminal it started in.
//
// Run it with `phpscript run testdata/testserver.php`, against a port the
// system picks with TESTSERVER_ADDR=127.0.0.1:0, and for longer than ten
// seconds with TESTSERVER_LIMIT. TESTSERVER_WORKERS changes how many requests
// are answered at once, which is what a load test sweeps.

class Server {
	// The handlers, each registered under the name of the property holding it.
	public $index;
	public $hello;
	public $showUser;
	public $echoRequest;
	public $slow;
	public $info;

	// __construct is where the handlers are written, so routes() below reads
	// like the registration it is and nothing builds them twice.
	function __construct() {
		$this->index = $this->indexHandler();
		$this->hello = $this->helloHandler();
		$this->showUser = $this->showUserHandler();
		$this->echoRequest = $this->echoRequestHandler();
		$this->slow = $this->slowHandler();
		$this->info = $this->infoHandler();
	}

	// config reads the environment into the one value everything else is built
	// from, before anything is built.
	function config() {
		$addr = getenv("TESTSERVER_ADDR");
		if ($addr === false || $addr === "") {
			$addr = "127.0.0.1:8099";
		}

		// Four workers answer at once; anything else waits in a queue of 64. A
		// handler here is well under a millisecond, so the queue is depth
		// rather than latency: it is what a burst waits in instead of starting
		// a runtime of its own.
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

	// routes builds the router and returns it.
	function routes() {
		$mux = new HTTP\Mux();
		$mux->handle('GET /{$}', $this->index);
		$mux->handle("GET /hello", $this->hello);
		$mux->handle("GET /users/{id}", $this->showUser);
		$mux->handle("POST /echo", $this->echoRequest);
		$mux->handle("GET /slow", $this->slow);
		$mux->handle("GET /info", $this->info);
		return $mux;
	}

	function indexHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "text/plain; charset=utf-8");
			// A handler's echo reaches the response: the runtime answering the
			// request writes there for the length of the call.
			echo "phpscript test server\n\n";
			echo "GET  /             this page ('{\$}' anchors it; a bare\n";
			echo "                   'GET /' is a net/http subtree and would\n";
			echo "                   answer for every path nothing else claimed)\n";
			echo "GET  /hello        a greeting\n";
			echo "GET  /users/{id}   reads a path value\n";
			echo "POST /echo         reports the request it was given\n";
			echo "GET  /slow         holds the connection, and notices if you leave\n";
			echo "GET  /info         phpinfo()\n";
		};
	}

	function helloHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "text/plain; charset=utf-8");
			// query() is net/http's url.Values, so ->get() answers the first
			// value under a name, or "" for a name that is not there.
			// $r->form_value($name) is the same answer over the query and a
			// form body together.
			$name = $r->url->query()->get("name");
			if ($name === "") {
				$name = "world";
			}
			$w->write("hello " . $name . "\n");
		};
	}

	// {id} is a net/http pattern segment, read back through the request. The
	// encoder writes straight to the response: there is no string of the
	// document in between, and it converts a PHP array the way json_encode()
	// does.
	function showUserHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "application/json");
			(new JSON\Encoder($w))->encode(array(
				"id" => $r->path_value("id"),
				"path" => $r->url->path,
			));
		};
	}

	function echoRequestHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "application/json");
			(new JSON\Encoder($w))->encode(array(
				"method" => $r->method,
				"path" => $r->url->path,
				"query" => $r->url->rawquery,
				"user_agent" => $r->user_agent(),
				// form_value reads the query and the body, and parses the body
				// on the first call, so parse_form() is not needed first.
				"name" => $r->form_value("name"),
			));
		};
	}

	// What connection_aborted() is for. The work is allowed to finish, because
	// in a real application it is the transaction that already committed; what
	// is skipped is the part nobody is left to read. ignore_user_abort(true) is
	// what keeps the handler running long enough to make that choice, because
	// without it the disconnect ends the handler where it next looks.
	function slowHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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
				// Nothing is written to the response: there is nobody to write
				// it to. error_log puts this on the request's trace and through
				// the host's error handler, which is where a served script says
				// something went wrong.
				error_log("aborted after " . $ticks . " ticks, response skipped");
				return;
			}

			$w->header()->set("Content-Type", "text/plain; charset=utf-8");
			$w->write("waited " . $ticks . " ticks, still connected\n");
		};
	}

	function infoHandler() {
		return function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "text/markdown; charset=utf-8");
			phpinfo();
		};
	}

	// run is main().
	function run() {
		$config = $this->config();
		set_time_limit($config["limit"]);

		$http = new HTTP\Server($config["addr"], $this->routes(), $config["workers"], $config["queue"]);
		$http->listen();

		// Stopping the server is the script's own business, so it is written
		// here rather than left to whatever ends the script. The shutdown pass
		// runs with the clock off, which is what makes this still run when the
		// time limit is what ended us. use ($http) copies the one value the
		// callback needs and nothing else.
		register_shutdown_function(function () use ($http) {
			$http->shutdown();
			echo "server stopped\n";
		});

		echo "serving on http://" . $http->addr() . " until the time limit is up\n";

		// Returns when the script runs out of time, which is the whole of the
		// run. It is the last statement on purpose: the limit is a fatal, as
		// php's is, so anything after it would not run and the process would
		// exit non-zero. Ending here lets the shutdown callback have the last
		// word and the process exit 0.
		$http->wait();
	}
}

$server = new Server;
$server->run();
