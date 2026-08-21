name: template render through include
description: >
  A template engine in the MiniTPL shape dbadmin boots on every page: assign()
  collects variables on the instance, render() includes the compiled template,
  and the included file reads them back through $this. This composes two
  flatstack fallback barriers, the class declaration and the include, in the
  arrangement an application actually runs them: the include sits inside a
  method and must see the instance it was included from.
---
<?php

class Template {
	const E_FILENAME_EMPTY = "Filename can't be empty, tried to render '%s'";

	protected $vars = array();
	protected $filename = false;

	function load($filename) {
		$this->filename = "templates/" . $filename;
	}

	function assign($key, $value = '') {
		if (is_array($key)) {
			foreach ($key as $k => $v) {
				$this->vars[$k] = $v;
			}
		} else {
			$this->vars[$key] = $value;
		}
	}

	function getVar($key) {
		return isset($this->vars[$key]) ? $this->vars[$key] : false;
	}

	function render() {
		if ($this->filename === false) {
			throw new Exception(sprintf(self::E_FILENAME_EMPTY, "?"));
		}
		include($this->filename);
	}
}

$tpl = new Template;
$tpl->load("overview.php");
$tpl->assign("title", "Database overview");
$tpl->assign(array("tables" => array("invoices" => 12, "users" => 3)));
$tpl->render();

$empty = new Template;
try {
	$empty->render();
} catch (Exception $e) {
	echo "caught: ", $e->getMessage(), "\n";
}
---
Database overview
invoices: 12 row(s)
users: 3 row(s)
caught: Filename can't be empty, tried to render '?'
