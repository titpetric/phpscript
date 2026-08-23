<?php

/**
 * Standard library functions phpscript does not register.
 *
 * A user-defined function shadows a builtin of the same name, so these keep
 * their PHP spellings: if the runtime grows one of them later, nothing here
 * breaks and the shim simply stops being the one that runs.
 *
 * Two rules run through the whole file. Arrays are handles, not values, so a
 * function that returns "a new array" starts from array() and a function that
 * mutates does so in the caller's array for free. And there is no ord() or
 * chr(), so nothing here can look at a byte; anything that would need to is
 * either done in SQL or not done at all.
 */

/**
 * array_key_exists reports whether $key is present in $array.
 *
 * isset() is the wrong test: it is false for a key whose value is null, and a
 * NULL column is exactly the row a database admin is looking at.
 */
function array_key_exists($key, $array) {
	if (!is_array($array)) {
		return false;
	}

	return in_array($key, array_keys($array));
}

/** array_copy returns a copy of $array that can be mutated without touching it. */
function array_copy($array) {
	if (!is_array($array)) {
		return array();
	}

	return array_merge(array(), $array);
}

/** array_push appends $value to $array in place and returns the new count. */
function array_push($array, $value) {
	$array[] = $value;
	return count($array);
}

/** array_pop removes and returns the last element of $array, or null. */
function array_pop($array) {
	$size = count($array);
	if ($size == 0) {
		return null;
	}

	$last = $array[$size - 1];

	array_splice($array, $size - 1, 1);
	return $last;
}

/** array_shift removes and returns the first element of $array, or null. */
function array_shift($array) {
	if (count($array) == 0) {
		return null;
	}

	$first = $array[0];

	array_splice($array, 0, 1);
	return $first;
}

/** array_search returns the key of the first $needle in $array, or false. */
function array_search($needle, $array) {
	foreach ($array as $key => $value) {
		if ($value === $needle) {
			return $key;
		}
	}

	return false;
}

/** array_filter returns the elements of $array for which $callback is true, keys kept. */
function array_filter($array, $callback) {
	$kept = array();
	foreach ($array as $key => $value) {
		if (call_user_func($callback, $value)) {
			$kept[$key] = $value;
		}
	}

	return $kept;
}

/** array_reduce folds $array into a single value, starting from $initial. */
function array_reduce($array, $callback, $initial) {
	$carry = $initial;
	foreach ($array as $value) {
		$carry = call_user_func($callback, $carry, $value);
	}

	return $carry;
}

/** array_reverse returns the values of $array in reverse order, reindexed. */
function array_reverse($array) {
	$values = array_values($array);
	$reversed = array();
	for ($i = count($values) - 1; $i >= 0; $i -= 1) {
		$reversed[] = $values[$i];
	}

	return $reversed;
}

/** array_column returns column $key from every row of $rows, as a list. */
function array_column($rows, $key) {
	$column = array();
	foreach ($rows as $row) {
		if (array_key_exists($key, $row)) {
			$column[] = $row[$key];
		}
	}

	return $column;
}

/**
 * array_index keys $rows by the value of their $key column.
 *
 * PHP spells this as array_column()'s third argument. Optional arguments that
 * change a return type read worse than a second name, and this runtime has no
 * named arguments to soften it.
 */
function array_index($rows, $key) {
	$indexed = array();
	foreach ($rows as $row) {
		if (array_key_exists($key, $row)) {
			$indexed[$row[$key]] = $row;
		}
	}

	return $indexed;
}

/** array_flip returns $array with its keys and values exchanged. */
function array_flip($array) {
	$flipped = array();
	foreach ($array as $key => $value) {
		$flipped[$value] = $key;
	}

	return $flipped;
}

/** array_sum returns the sum of the values in $array. */
function array_sum($array) {
	$total = 0;
	foreach ($array as $value) {
		$total = $total + $value;
	}

	return $total;
}

/** range returns the integers from $low to $high inclusive. */
function range($low, $high) {
	$out = array();
	for ($i = $low; $i <= $high; $i += 1) {
		$out[] = $i;
	}

	return $out;
}

/**
 * ksorted returns $array with its keys sorted.
 *
 * PHP's ksort() sorts in place through a reference, and references outside
 * foreach are silently dropped here, so this returns instead of mutating.
 */
function ksorted($array) {
	$keys = array_keys($array);

	sort($keys);

	$sorted = array();
	foreach ($keys as $key) {
		$sorted[$key] = $array[$key];
	}

	return $sorted;
}

/** abs returns the magnitude of $number. */
function abs($number) {
	if ($number < 0) {
		return 0 - $number;
	}

	return $number;
}

