<?php

// @route GET /

// The same API as testdata/phpscript-http-bridge/server.php, in the shape a
// phpscript application takes: one file per endpoint, the annotation above as
// the whole routing table, and the request in the superglobals. Nothing
// here builds a router or holds a handler.
//
// This is Path A, a runtime per request. The bridge is Path B, workers forked
// once at listen(). A number from one never belongs in a table with a number
// from the other; docs/agents/performance.md is the rule.

header("Content-Type: text/plain; charset=utf-8");

echo "phpscript test server\n\n";
echo "GET  /             this page ('{\$}' anchors it; a bare\n";
echo "                   'GET /' is a net/http subtree and would\n";
echo "                   answer for every path nothing else claimed)\n";
echo "GET  /hello        a greeting\n";
echo "GET  /users/{id}   reads a path value\n";
echo "POST /echo         reports the request it was given\n";
echo "GET  /slow         holds the connection, and notices if you leave\n";
echo "GET  /info         phpinfo()\n";
