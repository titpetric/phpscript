name: the go runtime installs no exception class to construct
description: >
  Standard library mounting registers Exception, the SPL classes and the Error
  classes for php. The go runtime registers none of them, so `new Exception`
  names an undefined class and the raised error reaches catch (error).
  php constructs the class, so only phpscript defines the expected output.
runtime: go
---
<?php

try {
    $e = new Exception("boom");
} catch (error $e) {
    echo $e, "\n";
}
---
new: undefined class "Exception"
