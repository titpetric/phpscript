name: http request and client construction
description: >
  HTTP\Request and HTTP\Client are host bindings with no PHP counterpart, so
  the expected output is the runtime's contract rather than PHP's. Only
  construction and introspection are covered: building a request sends
  nothing, so this stays offline. Sending is tested against httptest in
  stdlib/http/client_test.go, because a fixture that reached the network would
  fail whenever the network did.
runner:
  php: false
---
<?php

$request = new HTTP\Request("get", "https://example.invalid/users");
echo $request->method() . "\n";
echo $request->url() . "\n";

// The setters return the request, so they chain.
$request->set_header("Accept", "application/json")
        ->set_query("page", "2")
        ->set_body("payload");

echo $request->url() . "\n";
echo $request->header("accept") . "\n";
echo $request->body() . "\n";

$headers = $request->headers();
echo $headers["Accept"] . "\n";

// A body is optional, and the method is normalised.
$post = new HTTP\Request("post", "https://example.invalid/users", '{"name":"ada"}');
echo $post->method() . "\n";
echo $post->body() . "\n";

// A client with no options is valid: it has a default timeout and follows
// redirects. Constructing without a throw is the assertion; note that a
// Go-backed binding is not is_object(), which holds for every host class and
// not just this one.
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
?>
---
GET
https://example.invalid/users
https://example.invalid/users?page=2
application/json
payload
application/json
POST
{"name":"ada"}
client
configured
rejected
rejected
