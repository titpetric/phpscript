name: superglobals are writable in every scope
description: >
  A superglobal is one binding per request. Assigning the whole variable or a
  single key in the global scope is visible inside a function, and a write
  inside a function is visible after it returns — php allows all of this, and
  both phpscript runtimes match it. $_SESSION starts life unset under cli
  php, so the fixture assigns it before reading it back.
---
<?php

function read_get()     { return $_GET["foo"]; }
function read_post()    { return $_POST["foo"]; }
function read_cookie()  { return $_COOKIE["foo"]; }
function read_server()  { return $_SERVER["foo"]; }
function read_env()     { return $_ENV["foo"]; }
function read_files()   { return $_FILES["foo"]; }
function read_request() { return $_REQUEST["foo"]; }
function read_session() { return $_SESSION["foo"]; }

$_GET = array("foo" => "get-bar");
$_POST = array("foo" => "post-bar");
$_COOKIE = array("foo" => "cookie-bar");
$_SERVER = array("foo" => "server-bar");
$_ENV = array("foo" => "env-bar");
$_FILES = array("foo" => "files-bar");
$_REQUEST = array("foo" => "request-bar");
$_SESSION = array("foo" => "session-bar");

echo read_get(), "\n";
echo read_post(), "\n";
echo read_cookie(), "\n";
echo read_server(), "\n";
echo read_env(), "\n";
echo read_files(), "\n";
echo read_request(), "\n";
echo read_session(), "\n";

// One key set in the global scope reads back inside a function.
$_GET["baz"] = "qux";
function read_baz() { return $_GET["baz"]; }
echo read_baz(), "\n";

// The other direction: a function's writes are visible after it returns.
function replace_post() { $_POST = array("foo" => "from-function"); }
replace_post();
echo $_POST["foo"], "\n";

function write_server_key() { $_SERVER["written"] = "inside"; }
write_server_key();
echo $_SERVER["written"], "\n";
---
get-bar
post-bar
cookie-bar
server-bar
env-bar
files-bar
request-bar
session-bar
qux
from-function
inside
