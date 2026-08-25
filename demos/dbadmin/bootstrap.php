<?php

/**
 * The composition root. Every routed file includes this and nothing else.
 *
 * A top-level include shares the includer's scope, so the variables built here
 * are the controller's variables. That is also why nothing here uses `global`:
 * the keyword parses in this runtime and then does nothing, which is worse
 * than not having it.
 */

require "vendor/autoload.php";
require_once "lib/shim.php";
require_once "lib/dao.php";

$request_started = microtime(true);

// The object graph, leaves first. Every DAO opens its own Database("dbadmin"),
// and the provider hands all of them the same pool, so this is one connection
// wearing several hats rather than a dozen.
$audit = new audit_dao;
$users = new user_dao($audit);
$sessions = new session_dao($audit);
$groups = new user_group_dao($audit);
$connections = new connection_dao($audit);
$acl = new acl_dao($groups, $connections);
$tables = new tables_dao;
$browse = new browse_dao($tables, $audit);
$rows = new row_dao($tables, $audit);
$ddl = new ddl_dao($tables, $audit);
$console = new sql_dao($audit);
$login = new login_dao($users, $sessions, $audit);
$registration = new register_dao($users, $audit);

$tpl = new MiniTPL\Template;

// The request context: who is signed in, what they are looking at, and what
// they are allowed to do to it. Templates receive this array rather than any
// of the objects above, because the template compiler emits `global $obj` for
// an object it sees a method called on and `global` does nothing here.
$ctx = $sessions->current();
$ctx = array_merge($ctx, $acl->decide($ctx));
$ctx["flash"] = $sessions->take_flash($ctx);
$ctx["registration_open"] = $registration->is_open();

// Whether this session may write to the connection it is on. The chrome reads
// it to decide which actions to offer; open_connection() asks the same DAO
// again for the answer that actually gates the statement, so a template that
// forgot the check is a missing link and never a permitted write.
$ctx["is_readonly"] = true;
if ($ctx["connection_id"] > 0) {
	$ctx["is_readonly"] = $acl->may_use($ctx, $ctx["connection_id"])["is_readonly"];
}

/** redirect_to sends the browser to $url and ends the request. */
function redirect_to($url) {
	header("Location: " . $url);
	exit();
}

/** fail renders $message as an error page with HTTP status $status. */
function fail($ctx, $tpl, $status, $message) {
	http_response_code($status);

	$tpl->load("layout.tpl");
	$tpl->assign(array(
		"title" => "Error",
		"content" => "pane_error.tpl",
		"ctx" => $ctx,
		"nav" => "",
		"message" => $message,
		"status" => $status,
		"sidebar" => sidebar_off(),
	));
	$tpl->render();
	exit();
}

/** require_auth redirects to the login page when nobody is signed in. */
function require_auth($ctx) {
	if (!$ctx["logged_in"]) {
		redirect_to("/login");
	}
}

/** require_admin refuses a request from an account that is not an administrator. */
function require_admin($ctx, $tpl) {
	require_auth($ctx);

	if (!$ctx["is_admin"]) {
		fail($ctx, $tpl, 403, "This page is for administrators.");
	}
}

/**
 * require_csrf refuses a POST whose form did not come from this session.
 *
 * The token is compared with ===, not ==, because == would make two numeric
 * looking tokens equal if they happened to parse to the same number.
 */
function require_csrf($ctx, $tpl) {
	$sent = isset($_POST["csrf_token"]) ? (string)$_POST["csrf_token"] : "";
	if ($ctx["csrf_token"] === "" || $sent !== $ctx["csrf_token"]) {
		fail($ctx, $tpl, 403, "This form has expired. Go back, reload the page and try again.");
	}
}

/**
 * open_connection returns the client for the session's connection, or fails.
 *
 * $need_write says whether the page intends to write. A page that only reads
 * gets a read-only client, so a bug in it cannot write; that is why sqlite
 * introspection is written against pragma_table_info() rather than PRAGMA,
 * which a read-only client refuses.
 */
function open_connection($ctx, $acl, $connections, $tpl, $need_write) {
	if ($ctx["connection_id"] == 0) {
		redirect_to("/");
	}

	$grant = $acl->may_use($ctx, $ctx["connection_id"]);
	if (!$grant["allowed"]) {
		fail($ctx, $tpl, 403, "You do not have access to this connection.");
	}

	if ($need_write && $grant["is_readonly"]) {
		fail($ctx, $tpl, 403, "This connection is read-only for you.");
	}

	$connection = $connections->find($ctx["connection_id"]);
	if (!$connection) {
		fail($ctx, $tpl, 404, "That connection no longer exists.");
	}

	$opened = $connections->open($connection, !$need_write);
	if (array_key_exists("error", $opened)) {
		fail($ctx, $tpl, 502, "Could not open " . $connection["name"] . ": " . $opened["error"]);
	}

	$opened["connection"] = $connection;
	$opened["is_readonly"] = $grant["is_readonly"];
	return $opened;
}

