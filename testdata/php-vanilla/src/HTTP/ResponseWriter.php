<?php

declare(strict_types=1);

namespace HTTP;

use Psr\Http\Message\ResponseInterface;

// ResponseWriter is net/http's ResponseWriter over a PSR-7 response: header()
// answers headers a handler sets, write() and echo both reach the body, and
// write_header() sets the status.
//
// It exists because a PSR-7 response is immutable and a handler on the
// phpscript side writes to one value for the length of the call. The headers
// and the status are collected here and apply() is what puts them on the
// response Slim emits.
//
// The body goes through PHP's output buffer and not through a string here, so
// a handler's echo and its write() keep the order it made them in. Mux opens
// the buffer for the length of the call.
final class ResponseWriter implements Writer
{
    private Header $header;
    private int $status = 200;
    private bool $flushed = false;

    public function __construct()
    {
        $this->header = new Header();
    }

    public function header(): Header
    {
        return $this->header;
    }

    public function write(string $data): int
    {
        echo $data;

        return strlen($data);
    }

    public function write_header(int $status): void
    {
        $this->status = $status;
    }

    // flush sends what the handler has written so far to the client, which is
    // what a response written in parts needs here: php's connection_aborted()
    // reports a write that failed, so a handler that never flushes never
    // notices the client leaving.
    //
    // The headers go out ahead of the first flush, because nothing can set one
    // after a body has started. Everything after that is body alone, and
    // apply() answers an empty response for Slim to emit nothing more of.
    public function flush(): void
    {
        if (!$this->flushed) {
            $this->flushed = true;
            $this->sendHeaders();
        }
        if (ob_get_level() > 0) {
            ob_flush();
        }
        flush();
    }

    public function flushed(): bool
    {
        return $this->flushed;
    }

    // apply puts the status, the headers and the body on the response. A
    // handler that flushed has sent its own headers already and its body is on
    // the wire, so what comes back is the status alone.
    public function apply(ResponseInterface $response, string $body): ResponseInterface
    {
        $response = $response->withStatus($this->status);
        if ($this->flushed) {
            return $response;
        }

        foreach ($this->header->all() as $name => $values) {
            $response = $response->withHeader($name, $values);
        }
        if ($body !== '') {
            $response->getBody()->write($body);
        }

        return $response;
    }

    private function sendHeaders(): void
    {
        if (headers_sent()) {
            return;
        }
        foreach ($this->header->all() as $name => $values) {
            $replace = true;
            foreach ($values as $value) {
                header($name . ': ' . $value, $replace);
                $replace = false;
            }
        }
    }
}
