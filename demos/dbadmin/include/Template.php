<?php

/** Template loader modelled after go-web-crontab/frontend. */
class Template {
	var $_paths;

	var $filename;

	var $vars = array();

	function __construct($paths = false) {
		$this->set_paths($paths);
	}

	function set_paths($paths = false) {
		if ($paths === false) {
			$paths = array("templates/");
		}

		if (is_string($paths)) {
			$paths = func_get_args();
		}

		$this->_paths = $paths;
	}

	function load($filename) {
		$this->filename = $this->_paths[0] . $filename;
		return true;
	}

	function assign($key, $value = "") {
		if (is_array($key)) {
			foreach ($key as $k => $v) {
				$this->vars[$k] = $v;
			}
		} else {
			$this->vars[$key] = $value;
		}
	}

	function render() {
		if ($this->filename === false) {
			throw new Exception("Template file not loaded");
		}

		include $this->filename;
	}
}
