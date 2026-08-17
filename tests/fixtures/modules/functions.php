<?php

function rem_post() {
	$result = array();
	$args = func_get_args();
	foreach ($args as $k) {
		if (isset($_POST[$k])) {
			$result[$k] = $_POST[$k];
		}
	}

	return $result;
}
