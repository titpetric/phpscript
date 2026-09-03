name: go plugin replaces the http bindings
description: >
  A Go plugin re-registers HTTP\Request and HTTP\Client after the standard
  library, replacing both. The marker line and driver() are what show the
  plugin served the call and not stdlib/http. The classes come from a .so, so
  php cannot run this source.
plugins: ../../testdata/plugins/http/plugin.so
runner:
  php: false
---
<?php

// No "log" flag: the plugin serves the call, but says nothing.
$quiet = new HTTP\Request("get", "https://example.invalid/quiet");
echo $quiet->driver() . "\n";

// With the flag, the plugin writes a marker line before the constructor
// returns. The native binding writes nothing, so this line is only present
// when the plugin ran.
$request = new HTTP\Request("get", "https://example.invalid/users", array("log" => true));
echo $request->method() . " " . $request->url() . "\n";

$client = new HTTP\Client(array("log" => true, "timeout" => 5));
echo $client->driver() . " " . $client->timeout() . "\n";

// The plugin's client replaces the native one wholesale, so it does not send.
try {
    $client->send($request);
    echo "sent\n";
} catch (Exception $e) {
    echo "does_not_send\n";
}

// The counter is process-global, so the fixture asserts that it moved rather
// than what it reached.
echo ($request->count() >= 2) ? "counted\n" : "not_counted\n";
?>
---
plugin
plugin: HTTP\Request GET https://example.invalid/users
GET https://example.invalid/users
plugin: HTTP\Client timeout=5
plugin 5
does_not_send
counted
