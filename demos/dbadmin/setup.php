<?php

// The setup every page shares: the connection, the template engine, the
// session and the user database, and the helpers the controllers call.
//
// A page behind the login includes bootstrap.php, which is this file plus the
// gate. The sign-in page and the sign-out route include this one, because
// neither can require a session: one is how you get a session and the other is
// how you end it.
require "vendor/autoload.php";

$request_start_time = microtime(true);

$db = new Database("dbadmin");
$tpl = new MiniTPL\Template;

// The user database is the .htpasswd file in this directory, and the session
// is a cookie over it. Both are the defaults, so neither constructor names
// one: the manager reads .htpasswd and keeps sessions on disk, so a reload
// leaves everybody signed in.
//
// No password and no hash appears in this application. The runtime reads the
// file, authenticate() answers a record or nothing, and a session carries the
// user's id.
$session = new Session\Manager;
$auth = new Auth\Storage\PasswordFile;

// Every page renders the same header, and the sign-in page renders it with
// nobody signed in. add_default and not assign: load() clears the assigned
// variables and re-applies the defaults, so a value no controller knows about
// survives only as a default.
$tpl->add_default("username", "");
$tpl->add_default("user_id", "");

/**
 * Sends a visitor without a session to the sign-in page, and tells the
 * template who they are.
 *
 * bootstrap.php calls it, so every page that includes bootstrap.php is behind
 * the login and a page added later is behind it too.
 */
function require_login($session, $db, $tpl) {
	if (!$session->valid()) {
		redirect_to("/login");
	}

	$user = current_user($db, $session);
	$tpl->add_default("username", $user["username"]);
	$tpl->add_default("user_id", $user["id"]);
}

/**
 * The signed-in user's row, read by the id the session carries.
 *
 * The id is a ULID computed from the username, so the user table is a view of
 * the password file and this is an ordinary primary-key read. A session whose
 * account the file has since dropped resolves to nothing, and the header
 * prints the id in place of a name.
 */
function current_user($db, $session) {
	$id = $session->get();
	$row = $db->get("SELECT id, username FROM \"user\" WHERE id = ?", $id);
	if (!$row) {
		return array("id" => $id, "username" => $id);
	}

	return $row;
}

function format_memory_bytes($bytes) {
	if ($bytes >= 1048576) {
		return sprintf("%.2f MB", $bytes / 1048576);
	}
	if ($bytes >= 1024) {
		return sprintf("%.2f KB", $bytes / 1024);
	}
	return $bytes . " B";
}

function render_footer_stats() {
	global $request_start_time;
	$duration = microtime(true) - $request_start_time;
	$memory = memory_get_usage();
	return sprintf("page rendered in %.4f seconds, used %s memory", $duration, format_memory_bytes($memory));
}

// There is no h() here, and no template names an escape filter: minitpl
// escapes by markup context on its own, so a value crossing a second escaper
// reaches the page as &amp;quot; where a quote belongs. The three templates
// that show a null marker spell it themselves.
function identifier_ok($name) {
	return preg_match("/^[A-Za-z_][A-Za-z0-9_]*$/", $name) == 1;
}

function qi($name) {
	if (!identifier_ok($name)) {
		die("Invalid SQL identifier.");
	}

	return "\"" . $name . "\"";
}

function table_info($db, $table) {
	if (!identifier_ok($table)) {
		die("Invalid table name.");
	}

	$found = $db->get("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name = ? AND name NOT LIKE 'sqlite_%'", $table);
	if (!$found) {
		die("Table not found.");
	}

	return $found;
}

function columns_for($db, $table) {
	table_info($db, $table);
	return $db->get_all("PRAGMA table_info(" . qi($table) . ")");
}

function redirect_to($url) {
	header("Location: " . $url);
	exit();
}

function csv_cell($value) {
	if ($value === null) {
		$value = "";
	}

	return "\"" . str_replace("\"", "\"\"", "" . $value) . "\"";
}

/**
 * Runs a console statement and returns the message, rows and column names.
 *
 * query() reports success as a boolean, so statements that return rows are
 * read with get_all() instead. Rows arrive as maps without a column order;
 * this sorts the names to keep the rendered table stable between requests.
 */
function sql_execute($db, $query) {
	$prefix = strtolower(ltrim($query));
	$returns_rows = strpos($prefix, "select") === 0 || strpos($prefix, "pragma") === 0 || strpos($prefix, "with") === 0 || strpos($prefix, "explain") === 0;
	if (!$returns_rows) {
		$db->query($query);
		return array("message" => "Statement executed successfully.", "rows" => array(), "result_columns" => array());
	}

	$rows = $db->get_all($query);
	$result_columns = array();
	if (count($rows) > 0) {
		$result_columns = array_keys($rows[0]);
		sort($result_columns);
	}

	return array("message" => "Query completed; " . count($rows) . " row(s) returned.", "rows" => $rows, "result_columns" => $result_columns);
}
