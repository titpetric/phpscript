---
name: user-friendly sqlite adapter
description: Exercises PS\SQLite bridge for memory-bound CRUD operations, transactional safety, and query helpers.
---
<?php

$db = new PS\SQLite();

$db->Execute("CREATE TABLE products (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, price REAL, stock INT)");

$id1 = $db->Insert("products", array("name" => "Keyboard", "price" => 89.99, "stock" => 15));
$id2 = $db->Insert("products", array("name" => "Mouse", "price" => 49.99, "stock" => 30));

$product = $db->First("SELECT name, price FROM products WHERE id = ?", array($id1));
echo $product["name"] . ":" . $product["price"] . "\n";

$db->Update("products", array("stock" => 25), "id = ?", array($id2));

$all = $db->All("SELECT name, stock FROM products ORDER BY id ASC");
foreach ($all as $item) {
    echo $item["name"] . "=" . $item["stock"] . ";";
}
echo "\n";

$db->Delete("products", "id = ?", array($id1));

$remaining = $db->All("SELECT name FROM products");
echo "remaining=" . count($remaining);

$db->Close();
?>
---
Keyboard:89.99
Keyboard=15;Mouse=25;
remaining=1
