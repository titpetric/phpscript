name: an import enumerates the types its package's signatures reach
description: >
  The signatures a package binds name its own types, and the area registers
  each one as a class: `http.NewRequest` names `*http.Request`, whose fields
  name `http.Header` and the url behind it. A script constructs one with `new`,
  reads and writes its fields, calls a method of a nested value, and names it
  as the element of a slice. php has none of these names, so only phpscript
  defines this output.
runner:
  php: false
---
<?php

use Runtime\Go;

import "net/http";

// A type the walk enumerated, built empty.
$empty = new http.Request;
var_dump($empty->Method, $empty->ContentLength);

// One the binding built, read and written.
$req = http.NewRequest("GET", "https://example.com/a/b?q=1", null);
echo $req->Method, " ", $req->URL->Path, " ", $req->URL->Host, "\n";
$req->Method = "PUT";
echo $req->Method, "\n";

// A method of a nested value, and a header the request carries.
$req->Header->Set("X-Row", "7");
echo $req->Header->Get("X-Row"), "\n";
var_dump($req->URL->Query());

// The type as the element of a slice.
$rows = make([]http.Request, 2);
echo len($rows), " ", cap($rows), "\n";

// The client a script builds to send what it built, with the zero timeout go
// gives it. The binding names the type and no signature reaches it.
$client = new http.Client;
var_dump($client->Timeout);
---
string(0) ""
int(0)
GET /a/b example.com
PUT
7
array(1) {
  ["q"]=>
  array(1) {
    [0]=>
    string(1) "1"
  }
}
2 2
string(2) "0s"
