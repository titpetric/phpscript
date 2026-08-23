name: class constants
description: >
  Constants declared with const, read as self::NAME inside the class and
  Class::NAME outside it, used in sprintf messages and as keys when a property
  default is built. This is the MiniTPL shape dbadmin runs: Template declares
  its error strings as constants and keys its hooks array with
  Hook::POSITION_PRE and Hook::POSITION_POST. A class declaration selects the
  interpreter fallback in flatstack; this fixture holds both engines to the
  same output.
---
<?php

class Hook {
	const POSITION_PRE = "pre";
	const POSITION_POST = "post";
}

class Template {
	const E_TEMPLATE_COMPILE = "Template file '%s' doesn't exist! Is the compile dir writable?";
	const E_FILENAME_EMPTY = "Filename can't be empty, tried to render '%s'";

	protected $hooks;

	function __construct() {
		$this->hooks = array(
			Hook::POSITION_PRE => array(),
			Hook::POSITION_POST => array()
		);
	}

	function add_hook($hook, $position) {
		$this->hooks[$position][] = $hook;
	}

	function hook_count($position) {
		return count($this->hooks[$position]);
	}

	function missing($filename) {
		return sprintf(self::E_TEMPLATE_COMPILE, $filename);
	}
}

$tpl = new Template;
$tpl->add_hook("trim", Hook::POSITION_PRE);
$tpl->add_hook("minify", Hook::POSITION_POST);
$tpl->add_hook("gzip", Hook::POSITION_POST);

echo $tpl->hook_count(Hook::POSITION_PRE), ":", $tpl->hook_count(Hook::POSITION_POST), "\n";
echo $tpl->missing("dashboard.tpl"), "\n";
echo sprintf(Template::E_FILENAME_EMPTY, "footer.tpl"), "\n";
echo Hook::POSITION_PRE === "pre" ? "match" : "differ", "\n";
---
1:2
Template file 'dashboard.tpl' doesn't exist! Is the compile dir writable?
Filename can't be empty, tried to render 'footer.tpl'
match
