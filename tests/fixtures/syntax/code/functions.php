<?php

function array_keep($arr, $keep)
{
	if (is_string($keep)) {
		$keep = array_slice(func_get_args(), 1);
	}

	$retval = array();
	foreach ($keep as $key) {
		if (isset($arr[$key])) {
			$retval[$key] = $arr[$key];
		}
	}
	return $retval;
}

function is_email($email)
{
	$trim = trim($email);
	$trim = trim($email, '.');
	if ($trim !== $email) {
		return false;
	}
	if (strpos($email, " ") !== false) {
		return false;
	}
	$a = explode("@", $email);
	if (count($a) !== 2) {
		return false;
	}
	if (strpos($email, "..") !== false) {
		return false;
	}
	if (strpos($a[1], ".") === false) {
		return false;
	}
	return true;
}

function normalize($string, $delimiter = '-')
{
	$string = mb_strtolower($string);
	$string = preg_replace("/\s+/su", "-", $string);
	return $string;
}

function location($url) {
	header("Location: $url");
	exit;
}

function dump($x) {
	echo '<pre>';
	var_dump($x);
	echo '</pre>';
}

function issetor()
{
	$args = func_get_args();
	foreach ($args as $arg) {
		if (is_string($arg) && strlen($arg) > 0) {
			return $arg;
		}
		if (is_int($arg) || is_array($arg)) {
			return $arg;
		}
		if (!empty($arg)) {
			return $arg;
		}
	}
	return false;
}

function now()
{
	return date("Y-m-d H:i:s");
}