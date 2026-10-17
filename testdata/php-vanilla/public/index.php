<?php

// testserver.php written as a vanilla PHP application, route for route, so the
// two can be put through the same load generator and the difference read off.
//
// It is the same API: the same patterns, the same JSON documents, the same reply
// bodies. What it has that the phpscript side does not is a bootstrap per
// request. Apache hands the request to mod_php, the autoloader runs, Slim builds
// its router and its middleware stack, the handler answers, and everything built
// is torn down again. That bootstrap is the whole of what the comparison
// measures, and opcache is on because a PHP application served without it
// recompiles every file on every request and nobody serves that.
//
// Slim carries the routing, which is the one part of testserver.php that has no
// vanilla equivalent: HTTP\Mux is net/http's ServeMux, and `GET /users/{id}` is
// its pattern syntax. Slim's is the same shape, so the route table below reads
// the way the mount() method over there does.
//
// scripts/bench-http.sh runs it as the `vanilla` target and
// testdata/php-vanilla/README.md is how to run it by hand.

declare(strict_types=1);

use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use Slim\Factory\AppFactory;

require __DIR__ . '/../vendor/autoload.php';

$app = AppFactory::create();

// The route cache, which is what Slim is configured with in production: without
// it FastRoute compiles the route table on every request. The file is written
// by the first request that finds it missing, so the sweep's warmup is what
// creates it, and opcache holds the compiled form from then on.
$app->getRouteCollector()->setCacheFile(__DIR__ . '/../var/route-cache.php');

// '/' alone, which is what '{$}' anchors on the phpscript side: Slim matches a
// route exactly and has no subtree patterns, so nothing extra is needed to keep
// this off every path nothing else claimed.
$app->get('/', function (Request $request, Response $response): Response {
    $response->getBody()->write("phpscript test server\n\n");
    $response->getBody()->write("GET  /             this page ('{\$}' anchors it; a bare\n");
    $response->getBody()->write("                   'GET /' is a net/http subtree and would\n");
    $response->getBody()->write("                   answer for every path nothing else claimed)\n");
    $response->getBody()->write("GET  /hello        a greeting\n");
    $response->getBody()->write("GET  /users/{id}   reads a path value\n");
    $response->getBody()->write("POST /echo         reports the request it was given\n");
    $response->getBody()->write("GET  /slow         holds the connection, and notices if you leave\n");
    $response->getBody()->write("GET  /info         phpinfo()\n");

    return $response->withHeader('Content-Type', 'text/plain; charset=utf-8');
});

$app->get('/hello', function (Request $request, Response $response): Response {
    $name = $request->getQueryParams()['name'] ?? '';
    if ($name === '') {
        $name = 'world';
    }
    $response->getBody()->write('hello ' . $name . "\n");

    return $response->withHeader('Content-Type', 'text/plain; charset=utf-8');
});

// JSON_UNESCAPED_SLASHES, because php escapes a forward slash in a string and
// Go's encoder does not, so a path would read "\/users\/42" here and
// "/users/42" on the phpscript side. Both sides put the same bytes on the wire
// with it, which is what makes the throughput columns comparable. php and Go
// still disagree on two things no route here produces: php escapes non-ASCII to
// \uXXXX where Go writes UTF-8, and Go escapes <, > and & where php does not.
//
// The newline after the document is JSON\Encoder's, which writes a stream of
// values and separates one from the next with it. json_encode() returns the
// document alone, so the newline is appended here.
$app->get('/users/{id}', function (Request $request, Response $response, array $args): Response {
    $document = json_encode([
        'id' => $args['id'],
        'path' => $request->getUri()->getPath(),
    ], JSON_UNESCAPED_SLASHES);
    $response->getBody()->write($document . "\n");

    return $response->withHeader('Content-Type', 'application/json');
});

// form_value() reads the body and the query together, the body first, which is
// what net/http's FormValue answers and so what the phpscript side answers.
$app->post('/echo', function (Request $request, Response $response): Response {
    $body = $request->getParsedBody();
    $name = '';
    if (is_array($body) && isset($body['name'])) {
        $name = (string) $body['name'];
    } elseif (isset($request->getQueryParams()['name'])) {
        $name = (string) $request->getQueryParams()['name'];
    }

    $document = json_encode([
        'method' => $request->getMethod(),
        'path' => $request->getUri()->getPath(),
        'query' => $request->getUri()->getQuery(),
        'user_agent' => $request->getHeaderLine('User-Agent'),
        'name' => $name,
    ], JSON_UNESCAPED_SLASHES);
    $response->getBody()->write($document . "\n");

    return $response->withHeader('Content-Type', 'application/json');
});

// What connection_aborted() is for, with the difference vanilla PHP forces: it
// reports a write that failed, and nothing here writes, so a disconnect goes
// unnoticed until something does. The space per tick is that write. The
// phpscript side reads the request's context instead and needs none, which is
// why its reply is the final line alone.
//
// Written outside Slim's response, because a body already on the wire cannot
// carry a header set afterwards. The work is allowed to finish either way: in a
// real application it is the transaction that already committed, and what is
// skipped is the part nobody is left to read.
$app->get('/slow', function (Request $request, Response $response): Response {
    ignore_user_abort(true);
    header('Content-Type: text/plain; charset=utf-8');

    $ticks = 0;
    while ($ticks < 20) {
        usleep(100000);
        $ticks++;
        echo ' ';
        flush();
        if (connection_aborted()) {
            break;
        }
    }

    if (connection_aborted()) {
        error_log('aborted after ' . $ticks . ' ticks, response skipped');

        return $response;
    }

    echo 'waited ' . $ticks . " ticks, still connected\n";

    return $response;
});

$app->get('/info', function (Request $request, Response $response): Response {
    header('Content-Type: text/markdown; charset=utf-8');
    phpinfo();

    return $response;
});

$app->run();
