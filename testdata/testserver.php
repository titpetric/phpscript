<?php

// A PHP program that is the HTTP server rather than something a server runs.
//
// The server is a class. mount() builds the routes, run() is the entrypoint,
// and each handler is a closure held in a property, registered as
// $this->fnName. A handler takes its state from its arguments: it starts on a
// clean stack holding $w and $r and nothing else, no superglobals are decoded,
// nothing carries over from the last request, and two requests answered at the
// same moment cannot see each other, because each runs on a runtime of its own.
//
// What a handler captures - $this, and anything a use (...) clause names - is
// shared by every request, the way a Go handler closing over its configuration
// is. Read it; do not write to it.
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
	public $addr;
	public $workers;
	public $queue;
	public $limit;

	public $mux;
	public $http;

	public $index;
	public $hello;
	public $show_user;
	public $echo_request;
	public $slow;
	public $info;

	// configure settles what the server is before it is anything else.
	public function configure() {
		$this->addr = getenv("TESTSERVER_ADDR");
		if ($this->addr === false || $this->addr === "") {
			$this->addr = "127.0.0.1:8099";
		}
		// Four workers answer at once; anything else waits in a queue of 64. A
		// handler here is well under a millisecond, so the queue is depth
		// rather than latency: it is what a burst waits in instead of starting
		// a runtime of its own.
		$this->workers = 4;
		$workers = getenv("TESTSERVER_WORKERS");
		if ($workers !== false && $workers !== "") {
			$this->workers = intval($workers);
		}
		$this->queue = 64;

		// Ten seconds unless the environment says otherwise, which is long
		// enough to answer a few requests by hand and short enough that a
		// forgotten one does not outlive the terminal. A load test wants more.
		$this->limit = 10;
		$limit = getenv("TESTSERVER_LIMIT");
		if ($limit !== false && $limit !== "") {
			$this->limit = intval($limit);
		}
	}

	// handlers assigns each one to the property it is registered under. They
	// are closures rather than methods because a handler is a function of its
	// arguments, and a closure is that written down.
	public function handlers() {
		$this->index = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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

		$this->hello = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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

		// {id} is a net/http pattern segment, read back through the request.
		// The encoder writes straight to the response: there is no string of
		// the document in between, and it converts a PHP array the way
		// json_encode() does.
		$this->show_user = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "application/json");
			(new JSON\Encoder($w))->encode(array(
				"id" => $r->path_value("id"),
				"path" => $r->url->path,
			));
		};

		$this->echo_request = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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

		// What connection_aborted() is for. The work is allowed to finish,
		// because in a real application it is the transaction that already
		// committed; what is skipped is the part nobody is left to read.
		// ignore_user_abort(true) is what keeps the handler running long enough
		// to make that choice, because without it the disconnect ends the
		// handler where it next looks.
		$this->slow = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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

		$this->info = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
			$w->header()->set("Content-Type", "text/markdown; charset=utf-8");
			phpinfo();
		};
	}

	// mount routes to the handlers, each named by the property holding it.
	public function mount() {
		$this->mux = new HTTP\Mux();
		$this->mux->handle('GET /{$}', $this->index);
		$this->mux->handle("GET /hello", $this->hello);
		$this->mux->handle("GET /users/{id}", $this->show_user);
		$this->mux->handle("POST /echo", $this->echo_request);
		$this->mux->handle("GET /slow", $this->slow);
		$this->mux->handle("GET /info", $this->info);
	}

	// listen binds and starts answering without blocking, and answers the
	// address it bound, so asking for port 0 is how a test takes a free one
	// rather than hoping.
	public function listen() {
		$this->http = new HTTP\Server($this->addr, $this->mux, $this->workers, $this->queue);
		return $this->http->listen();
	}

	// run is the entrypoint: everything above, in the order it has to happen.
	public function run() {
		$this->configure();
		set_time_limit($this->limit);

		$this->handlers();
		$this->mount();
		$bound = $this->listen();

		// Stopping the server is the script's own business, so it is written
		// here rather than left to whatever ends the script. The shutdown pass
		// runs with the clock off, which is what makes this still run when the
		// time limit is what ended us.
		$http = $this->http;
		register_shutdown_function(function () use ($http) {
			$http->shutdown();
			echo "server stopped\n";
		});

		// What this runtime is, before it starts answering for it.
		phpinfo();

		echo "\nserving on http://" . $bound . " until the time limit is up\n";

		// Returns when the script runs out of time, which is the whole of the
		// run. It is the last statement on purpose: the limit is a fatal, as
		// php's is, so anything after it would not run and the process would
		// exit non-zero. Ending here lets the shutdown callback have the last
		// word and the process exit 0.
		$this->http->wait();
	}
}

$server = new Server;
$server->run();
