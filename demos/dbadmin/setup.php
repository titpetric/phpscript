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

// The user database is the .htpasswd file in this directory. The session is a
// cookie over it. Both are the defaults and neither constructor names one: the
// manager reads .htpasswd and keeps sessions on disk, which leaves everybody
// signed in across a reload.
//
// No password and no hash appears in this application. The runtime reads the
// file and authenticate() answers a record or nothing. The session holds the
// user's id.
$session = new Session\Manager;
$auth = new Auth\Storage\PasswordFile;

// Every page renders the same header, and the sign-in page renders it with
// nobody signed in. add_default and not assign: load() clears the assigned
// variables and re-applies the defaults, and a value no controller knows about
// survives only as a default.
$tpl->add_default("username", "");
$tpl->add_default("user_id", "");

// The sign-in and sign-out forms hold this token in a hidden field, and the
// same value travels in a cookie; csrf_check() accepts a post only when the
// two halves agree. A default, because header.tpl renders the sign-out form on
// every page.
$tpl->add_default("csrf", csrf_token());

/**
 * Sends a visitor without a session to the sign-in page, and tells the
 * template who they are.
 *
 * bootstrap.php calls it, and every page that includes bootstrap.php is behind
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
 * The signed-in user's row, read by the id the session holds.
 *
 * The id is a ULID computed from the username, and the user table is a view of
 * the password file and this is an ordinary primary-key read. A session whose
 * account the file has since dropped resolves to nothing, and header.tpl
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

/**
 * The footer's render time and heap use, measured from $start_time.
 *
 * The caller passes the start time. `global` parses here and binds nothing
 * (docs/design.md), and the subtraction against an unset name printed the
 * epoch as the duration.
 */
function render_footer_stats($start_time) {
	$duration = microtime(true) - $start_time;
	$memory = memory_get_usage();
	return sprintf("%.4f s · %s", $duration, format_memory_bytes($memory));
}

/**
 * The visitor's form token: the value of the csrf cookie, or a fresh one the
 * response stages as that cookie when the request sends none.
 *
 * The pair is the double-submit pattern. A page on another origin can neither
 * read the cookie nor set it nor write a form whose hidden field matches. The
 * token needs no store on the server. HttpOnly holds, because the server
 * renders the token into the form and no script reads the cookie.
 */
function csrf_token() {
	if (isset($_COOKIE["csrf"]) && preg_match("/^[0-9a-f]{32}$/", $_COOKIE["csrf"]) == 1) {
		return $_COOKIE["csrf"];
	}

	$token = bin2hex(random_bytes(16));
	setcookie("csrf", $token, array("path" => "/", "httponly" => true, "samesite" => "Lax"));
	return $token;
}

/**
 * Whether the posted csrf field agrees with the csrf cookie. A post missing
 * either half fails. The comparison is hash_equals, which reports that the two
 * differ and not where.
 */
function csrf_check() {
	if (!isset($_COOKIE["csrf"]) || !isset($_POST["csrf"]) || $_COOKIE["csrf"] == "") {
		return false;
	}

	return hash_equals($_COOKIE["csrf"], $_POST["csrf"]);
}

// There is no h() here, and no template names an escape filter: minitpl
// escapes by markup context unprompted, and a value crossing a second escaper
// reaches the page as &amp;quot; where a quote belongs. The three templates
// that show a null marker spell it themselves.
function identifier_ok($name) {
	return preg_match("/^[A-Za-z_][A-Za-z0-9_]*$/", $name) == 1;
}

function qi($name) {
	if (!identifier_ok($name)) {
		die("Invalid SQL identifier");
	}

	return "\"" . $name . "\"";
}

function table_info($db, $table) {
	if (!identifier_ok($table)) {
		die("Invalid table name");
	}

	$found = $db->get("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name = ? AND name NOT LIKE 'sqlite_%'", $table);
	if (!$found) {
		die("Table not found");
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
 * query() reports success as a boolean. get_all() reads a statement that
 * returns rows and answers maps without a column order; this sorts the names to
 * keep the rendered table stable between requests.
 */
function sql_execute($db, $query) {
	$prefix = strtolower(ltrim($query));
	$returns_rows = strpos($prefix, "select") === 0 || strpos($prefix, "pragma") === 0 || strpos($prefix, "with") === 0 || strpos($prefix, "explain") === 0;
	if (!$returns_rows) {
		$db->query($query);
		return array("message" => "Statement executed", "rows" => array(), "result_columns" => array());
	}

	$rows = $db->get_all($query);
	$result_columns = array();
	if (count($rows) > 0) {
		$result_columns = array_keys($rows[0]);
		sort($result_columns);
	}

	return array("message" => count($rows) . " row(s)", "rows" => $rows, "result_columns" => $result_columns);
}
