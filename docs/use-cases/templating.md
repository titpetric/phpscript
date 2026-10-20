# Templating

The phpscript engine is capable enough to load a fully featured template engine. [titpetric/minitpl](https://github.com/titpetric/minitpl) runs unmodified: composer pulls it in like in any PHP project, with no copy in this repository and no patch.

```json
{
    "require": {
        "titpetric/minitpl": "^1.3"
    }
}
```

Run `composer install`, then include the autoloader and use the engine:

```php
<?php

include("vendor/autoload.php");

$tpl = new MiniTPL\Template();
$tpl->set_paths('templates/');
$tpl->set_compile_location('cache/', false);

$tpl->load('hello.tpl');
$tpl->assign('name', 'phpscript');
$tpl->render();
```

Using templates and `$tpl->assign`, the template renders the arbitrary data you assign to it into HTML or other formats. The template engine supports the php syntax itself. The engine loads a template, compiles it into PHP code, and then runs it through `include`.

## The model

While not templating specific, the mechanics of the language allow creating small reusable parts like storage repository packages in Go.

```php
class UserStorage {
	var $db;

	function __construct() {
		$this->db = new Database("app");
	}

	function getUserById($id) {
		return $this->db->get("select * from users where id = ?", $id);
	}

	function getGroupList() {
		return $this->db->get_all("select * from user_groups order by name asc");
	}

	function getUserGroups($id) {
		return $this->db->get_all("select group_id from user_group where user_id = ?", $id);
	}

	function saveUserMemberships($id, $group_ids) {
		$this->db->begin();
		$this->db->query("delete from user_group where user_id = ?", $id);
		foreach ($group_ids as $group_id) {
			$this->db->insert("user_group", array("user_id" => $id, "group_id" => $group_id));
		}
		$this->db->commit();
	}
}
```

This makes any request use database APIs that stay small and explicit: each method on `UserStorage` is one query or one transaction, so a test can cover it alone. A complex request may perform hundreds of database interactions based on what the page renders.

For example, for a news CMS website, you may fetch more than one section of news articles, get their comment counts, their images, the journalist which posted the article, and the sections sorted by editorial system of weights. Using the PHPscript runtime makes such code much shorter than Go with its type safety and explicit error handling.

Go shines for CRUD-like APIs that fetch a single bit of data. Once this becomes a O(N) operation, the process to compose the returned data together can involve a lot of error handling boilerplate.

## The view

The basics are simple. A view takes many model outputs and composes them together into a HTML page. This may mean significant amount of database or cache interactions.

Even a simple CMS may:

- fetch navigation items for header, sidebar, footer
- render some form of "main content" area (landing page)
- mix content sources and types on a single page

For example, an edit form for an user may:

```php
<?php

// @route GET /user/{id}/edit

include("UserStorage.php");

$id = $_REQUEST['id'];

$db = new UserStorage;
$user = $db->getUserById($id);
$user_groups = $db->getGroupList();

$tpl = new MiniTPL\Template;
$tpl->load("user_edit.tpl");
$tpl->assign(compact("user", "user_groups"));
$tpl->render();
```

And a POST endpoint:

```php
<?php

// @route POST /user/{id}/edit

include("UserStorage.php");

$id = $_REQUEST['id'];

$db = new UserStorage();
$db->saveUserMemberships($id, $_POST['user_groups']);
```

The `UserStorage` class owns all the database interactions, including transactionality details. Keep this in mind as best practice.

## References

- [Routing](./routing.md)
