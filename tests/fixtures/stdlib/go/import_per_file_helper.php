<?php

use Runtime\Go;

// This file writes no import line: the entry's `strings` reached nothing here,
// and a bare name stays php's. The area itself needs no import.
try {
    echo strings.ToUpper("x"), "\n";
} catch (RuntimeException $e) {
    echo "helper: no import\n";
}

echo "helper: ", Runtime\Go\Strings\ToUpper("s"), "\n";
