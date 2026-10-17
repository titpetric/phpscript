<?php

declare(strict_types=1);

namespace JSON;

use HTTP\Writer;

// Encoder writes JSON values to a stream, which is what JSON\Encoder does on
// the phpscript side: it takes the writer it wraps, so a handler encodes
// straight to the response and no string of the document exists in between.
final class Encoder
{
    // The flags that make php's encoder write Go's bytes, which is what the
    // two sides answering the same document takes.
    //
    // JSON_UNESCAPED_SLASHES, because php escapes a forward slash and Go does
    // not: without it a path reads "\/users\/42" here and "/users/42" over
    // there. JSON_HEX_AMP, because Go's encoder escapes an ampersand to
    // & and php writes it as itself, which a raw query string reaches on
    // any request carrying two parameters.
    //
    // Two differences are left and neither has a flag that closes it. php
    // escapes non-ASCII to \uXXXX where Go writes UTF-8, which
    // JSON_UNESCAPED_UNICODE would fix if the rest agreed. And Go escapes <
    // and > to lowercase < and > where php's JSON_HEX_TAG writes
    // < and >, so the escape is right and the hex case is not. No
    // handler here produces either.
    private const FLAGS = JSON_UNESCAPED_SLASHES | JSON_HEX_AMP;

    private string $prefix = '';
    private string $indent = '';

    public function __construct(private Writer $stream)
    {
    }

    // encode writes $value to the stream as JSON, followed by a newline.
    //
    // The newline is Go's, not php's: an Encoder writes a stream of values and
    // the newline is what separates one from the next. A single value followed
    // by a newline is still valid JSON to any reader.
    public function encode(mixed $value): void
    {
        $flags = self::FLAGS;
        if ($this->indent !== '') {
            $flags |= JSON_PRETTY_PRINT;
        }

        $document = json_encode($value, $flags | JSON_THROW_ON_ERROR);
        if ($this->prefix !== '' || $this->indent !== '') {
            $document = $this->reindent($document);
        }

        $this->stream->write($document . "\n");
    }

    // set_indent is json.Encoder.SetIndent: each value is written across
    // several lines, $indent per level under $prefix, and two empty strings go
    // back to one line per value.
    public function set_indent(string $prefix, string $indent): void
    {
        $this->prefix = $prefix;
        $this->indent = $indent;
    }

    // reindent rewrites json_encode's four spaces per level as the prefix and
    // the indent that were asked for. php's pretty printer has no setting for
    // either, so the document it wrote is what this reads.
    private function reindent(string $document): string
    {
        $lines = explode("\n", $document);
        foreach ($lines as $i => $line) {
            $depth = 0;
            while (str_starts_with(substr($line, $depth * 4), '    ')) {
                $depth++;
            }
            $lines[$i] = $this->prefix . str_repeat($this->indent, $depth) . substr($line, $depth * 4);
        }

        return implode("\n", $lines);
    }
}
