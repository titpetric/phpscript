name: http request and client construction
description: >
  HTTP\Request and HTTP\Client are host bindings with no PHP counterpart, so
  the expected output is the runtime's contract rather than PHP's. A request is
  a net/http request, so this covers the Go spellings a script uses to read and
  write one. Only construction and introspection are covered: building a
  request sends nothing, so this stays offline. Sending, including parallel(),
  is tested against httptest in stdlib/http/client_test.go, because a fixture
  that reached the network would fail whenever the network did.
runner:
  php: false
---
<?php

$request = new HTTP\Request("GET", "https://example.invalid/users?page=1");

// A request is a net/http request, so its fields are read the way Go names
// them, matched case-insensitively.
echo $request->method . "\n";
echo $request->host . "\n";
echo $request->url->path . "\n";
echo $request->url->string() . "\n";
echo $request->proto . "\n";

// Headers go through net/http's own Header methods.
$request->header->set("Accept", "application/json");
$request->header->add("X-Trace", "abc");
echo $request->header->get("accept") . "\n";

// A body is optional, and its length is set on the request.
$post = new HTTP\Request("POST", "https://example.invalid/users", '{"name":"ada"}');
echo $post->method . "\n";
echo $post->contentlength . "\n";

// A client with no options is valid: it has a default timeout and follows
// redirects. Constructing without a throw is the assertion; note that a
// Go-backed binding is not is_object(), which holds for every host class.
try {
    new HTTP\Client();
    echo "client\n";
} catch (Exception $e) {
    echo "no_client\n";
}

try {
    new HTTP\Client(array(
        "timeout"          => 5,
        "base_url"         => "https://example.invalid",
        "follow_redirects" => false,
        "user_agent"       => "phpscript/1",
        "headers"          => array("Authorization" => "Bearer token"),
    ));
    echo "configured\n";
} catch (Exception $e) {
    echo "not_configured\n";
}

// An unknown option is a mistake in the script, not a value to ignore.
try {
    new HTTP\Client(array("timeuot" => 5));
    echo "accepted\n";
} catch (Exception $e) {
    echo "rejected\n";
}

// A request needs a method and a url.
try {
    new HTTP\Request("", "https://example.invalid");
    echo "accepted\n";
} catch (Exception $e) {
    echo "rejected\n";
}

// parallel() takes an array of HTTP\Request keyed by name. Anything else is
// reported before a connection is opened.
$client = new HTTP\Client();
try {
    $client->parallel(array("users" => "not a request"));
    echo "accepted\n";
} catch (Exception $e) {
    echo "rejected\n";
}
?>
---
GET
example.invalid
/users
https://example.invalid/users?page=1
HTTP/1.1
application/json
POST
14
client
configured
rejected
rejected
rejected
