name: http request and client construction
description: >
  Building a request and a client, and reading the request back through the
  net/http fields a script uses. Sending is covered in
  stdlib/http/client_test.go against httptest. HTTP\Request and HTTP\Client
  have no PHP counterpart, so php cannot run this source.
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
// redirects. Constructing without a throw is what this asserts, because the
// settings it applies are only observable once a request is sent, which
// stdlib/http/client_test.go covers.
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
