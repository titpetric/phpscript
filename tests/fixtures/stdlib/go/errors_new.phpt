name: errors.New throws a go error the go runtime catches as error
description: >
  The go runtime has one failure value, go's error, and no throwable class.
  A script imports go's errors package, throws the value it constructs and a
  catch clause declaring error binds it. echo renders the error's message the
  way go does. php installs no errors package here and its catch clauses name
  a class, so only phpscript defines the expected output.
runtime: go
---
<?php

import "errors";

try {
    throw errors.New("boom");
} catch (error $e) {
    echo $e, "\n";
}
---
boom
