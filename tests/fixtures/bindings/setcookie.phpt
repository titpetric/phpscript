name: setcookie stages a Set-Cookie header
description: >
  setcookie and setrawcookie stage a response header rather than writing one,
  which is why the harness supplies the request and the php cli runner sits this
  one out - it has no response to stage against. Covers the url-encoding that
  separates the two spellings, the array form that carries samesite, and the
  reading of expires 0 as "no expiry at all". The attribute spelling is
  net/http's rather than php's; RFC 6265 makes attribute names
  case-insensitive, so a client cannot tell and only a test like this one can.
runner:
  php: false

response:
  headers:
    Set-Cookie: "session=abc123; Path=/; HttpOnly"
---
<?php

// The first cookie is the one the header assertion above reads: name, value,
// no expiry, and the two attributes a session cookie carries.
var_dump(setcookie("session", "abc123", 0, "/", "", false, true));

// A value with a space is url-encoded by setcookie and passed through by
// setrawcookie. Both answer true, because both staged a header.
var_dump(setcookie("greeting", "hello world"));
var_dump(setrawcookie("raw", "hello_world"));

// The array form is the one that can carry samesite. An option it does not
// name keeps its zero value.
var_dump(setcookie("wide", "1", array(
	"expires" => 2000000000,
	"path" => "/app",
	"domain" => "example.test",
	"secure" => true,
	"httponly" => true,
	"samesite" => "Lax",
)));

// Deleting a cookie is the ordinary call with an expiry in the past, which is
// a timestamp like any other rather than a case of its own.
var_dump(setcookie("session", "", 1));
?>
---
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
