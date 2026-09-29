<?php

// A PHP program that is the HTTP server rather than something a server runs.
//
// A handler is a function of its arguments. It starts on a clean stack holding
// $w and $r and nothing else: no superglobals are decoded, no state carries
// over from the last request, and two requests answered at the same moment
// cannot see each other, because each runs on a runtime of its own.
//
// That is what a closure means here - a declaration, with its state arriving in
// its arguments. One that captures the scope it was written in, through
// use (...) or $this, is refused, because that scope belongs to the runtime
// that built it and two requests would be sharing it.
//
// HTTP\Server's third argument is how many requests may be answered at once;
// omitted it is the number of cores.
//
// set_time_limit is what ends the whole thing. The limit is a deadline on the
// context the interpreter checks and every binding is handed, and wait() is a
// binding, so the script runs out of time while it is parked there and the
// shutdown callback stops the server. Ten seconds, which is long enough to
// answer a few requests by hand and short enough that a forgotten one does not
// outlive the terminal it started in.
//
// Run it with `phpscript run testdata/testserver.php`, or against a port the
// system picks with TESTSERVER_ADDR=127.0.0.1:0.

set_time_limit(10);

$index = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/plain; charset=utf-8");
	// A handler's echo reaches the response: the runtime answering the request
	// writes there for the length of the call.
	echo "phpscript test server\n\n";
	echo "GET  /             this page ('{\$}' anchors it; a bare 'GET /' is a\n";
	echo "                   net/http subtree and would answer for every path\n";
	echo "                   nothing else claimed)\n";
	echo "GET  /hello        a greeting\n";
	echo "GET  /users/{id}   reads a path value\n";
	echo "POST /echo         reports the request it was given\n";
	echo "GET  /slow         holds the connection, and notices if you leave\n";
	echo "GET  /info         phpinfo()\n";
};

$hello = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/plain; charset=utf-8");
	// query() is net/http's url.Values, so ->get() answers the first value
	// under a name, or "" for a name that is not there. $r->form_value($name)
	// is the same answer over the query and a form body together.
	$name = $r->url->query()->get("name");
	if ($name === "") {
		$name = "world";
	}
	$w->write("hello " . $name . "\n");
};

// {id} is a net/http pattern segment, read back through the request. The
// encoder writes straight to the response: there is no string of the document
// in between, and it converts a PHP array the way json_encode() does.
$show_user = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "application/json");
	(new JSON\Encoder($w))->encode(array(
		"id" => $r->path_value("id"),
		"path" => $r->url->path,
	));
};

$echo_request = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "application/json");
	(new JSON\Encoder($w))->encode(array(
		"method" => $r->method,
		"path" => $r->url->path,
		"query" => $r->url->rawquery,
		"user_agent" => $r->user_agent(),
		// form_value reads the query and the body, and parses the body on the
		// first call, so parse_form() beforehand is not needed.
		"name" => $r->form_value("name"),
	));
};

// What connection_aborted() is for. The work is allowed to finish, because in a
// real application it is the transaction that already committed; what is
// skipped is the part nobody is left to read. ignore_user_abort(true) is what
// keeps the handler running long enough to make that choice, because without it
// the disconnect ends the handler where it next looks.
$slow = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
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
		// Nothing is written to the response: there is nobody to write it to.
		// The work above is what a real handler would have committed anyway.
		// error_log puts this on the request's trace and through the host's
		// error handler, which is where a served script says something went
		// wrong: the response belongs to a client that has left.
		error_log("aborted after " . $ticks . " ticks, response skipped");
		return;
	}

	$w->header()->set("Content-Type", "text/plain; charset=utf-8");
	$w->write("waited " . $ticks . " ticks, still connected\n");
};

$info = function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/markdown; charset=utf-8");
	phpinfo();
};

$addr = getenv("TESTSERVER_ADDR");
if ($addr === false || $addr === "") {
	$addr = "127.0.0.1:8099";
}

$mux = new HTTP\Mux();
$mux->handle('GET /{$}', $index);
$mux->handle("GET /hello", $hello);
$mux->handle("GET /users/{id}", $show_user);
$mux->handle("POST /echo", $echo_request);
$mux->handle("GET /slow", $slow);
$mux->handle("GET /info", $info);

// The mux routes; the server listens. listen() binds and starts answering
// without blocking, and returns the address it bound, so asking for port 0 is
// how a test takes a free one rather than hoping. Four workers, so /slow
// holding one still leaves three.
$server = new HTTP\Server($addr, $mux, 4);
$bound = $server->listen();

// Stopping the server is the script's own business, so it is written here
// rather than left to whatever ends the script. The shutdown pass runs with the
// clock off, which is what makes this still run when the time limit is what
// ended us.
register_shutdown_function(function () use ($server) {
	$server->shutdown();
	echo "server stopped\n";
});

// What this runtime is, before it starts answering for it.
phpinfo();

echo "\nserving on http://" . $bound . " until the time limit is up\n";

// Returns when the script runs out of time, which is the whole of the run.
//
// It is the last statement on purpose. The limit is a fatal, as php's is, so
// anything written after it would not run and the process would exit non-zero;
// ending here means the shutdown callback above gets the last word and the
// process exits 0.
$server->wait();
