<?php

$databases = array("sqlite", "mysql", "postgres");

if ($_GET['db']) {
	$databases = array($_GET['db']);
}

echo '<table border="1">';
echo '<tr>';

foreach ($databases as $dbname) {
	echo '<td>' . $dbname . '</td>';
}

echo '</tr>';

echo '<tr>';

foreach ($databases as $dbname) {
	echo '<td><pre>';
	if ($dbname == "postgres") {
		$db = new Database("postgres_test");

		echo json_encode($db->get_all("select datname from pg_database where datistemplate = false;"));

		echo json_encode($db->get("show max_connections;"));

		$db->close();
	}

	if ($dbname == "mysql") {
		$db = new Database("mysql_test");

		echo json_encode($db->get_all("show databases;"));

		$db->close();
	}

	if ($dbname == "sqlite") {
		$db = new Database("sqlite_test");
		$db->query("PRAGMA journal_mode = WAL;");
		$db->query("PRAGMA synchronous = NORMAL;");
		$db->query("PRAGMA busy_timeout = 1000;");

		try {
			$db->get("select count(id) from users");
		} catch ($e) {
			$db->query("drop table if exists users");
			$db->query("create table users (id integer primary key autoincrement, name text)");
			$db->insert("users", array("name" => "Ada"));
			$db->insert("users", array("name" => "Grace"));
		}

		$row = $db->get("select id, name from users where name = ?", "Ada");
		$users = $db->get_all("select name from users order by id");

		echo $row["name"] . "#" . $row["id"] . "\n";
		foreach ($users as $row) {
			echo $row["name"] . "\n";
		}

		$db->close();
	}
	echo '</pre></td>';
}

echo '</tr>';
