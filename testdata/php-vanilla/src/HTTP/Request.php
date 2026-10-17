<?php

declare(strict_types=1);

namespace HTTP;

use Psr\Http\Message\ServerRequestInterface;

// Request is the request a handler is given, with the surface the phpscript
// side's \HTTP\Request has: $method and $url as properties, and path_value,
// form_value, user_agent and header as methods.
//
// It wraps the PSR-7 request Slim built and holds the path arguments the router
// matched, because net/http carries those on the request itself and PSR-7
// hands them to the handler separately.
final class Request
{
    public readonly string $method;
    public readonly Url $url;

    private ?Header $header = null;

    /** @param array<string, string> $pathValues */
    public function __construct(
        private ServerRequestInterface $request,
        private array $pathValues = [],
    ) {
        $this->method = $request->getMethod();
        $this->url = new Url($request);
    }

    // path_value answers the {name} segment the route matched, or "" for a name
    // the pattern does not carry.
    public function path_value(string $name): string
    {
        return $this->pathValues[$name] ?? '';
    }

    // form_value reads the body and the query together, the body first, which
    // is what net/http's FormValue answers. The body is parsed by the time a
    // handler runs, so nothing has to be parsed first.
    public function form_value(string $name): string
    {
        $body = $this->request->getParsedBody();
        if (is_array($body) && isset($body[$name])) {
            $value = $body[$name];

            return is_scalar($value) ? (string) $value : '';
        }

        return $this->url->query()->get($name);
    }

    public function user_agent(): string
    {
        return $this->request->getHeaderLine('User-Agent');
    }

    // header answers the request's headers, as net/http's Request.Header does.
    public function header(): Header
    {
        if ($this->header === null) {
            $this->header = new Header();
            foreach ($this->request->getHeaders() as $name => $values) {
                foreach ($values as $value) {
                    $this->header->add($name, $value);
                }
            }
        }

        return $this->header;
    }
}
