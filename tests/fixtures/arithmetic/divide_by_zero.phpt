name: division and modulo by zero
description: >
  php 8 throws DivisionByZeroError from `/` and `%` where a zero divisor
  arrives, whatever shape it arrives in: an int, a float, a numeric string,
  false or null. intdiv() throws the same class, fdiv() answers INF, -INF or
  NAN instead, and the class a script catches is the one php names.
---
<?php

function show($f)
{
	try {
		$v = $f();
		echo "value: ";
		var_dump($v);
	} catch (Throwable $e) {
		echo get_class($e), ": ", $e->getMessage(), "\n";
	}
}

show(function () { return 1 / 0; });
show(function () { return 1 % 0; });
show(function () { return 1.0 / 0; });
show(function () { return 1.5 % 0; });
show(function () { return 0 / 0; });
show(function () { return -1 / 0; });
show(function () { return 1 / 0.0; });

// A zero that arrives as a string, a bool or null.
show(function () { return 1 / "0"; });
show(function () { return 1 % "0"; });
show(function () { return 1 / false; });
show(function () { return 1 / null; });

// The compound forms raise it from the same operator.
show(function () { $x = 5; $x /= 0; return $x; });
show(function () { $x = 5; $x %= 0; return $x; });

// intdiv throws the same class; fdiv answers a float for all three signs.
show(function () { return intdiv(1, 0); });
show(function () { return fdiv(1, 0); });
show(function () { return fdiv(-1, 0); });
show(function () { return fdiv(0, 0); });

// A divisor that is not zero still divides, and the run carries on past a
// caught throw.
try {
	echo 10 / 0;
} catch (DivisionByZeroError $e) {
	echo "caught ", $e->getMessage(), "\n";
}
try {
	echo 10 % 0;
} catch (Error $e) {
	echo "caught as Error\n";
}
echo 10 / 4, " ", 10 % 4, "\n";
---
DivisionByZeroError: Division by zero
DivisionByZeroError: Modulo by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Modulo by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Modulo by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Division by zero
DivisionByZeroError: Modulo by zero
DivisionByZeroError: Division by zero
value: float(INF)
value: float(-INF)
value: float(NAN)
caught Division by zero
caught as Error
2.5 2
