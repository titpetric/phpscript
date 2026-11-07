name: import binds a go package under the name its path ends in
description: >
  `import "net/http"` is the go spelling of `use Runtime\Go\Net\HTTP`: it
  installs the area that binds the package, and `http.StatusText(404)` reaches
  it. The statement needs the go runtime, the dot is a name only beside a
  package the file imported, and a bare name stays php's. php has no import
  statement and no Runtime namespace, so only phpscript defines this output.
runner:
  php: false
---
<?php

use Runtime\Go;

import "fmt";
import "net/http";
import "strings";
import "encoding/json";

echo fmt.Sprintf("%s=%d", "n", 42), "\n";
echo http.StatusText(404), " ", http.StatusNotFound, " ", http.MethodPost, "\n";
echo strings.ToUpper("go"), " ", strings.Index("chicken", "ken"), "\n";
echo json.Marshal(["a" => 1, "b" => [2, 3]]), "\n";
var_dump(json.Valid("{"));

// The area answers under its own name too, as every area does.
echo Runtime\Go\Strings\ToUpper("area"), "\n";

// A bare name is php's, imported package or not, and php's own strtoupper is
// not the strings package.
echo strtoupper("php"), " ", strlen("four"), "\n";

// The dot concatenates everything else, in the same file.
echo "a" . "b", " ", 1 . 2, "\n";
---
n=42
Not Found 404 POST
GO 4
{"a":1,"b":[2,3]}
bool(false)
AREA
PHP 4
ab 12
