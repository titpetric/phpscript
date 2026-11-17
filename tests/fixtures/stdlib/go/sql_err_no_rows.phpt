name: database/sql ErrNoRows is a constant the script reads and throws
description: >
  A package var binds as a constant an import installs. database/sql's
  ErrNoRows arrives as the error value itself, so two reads name the same
  value and throw hands the catch clause the error go returns. php has no Go
  package to import, so only phpscript defines the output.
runtime: go
---
<?php

import "database/sql";

$e = sql.ErrNoRows;
echo $e == sql.ErrNoRows ? "same\n" : "different\n";

try {
    throw sql.ErrNoRows;
} catch (error $e) {
    echo $e, "\n";
}
---
same
sql: no rows in result set
