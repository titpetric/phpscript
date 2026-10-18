<?php

// testdata/phpscript-http-bridge/server.php on Slim, the same program with the
// signatures changed: the same class, the same six handlers against the same
// patterns, answering the same bytes, so one load generator drives both.
//
// What a handler is written against is the difference. Slim's router and PSR-7
// force four substitutions, and a PSR-7 response is immutable, so a handler
// returns the response it built where the bridge writes to the one its caller
// passed in:
//
//	$w->header()->set($n, $v)             $response = $response->withHeader($n, $v)
//	$w->write($s) / echo $s               $response->getBody()->write($s)
//	new JSON\Encoder($w))->encode($a)     json_encode($a, JSON_FLAGS) . "\n"
//	$r->path_value("id")                  $args["id"]
//
// This side pays a bootstrap per request: the autoloader runs, Slim builds its
// App, its router and its middleware stack, the handler answers, and all of it
// is torn down again. The README beside this file prices that against the same
// handler with nothing under it.

declare(strict_types=1);

use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use Slim\App;
use Slim\Exception\HttpMethodNotAllowedException;
use Slim\Exception\HttpNotFoundException;
use Slim\Factory\AppFactory;

require __DIR__ . '/../vendor/autoload.php';

// The flags php needs to write Go's bytes. JSON_UNESCAPED_SLASHES, because php
// escapes a forward slash and Go does not. JSON_HEX_AMP, because Go's encoder
// escapes an ampersand and php writes it as itself, which a raw query string
// carrying two parameters reaches.
//
// Two differences are left and neither has a flag that closes it: php escapes
// non-ASCII to \uXXXX where Go writes UTF-8, and Go escapes < and > to
// lowercase < where php's JSON_HEX_TAG writes <. No handler here
// produces either.
const JSON_FLAGS = JSON_UNESCAPED_SLASHES | JSON_HEX_AMP | JSON_THROW_ON_ERROR;

