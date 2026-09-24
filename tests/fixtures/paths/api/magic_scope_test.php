<?php

// __LINE__ is the line it is written on, not the line of the call it reaches.
echo __LINE__, "\n";
echo strlen("x"), ":", __LINE__, "\n";

// The path constants name the file the body was read from.
echo basename(__FILE__), "\n";
echo basename(__DIR__), "\n";

// Outside any function or class, php answers all three as empty strings.
echo "[", __FUNCTION__, "][", __CLASS__, "][", __METHOD__, "]\n";

function freeFunction() {
	return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
}
echo freeFunction(), "\n";

class Widget {
	public function instance() {
		return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
	}
	public static function statically() {
		return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
	}
	public function line() {
		return __LINE__;
	}
}
$w = new Widget();
echo $w->instance(), "\n";
echo Widget::statically(), "\n";
echo $w->line(), "\n";

// A name is resolved where it is written, so a method returning __CLASS__
// answers its own class whoever calls it.
function ask(Widget $w) {
	return $w->instance() . "|" . __FUNCTION__;
}
echo ask($w), "\n";
