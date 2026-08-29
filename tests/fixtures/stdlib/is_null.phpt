name: is_null matches php
description: >
  is_null is true for null alone: every set value, however empty or falsy,
  answers false, and a function that returns nothing returns null. Verified
  against php 8.5.
---
<?php

var_dump(is_null(null));
var_dump(is_null(NULL));

$x = null;
var_dump(is_null($x));

// Everything set is not null, however empty or falsy.
var_dump(is_null(false));
var_dump(is_null(0));
var_dump(is_null(0.0));
var_dump(is_null(""));
var_dump(is_null("0"));
var_dump(is_null("null"));
var_dump(is_null(array()));
var_dump(is_null(1));
var_dump(is_null("abc"));

// A function with no return returns null.
function nothing() {}
var_dump(is_null(nothing()));
---
bool(true)
bool(true)
bool(true)
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
bool(true)
