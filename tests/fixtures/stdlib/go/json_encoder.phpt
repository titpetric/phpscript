name: encoding/json NewEncoder takes an io.Writer the go runtime binds
description: >
  NewEncoder is bound as the constructor it is and os.Stdout as the writer it
  is, so a script builds an encoder from the imported package and calls it.
  The document Encode writes is the one json_encode() writes, held to it by
  the package's own tests; the server's handlers write through it. php has no
  Go package to import, so only phpscript defines the output.
runtime: go
---
<?php

import "encoding/json";
import "os";

$enc = json.NewEncoder(os.Stdout);
echo "encoder\n";
---
encoder
