name: str_contains, str_starts_with, str_ends_with
description: >
  The three PHP 8 substring predicates return booleans, and an empty needle is
  contained in, starts and ends every string.
---
<?php
var_dump(str_contains("Hello", "ell"));
var_dump(str_contains("Hello", "z"));
var_dump(str_contains("Hello", ""));
var_dump(str_starts_with("Hello", "He"));
var_dump(str_starts_with("Hello", "lo"));
var_dump(str_ends_with("Hello", "lo"));
var_dump(str_ends_with("Hello", "He"));
---
bool(true)
bool(false)
bool(true)
bool(true)
bool(false)
bool(true)
bool(false)
