<?php

// The setup hook of this suite, named by test.hooks.setup in phpscript.yml. It
// runs once per session, before any fixture in this folder.
//
// Two halves, because a fixture needs rows and not only tables. The migration
// creates the schema and mig records it, so a second run applies nothing; the
// seed is deleted and reinserted, so the fixtures below start from the same
// rows whatever state the hook found.

$migrate = new Database\Migrate("scaffold");
$migrate->load("./schema/*.up.sql");
$migrate->run();

$db = new Database("scaffold");
$db->query("delete from catalogue");

// The ids are part of the state the fixtures assert against, and a delete does
// not return them. sqlite's AUTOINCREMENT keeps its high-water mark in
// sqlite_sequence, so a second setup over the same database would seed 4, 5, 6.
$db->query("delete from sqlite_sequence where name = 'catalogue'");

$db->insert("catalogue", array("name" => "Ada", "category" => "People"));
$db->insert("catalogue", array("name" => "Grace", "category" => "People"));
$db->insert("catalogue", array("name" => "Desk Lamp", "category" => "Equipment"));
