name: inbound http request binding
description: >
  HTTP\Request::current() is a host binding with no PHP counterpart, so the
  expected output is the runtime's contract rather than PHP's. A fixture runs
  off a request, which is the case this covers: the binding answers null, and a
  script reads the superglobals instead. The value it hands back while a request
  is being served is covered by TestRouteAPIEcho in tests/route_test.go, which
  drives a real handler.

  It also pins two things a script sees about a host-backed object, because both
  read as bugs when met for the first time: the class name is the Go type's,
  without the namespace the class was registered under, and a nil pointer field
  is neither null nor false.
runner:
  php: false
---
<?php

// Off a request there is nothing to hand back.
$current = HTTP\Request::current();
var_dump($current);

$request = new HTTP\Request("GET", "https://example.invalid/users");

// The class name a script sees is the name of the Go type, not the name the
// class was registered under, so an outbound and an inbound request are the
// same class and neither is HTTP\Request by that name.
echo get_class($request), "\n";
var_dump($request instanceof HTTP\Request);
var_dump($request instanceof Request);

// A pointer field that is nil arrives as a value that is not null and is not
// false. $request->tls is the one a script reaches for to ask about TLS; the
// answer is $_SERVER, where PHP puts it and where an unset key means no.
var_dump(is_null($request->tls));
var_dump((bool) $request->tls);
?>
---
NULL
Request
bool(false)
bool(true)
bool(false)
bool(true)