class Server
{
    // `config()` reads the environment into the one value everything else is
    // built from, before anything is built.
    //
    // The worker count is the web server's here and not the script's: apache
    // forks VANILLA_WORKERS children and php-fpm starts that many, so conf/ is
    // where each is told it and this reads it back for the record alone.
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
            'cache' => __DIR__ . '/../var/route-cache.php',
        ];
    }

    // mount builds the router and returns it. $this->index(...) is the method
    // below as a bound callable, the same registration the bridge writes as
    // $this->index with no parentheses.
    public function mount(): App
    {
        $app = AppFactory::create();

        // The route cache, the setting a production Slim carries: without it
        // FastRoute compiles the route table on every request. The first
        // request to find the file missing writes it.
        $cache = $this->config()['cache'];
        if (is_dir(dirname($cache)) && is_writable(dirname($cache))) {
            $app->getRouteCollector()->setCacheFile($cache);
        }
        $app->addRoutingMiddleware();

        // An unrouted request answers what net/http's ServeMux answers, so the
        // two sides agree on every path outside the route table. Slim has no
        // handler for either exception and both come back 500 without this.
        $plain = static function (int $status, string $body) use ($app): Response {
            $response = $app->getResponseFactory()->createResponse($status);
            $response->getBody()->write($body);

            return $response->withHeader('Content-Type', 'text/plain; charset=utf-8')
                ->withHeader('X-Content-Type-Options', 'nosniff');
        };
        $errors = $app->addErrorMiddleware(false, true, false);
        $errors->setErrorHandler(
            HttpNotFoundException::class,
            static fn (): Response => $plain(404, "404 page not found\n"),
        );
        $errors->setErrorHandler(
            HttpMethodNotAllowedException::class,
            static function (Request $request, \Throwable $error) use ($plain): Response {
                $allowed = $error instanceof HttpMethodNotAllowedException ? $error->getAllowedMethods() : [];

                return $plain(405, "Method Not Allowed\n")->withHeader('Allow', implode(', ', $allowed));
            },
        );

        // '/' alone, where the bridge writes '{$}' to anchor it: Slim matches
        // a route exactly and has no subtree patterns, so nothing extra keeps
        // this off every path nothing else claimed.
        $app->get('/', $this->index(...));
        $app->get('/hello', $this->hello(...));
        $app->get('/users/{id}', $this->showUser(...));
        $app->post('/echo', $this->echoRequest(...));
        $app->get('/slow', $this->slow(...));
        $app->get('/info', $this->info(...));

        return $app;
    }

    public function index(Request $request, Response $response): Response
    {
        $response = $response->withHeader('Content-Type', 'text/plain; charset=utf-8');
        $response->getBody()->write("phpscript test server\n\n");
        $response->getBody()->write("GET  /             this page ('{\$}' anchors it; a bare\n");
        $response->getBody()->write("                   'GET /' is a net/http subtree and would\n");
        $response->getBody()->write("                   answer for every path nothing else claimed)\n");
        $response->getBody()->write("GET  /hello        a greeting\n");
        $response->getBody()->write("GET  /users/{id}   reads a path value\n");
        $response->getBody()->write("POST /echo         reports the request it was given\n");
        $response->getBody()->write("GET  /slow         holds the connection, and notices if you leave\n");
        $response->getBody()->write("GET  /info         phpinfo()\n");

        return $response;
    }

    public function hello(Request $request, Response $response): Response
    {
        $response = $response->withHeader('Content-Type', 'text/plain; charset=utf-8');
        // getQueryParams() is net/url's Values: a name that is not there is
        // absent, and "" is what ->get() answers for it over there.
        $name = $request->getQueryParams()['name'] ?? '';
        if ($name === '') {
            $name = 'world';
        }

        $response->getBody()->write('hello ' . $name . "\n");

        return $response;
    }

    // {id} is a route pattern segment, read back through the arguments the
    // router matched. net/http carries those on the request itself, which is
    // the one difference this signature shows.
    public function showUser(Request $request, Response $response, array $args): Response
    {
        $response = $response->withHeader('Content-Type', 'application/json');
        $document = ['id' => $args['id'], 'path' => $request->getUri()->getPath()];
        $response->getBody()->write(json_encode($document, JSON_FLAGS) . "\n");

        return $response;
    }

    // getParsedBody() is the form, parsed by the time a handler runs, so
    // nothing has to be parsed first. The body takes precedence over the
    // query, the order net/http's FormValue and the bridge's form_value()
    // both answer in.
    public function echoRequest(Request $request, Response $response): Response
    {
        $response = $response->withHeader('Content-Type', 'application/json');
        $body = $request->getParsedBody();
        $name = '';
        if (is_array($body) && isset($body['name'])) {
            $name = (string) $body['name'];
        } elseif (isset($request->getQueryParams()['name'])) {
            $name = (string) $request->getQueryParams()['name'];
        }

        $document = [
            'method' => $request->getMethod(),
            'path' => $request->getUri()->getPath(),
            'query' => $request->getUri()->getQuery(),
            'user_agent' => $request->getHeaderLine('User-Agent'),
            'name' => $name,
        ];
        $response->getBody()->write(json_encode($document, JSON_FLAGS) . "\n");

        return $response;
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
    // line alone. The response goes out directly and not through the PSR-7
    // body, because a body already on the wire carries no header set after it.
    public function slow(Request $request, Response $response): Response
    {
        ignore_user_abort(true);
        header('Content-Type: text/plain; charset=utf-8');

        $ticks = 0;
        while ($ticks < 20) {
            usleep(100000);
            $ticks++;
            echo ' ';
            if (ob_get_level() > 0) {
                ob_flush();
            }
            flush();
            if (connection_aborted()) {
                break;
            }
        }

        if (connection_aborted()) {
            // Nothing is written to the response: there is nobody to write it
            // to. error_log puts this through the host's error handler, which
            // here is the web server's error log.
            error_log('aborted after ' . $ticks . ' ticks, response skipped');

            return $response;
        }

        echo 'waited ' . $ticks . " ticks, still connected\n";

        return $response;
    }

    public function info(Request $request, Response $response): Response
    {
        header('Content-Type: text/markdown; charset=utf-8');
        phpinfo();

        return $response;
    }

    // run is main().
    //
    // No listen() and no wait(): the web server is the server, this process is
    // already inside one request, and run() is what answers it. The address and
    // the worker count `config()` reads are the web server's, set in conf/, and
    // the shutdown that stops it listening is the web server's too.
    public function run(): void
    {
        $this->mount()->run();
    }
}

$server = new Server();

$server->run();
