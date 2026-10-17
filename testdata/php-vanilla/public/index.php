<?php

// testdata/testserver.php as a vanilla PHP application, so the two can be put
// through the same load generator and the difference read off.
//
// It is the same program. The Server class is the same class, config() reads
// the same environment, mount() registers the same six handlers against the
// same patterns, and every handler body below is the one over there. What
// changed is run(): apache is the server here, so there is nothing to listen
// on and nothing to wait for, and src/HTTP is the surface the handlers are
// written against, built over Slim's router and PSR-7.
//
// A handler takes its state from its arguments, as it does over there. It
// starts holding $w and $r and nothing else, no superglobal is read, and there
// are no properties, so there is nothing else for a handler to reach. What
// does carry over between requests is the process itself: apache hands the
// next request to the same worker, opcache holds the compiled files, and
// everything the last request built was torn down at the end of it. That
// bootstrap is the whole of what the comparison measures.
//
// Run it with `docker compose up -d --wait`, and sweep it with
// `BENCH_TARGETS="php vanilla" scripts/bench-http.sh after`. The README beside
// this file is the rest.

declare(strict_types=1);

require __DIR__ . '/../vendor/autoload.php';

class Server
{
    // config reads the environment into the one value everything else is built
    // from, before anything is built.
    //
    // The worker count and the queue depth are apache's here and not the
    // script's: mpm_prefork forks VANILLA_WORKERS children at startup and the
    // kernel's listen backlog is the queue, so conf/mpm.conf is where they are
    // set and this reads them back for the manifest alone.
    public function config(): array
    {
        $addr = getenv('VANILLA_ADDR');
        if ($addr === false || $addr === '') {
            $addr = '127.0.0.1:' . (getenv('VANILLA_PORT') ?: '8098');
        }

        $workers = intval(getenv('VANILLA_WORKERS'));
        if ($workers < 1) {
            $workers = 4;
        }

        return [
            'addr' => $addr,
            'workers' => $workers,
            'queue' => 64,
            'cache' => __DIR__ . '/../var/route-cache.php',
        ];
    }

    // mount builds the router and returns it. $this->index(...) is the method
    // below as a bound callable, which is what reading it without parentheses
    // answers on the phpscript side.
    public function mount(): \HTTP\Mux
    {
        $mux = new \HTTP\Mux($this->config()['cache']);

        $mux->handle('GET /{$}', $this->index(...));
        $mux->handle('GET /hello', $this->hello(...));
        $mux->handle('GET /users/{id}', $this->showUser(...));
        $mux->handle('POST /echo', $this->echoRequest(...));
        $mux->handle('GET /slow', $this->slow(...));
        $mux->handle('GET /info', $this->info(...));

        return $mux;
    }

    public function index(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        $w->header()->set('Content-Type', 'text/plain; charset=utf-8');
        // A handler's echo reaches the response: the buffer is open for the
        // length of the call and what it holds becomes the body.
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

    public function hello(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        $w->header()->set('Content-Type', 'text/plain; charset=utf-8');
        // query() is net/url's Values, so ->get() answers the first value
        // under a name, or "" for a name that is not there.
        // $r->form_value($name) is the same answer over the query and a form
        // body together.
        $name = $r->url->query()->get('name');
        if ($name === '') {
            $name = 'world';
        }

        $w->write('hello ' . $name . "\n");
    }

    // {id} is a route pattern segment, read back through the request. The
    // encoder writes straight to the response: there is no string of the
    // document in between, and it converts a PHP array the way json_encode()
    // does.
    public function showUser(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        $w->header()->set('Content-Type', 'application/json');
        (new \JSON\Encoder($w))->encode(['id' => $r->path_value('id'), 'path' => $r->url->path]);
    }

    // form_value reads the query and the body, and the body is parsed by the
    // time a handler runs, so nothing has to be parsed first.
    public function echoRequest(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        $w->header()->set('Content-Type', 'application/json');
        (new \JSON\Encoder($w))->encode([
            'method' => $r->method,
            'path' => $r->url->path,
            'query' => $r->url->rawquery,
            'user_agent' => $r->user_agent(),
            'name' => $r->form_value('name'),
        ]);
    }

    // What connection_aborted() is for. The work is allowed to finish, because
    // in a real application it is the transaction that already committed; what
    // is skipped is the part nobody is left to read. ignore_user_abort(true) is
    // what keeps the handler running long enough to make that choice, because
    // without it the disconnect ends the handler where it next looks.
    //
    // The write and the flush per tick are what php needs here that the
    // phpscript side does not: connection_aborted() reports a write that
    // failed, so a loop writing nothing never notices the client leaving. Over
    // there the handler reads the request's context and its reply is the final
    // line alone.
    public function slow(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        ignore_user_abort(true);
        $w->header()->set('Content-Type', 'text/plain; charset=utf-8');

        $ticks = 0;
        while ($ticks < 20) {
            usleep(100000);
            $ticks++;
            $w->write(' ');
            $w->flush();
            if (connection_aborted()) {
                break;
            }
        }

        if (connection_aborted()) {
            // Nothing more is written to the response: there is nobody to
            // write it to. error_log puts this through the host's error
            // handler, which here is apache's error log.
            error_log('aborted after ' . $ticks . ' ticks, response skipped');

            return;
        }

        $w->write('waited ' . $ticks . " ticks, still connected\n");
    }

    public function info(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
    {
        $w->header()->set('Content-Type', 'text/markdown; charset=utf-8');
        phpinfo();
    }

    // run is main().
    //
    // No listen() and no wait(): apache is the server, this process is already
    // inside one request, and run() is what answers it. The address and the
    // worker count config() reads are apache's, set in conf/, and the shutdown
    // that stops it listening is apache's too.
    public function run(): void
    {
        $this->mount()->run();
    }
}

$server = new Server();

$server->run();