/**
 * sidebar builds the left panel: the connection in view and its tables.
 *
 * A failure to list tables is not a failure of the page. The right panel may
 * be an administration screen that has nothing to do with the connection, so
 * the sidebar reports its own error and the page still renders.
 */
function sidebar($ctx, $acl, $connections, $tables, $active_table) {
	$panel = sidebar_off();
	$panel["connections"] = $acl->connections_for($ctx);
	$panel["active_table"] = (string)$active_table;

	if ($ctx["connection_id"] == 0) {
		return $panel;
	}

	$connection = $connections->find($ctx["connection_id"]);
	if (!$connection) {
		return $panel;
	}

	$opened = $connections->open($connection, true);
	if (array_key_exists("error", $opened)) {
		$panel["error"] = $opened["error"];
		return $panel;
	}

	try {
		$panel["tables"] = $tables->tables($opened["db"], $opened["driver"], $ctx["schema_name"]);
		if ($opened["driver"] !== "sqlite") {
			$panel["schemas"] = $tables->schemas($opened["db"], $opened["driver"]);
		}
	} catch (Exception $e) {
		$panel["error"] = $e->getMessage();
	}

	return $panel;
}

/** sidebar_off is the panel of a page with no connection behind it. */
function sidebar_off() {
	return array(
		"connections" => array(),
		"tables" => array(),
		"schemas" => array(),
		"active_table" => "",
		"error" => "",
	);
}

/**
 * path returns the request path with its query string removed.
 *
 * A module file carries several @route annotations and runs for all of them,
 * so it has to ask which one it is answering. The method separates most of
 * them; the path separates the rest.
 */
function path() {
	$uri = isset($_SERVER["REQUEST_URI"]) ? (string)$_SERVER["REQUEST_URI"] : "/";
	$at = strpos($uri, "?");
	if ($at === false) {
		return $uri;
	}

	return substr($uri, 0, $at);
}

/**
 * safe_path returns $candidate when it is a path on this site, and $fallback
 * otherwise.
 *
 * A "go back to where I was" field is a redirect target chosen by whoever
 * submitted the form. Anything that is not a single-slash absolute path is
 * somebody else's site.
 */
function safe_path($candidate, $fallback) {
	$candidate = (string)$candidate;

	if (!str_starts_with($candidate, "/") || str_starts_with($candidate, "//")) {
		return $fallback;
	}

	if (str_contains($candidate, "\\") || str_contains($candidate, "\n") || str_contains($candidate, "\r")) {
		return $fallback;
	}

	return $candidate;
}

/** is_post reports whether this is a form submission. */
function is_post() {
	return isset($_SERVER["REQUEST_METHOD"]) && strtoupper((string)$_SERVER["REQUEST_METHOD"]) === "POST";
}

/** post reads $_POST[$key] as a trimmed string, or $fallback. */
function post($key, $fallback) {
	if (!isset($_POST[$key])) {
		return $fallback;
	}

	return trim((string)$_POST[$key]);
}

/** query reads $_GET[$key] as a trimmed string, or $fallback. */
function query($key, $fallback) {
	if (!isset($_GET[$key])) {
		return $fallback;
	}

	return trim((string)$_GET[$key]);
}

/** checked reports whether a checkbox named $key was ticked. */
function checked($key) {
	return isset($_POST[$key]) && (string)$_POST[$key] !== "";
}

/** remote_addr returns the client address, for the session record. */
function remote_addr() {
	return isset($_SERVER["REMOTE_ADDR"]) ? (string)$_SERVER["REMOTE_ADDR"] : "";
}

/** user_agent returns the client's user agent, for the session record. */
function user_agent() {
	$headers = getallheaders();
	return isset($headers["User-Agent"]) ? (string)$headers["User-Agent"] : "";
}

/** table_url builds a path under the current connection's table. */
function table_url($table, $suffix) {
	return "/t/" . urlencode($table) . $suffix;
}

/**
 * render draws $pane inside the two-panel layout.
 *
 * Every variable the chrome reads is defaulted here. A template that indexes a
 * key nobody assigned is a runtime error rather than an empty string, and the
 * chrome is included by pages that have no opinion about most of it.
 */
function render($tpl, $title, $pane, $ctx, $panel, $values) {
	$defaults = array(
		"title" => $title,
		"content" => $pane,
		"ctx" => $ctx,
		"sidebar" => $panel,
		"standalone" => false,
		"back_path" => path(),
		"errors" => array(),
		"table" => "",
		"tab" => "",
		"is_readonly" => false,
	);

	$tpl->load("layout.tpl");
	$tpl->assign(array_merge($defaults, $values));
	$tpl->render();
}
