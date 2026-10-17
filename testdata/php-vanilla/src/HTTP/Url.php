<?php

declare(strict_types=1);

namespace HTTP;

use Psr\Http\Message\ServerRequestInterface;

// Url is net/url's URL as a request carries it: $path and $rawquery are read
// straight off it and query() parses the raw form on demand, which is what the
// phpscript side's $r->url does.
final class Url
{
    public readonly string $path;
    public readonly string $rawquery;

    private ?Values $query = null;

    public function __construct(private ServerRequestInterface $request)
    {
        $this->path = $request->getUri()->getPath();
        $this->rawquery = $request->getUri()->getQuery();
    }

    // query parses the query string once and answers the same Values after
    // that, as net/url does for a request that reads it twice.
    public function query(): Values
    {
        return $this->query ??= new Values($this->request->getQueryParams());
    }
}
