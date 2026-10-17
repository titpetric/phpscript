<?php

declare(strict_types=1);

namespace HTTP;

use Psr\Http\Message\ResponseInterface;
use Psr\Http\Message\ServerRequestInterface;
use Slim\App;
use Slim\Exception\HttpMethodNotAllowedException;
use Slim\Exception\HttpNotFoundException;
use Slim\Factory\AppFactory;
use Slim\Interfaces\RouteCollectorInterface;

// Mux is net/http's ServeMux as Slim's router: a handler registers with one
// pattern and one callable, both spelled the way they are spelled on the
// phpscript side, and this is the whole of what this directory adds to run
// testserver.php's handlers on a served PHP stack.
//
// handle() takes net/http's "METHOD /path" pattern, because that is what the
// script writes. {name} segments are the same syntax in both routers, so the
// path passes through untouched and only the method and the exact-match anchor
// need reading.
final class Mux
{
    private App $app;

    // $cacheFile is where Slim writes the compiled route table, which is what
    // it is configured with in production: without it FastRoute compiles the
    // table on every request. The first request to find the file missing
    // writes it, so a warmup is what creates it.
    public function __construct(?string $cacheFile = null)
    {
        $this->app = AppFactory::create();
        if ($cacheFile !== null && self::cacheable($cacheFile)) {
            $this->routes()->setCacheFile($cacheFile);
        }
        $this->app->addRoutingMiddleware();
        $this->useServeMuxErrors();
    }

    // useServeMuxErrors answers an unrouted request the way net/http's ServeMux
    // answers it, which is what the phpscript side is running: 404 with "404
    // page not found", and 405 with "Method Not Allowed" and an Allow header
    // naming the methods the pattern does carry. Both carry the nosniff header
    // net/http sends with a plain-text error.
    //
    // Without this Slim has no handler for either exception and both come back
    // 500, so the two sides disagree on every path outside the route table.
    private function useServeMuxErrors(): void
    {
        $errors = $this->app->addErrorMiddleware(false, true, false);

        $errors->setErrorHandler(
            HttpNotFoundException::class,
            fn (): ResponseInterface => $this->plainError(404, "404 page not found\n"),
        );
        $errors->setErrorHandler(
            HttpMethodNotAllowedException::class,
            function (ServerRequestInterface $request, \Throwable $error): ResponseInterface {
                $response = $this->plainError(405, "Method Not Allowed\n");
                if ($error instanceof HttpMethodNotAllowedException) {
                    $response = $response->withHeader('Allow', implode(', ', $error->getAllowedMethods()));
                }

                return $response;
            },
        );
    }

    private function plainError(int $status, string $body): ResponseInterface
    {
        $response = $this->app->getResponseFactory()->createResponse($status);
        $response->getBody()->write($body);

        return $response
            ->withHeader('Content-Type', 'text/plain; charset=utf-8')
            ->withHeader('X-Content-Type-Options', 'nosniff');
    }

    // cacheable reports whether Slim can be handed the path, and makes the
    // directory when it is missing. Slim throws on a directory it cannot write
    // and this runs under whatever user the server runs as, so an unwritable
    // path answers false and the router compiles its table per request instead
    // of the process dying on a cache it was only ever offered.
    private static function cacheable(string $path): bool
    {
        if (is_file($path)) {
            return is_readable($path);
        }

        $dir = dirname($path);
        if (!is_dir($dir) && !@mkdir($dir, 0o775, true) && !is_dir($dir)) {
            return false;
        }

        return is_writable($dir);
    }

    // handle registers $handler for $pattern. $handler is called with the
    // response writer and the request, in that order, which is the signature
    // every handler in the script has.
    public function handle(string $pattern, callable $handler): void
    {
        [$method, $path] = $this->parse($pattern);
        $this->app->map([$method], $path, self::serve($handler));
    }

    // run answers the request this process was started for. Apache is the
    // server here, so there is nothing to listen on and nothing to wait for:
    // the process is already inside one request and this is what answers it.
    public function run(): void
    {
        $this->app->run();
    }

    public function routes(): RouteCollectorInterface
    {
        return $this->app->getRouteCollector();
    }

    // parse splits net/http's pattern into the method and the path.
    //
    // '{$}' anchors an exact match over there, where a bare 'GET /' is a
    // subtree that answers for every path nothing else claimed. Slim matches a
    // route exactly and has no subtree patterns, so the anchor is read off and
    // dropped.
    //
    // @return array{string, string}
    private function parse(string $pattern): array
    {
        $parts = explode(' ', trim($pattern), 2);
        if (count($parts) !== 2) {
            throw new \InvalidArgumentException('HTTP\Mux::handle: pattern is "METHOD /path", got "' . $pattern . '"');
        }
        [$method, $path] = $parts;

        if (str_ends_with($path, '{$}')) {
            $path = substr($path, 0, -3);
        }
        if ($path === '' || !str_starts_with($path, '/')) {
            $path = '/' . $path;
        }
        if ($path !== '/' && str_ends_with($path, '/')) {
            $path = rtrim($path, '/');
        }

        return [strtoupper($method), $path];
    }

    // serve adapts one handler to what Slim calls. The two differences it
    // covers are the argument order, Slim passing the request first where
    // net/http passes the writer first, and the path arguments, which net/http
    // carries on the request itself.
    //
    // The output buffer is open for the length of the call, so a handler's echo
    // reaches the response and not the process's own output, which is what the
    // runtime answering the request does on the phpscript side. ob_get_clean in
    // a finally, so a handler that throws still leaves the buffer closed and
    // its half-written body is what the client gets.
    private static function serve(callable $handler): callable
    {
        return static function (
            ServerRequestInterface $request,
            ResponseInterface $response,
            array $args,
        ) use ($handler): ResponseInterface {
            $w = new ResponseWriter();
            $r = new Request($request, $args);

            ob_start();
            try {
                $handler($w, $r);
            } finally {
                $body = ob_get_clean();
            }

            return $w->apply($response, $body === false ? '' : $body);
        };
    }
}