/** min2 returns the smaller of $a and $b. */
function min2($a, $b) {
	return ($a < $b) ? $a : $b;
}

/** max2 returns the larger of $a and $b. */
function max2($a, $b) {
	return ($a > $b) ? $a : $b;
}

/** floor_int returns the largest integer not greater than $number. */
function floor_int($number) {
	$whole = (int)$number;
	if ($number < 0 && $whole != $number) {
		return $whole - 1;
	}

	return $whole;
}

/** ceil_int returns the smallest integer not less than $number. */
function ceil_int($number) {
	return 0 - floor_int(0 - $number);
}

/**
 * div_ceil returns $a divided by $b, rounded up, and 0 when $b is 0.
 *
 * Integer division by zero returns 0 in this runtime rather than raising, so
 * the guard is here to make a page count of zero a decision rather than an
 * accident.
 */
function div_ceil($a, $b) {
	if ($b == 0) {
		return 0;
	}

	return ceil_int($a / $b);
}

/** str_contains reports whether $needle occurs in $haystack. */
function str_contains($haystack, $needle) {
	if ($needle === "") {
		return true;
	}

	return strpos($haystack, $needle) !== false;
}

/** str_starts_with reports whether $haystack begins with $needle. */
function str_starts_with($haystack, $needle) {
	return substr($haystack, 0, strlen($needle)) === $needle;
}

/** str_ends_with reports whether $haystack ends with $needle. */
function str_ends_with($haystack, $needle) {
	if ($needle === "") {
		return true;
	}

	return substr($haystack, 0 - strlen($needle)) === $needle;
}

/** ucfirst returns $string with its first byte upper-cased. */
function ucfirst($string) {
	if ($string === "") {
		return "";
	}

	return strtoupper(substr($string, 0, 1)) . substr($string, 1);
}

/** str_pad_left returns $string padded with $pad on the left to $length. */
function str_pad_left($string, $length, $pad) {
	while (strlen($string) < $length) {
		$string = $pad . $string;
	}

	return $string;
}

/** number_format_int returns $number with thousands separated by $separator. */
function number_format_int($number, $separator) {
	$digits = (string)(int)$number;
	$sign = "";
	if (str_starts_with($digits, "-")) {
		$sign = "-";
		$digits = substr($digits, 1);
	}

	$grouped = "";
	$length = strlen($digits);
	for ($i = 0; $i < $length; $i += 1) {
		if ($i > 0 && ($length - $i) % 3 == 0) {
			$grouped = $grouped . $separator;
		}

		$grouped = $grouped . substr($digits, $i, 1);
	}

	return $sign . $grouped;
}

/**
 * urlencode percent-encodes $value for use in a URL.
 *
 * There is no ord(), so a general encoder cannot be written: the map below is
 * the set of bytes that have to be escaped in a query string, and anything
 * outside [A-Za-z0-9_.-] that is not in it is rejected by url_safe() rather
 * than passed through wrongly.
 */
function urlencode($value) {
	$map = array(
		"%" => "%25",
		" " => "+",
		"!" => "%21",
		"\"" => "%22",
		"#" => "%23",
		"$" => "%24",
		"&" => "%26",
		"'" => "%27",
		"(" => "%28",
		")" => "%29",
		"*" => "%2A",
		"+" => "%2B",
		"," => "%2C",
		"/" => "%2F",
		":" => "%3A",
		";" => "%3B",
		"<" => "%3C",
		"=" => "%3D",
		">" => "%3E",
		"?" => "%3F",
		"@" => "%40",
		"[" => "%5B",
		"\\" => "%5C",
		"]" => "%5D",
		"^" => "%5E",
		"`" => "%60",
		"{" => "%7B",
		"|" => "%7C",
		"}" => "%7D",
		"~" => "%7E",
		"\n" => "%0A",
		"\r" => "%0D",
		"\t" => "%09",
	);

	return strtr((string)$value, $map);
}

/** url_safe reports whether $value survives urlencode() without loss. */
function url_safe($value) {
	return preg_match("/^[A-Za-z0-9 _.:@!*'(),$&+=?#\\/\\[\\]-]*$/", (string)$value) == 1;
}

/** http_build_query encodes $params as a query string. */
function http_build_query($params) {
	$pairs = array();
	foreach ($params as $key => $value) {
		if ($value === null || $value === "") {
			continue;
		}

		$pairs[] = urlencode($key) . "=" . urlencode($value);
	}

	return implode("&", $pairs);
}

/** file_put_contents writes $contents to $filename and returns the byte count. */
function file_put_contents($filename, $contents) {
	$handle = fopen($filename, "w");
	if (!$handle) {
		return false;
	}

	fwrite($handle, $contents);
	fclose($handle);
	return strlen($contents);
}
