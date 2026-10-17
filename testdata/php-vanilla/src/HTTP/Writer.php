<?php

declare(strict_types=1);

namespace HTTP;

// Writer is what a handler writes a response through, which is io.Writer on the
// phpscript side: JSON\Encoder takes one and so does anything else that streams
// a document, so neither has to know it is a response it is writing to.
interface Writer
{
    public function write(string $data): int;
}
