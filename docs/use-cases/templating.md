# Templating

The phpscript engine is capable enough to load a fully featured template
engine. [titpetric/minitpl](https://github.com/titpetric/minitpl) runs
unmodified: it is pulled in with composer like in any PHP project, rather than
copied into this repository and patched.

```json
{
    "require": {
        "titpetric/minitpl": "^1.2"
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

Using templates and `$tpl->assign`, the arbitrary data which you can
assign to the template can be rendered into HTML or other formats. The
template engine supports the php syntax itself. A template is loaded,
compiled into PHP code, and then ran with the use of `include`.

## The model

While not templating specific, the mechanics of the language allow creating
small reusable parts similar to storage repository packages in Go.

```php
class UserStorage {
	var $db;

	UserService() {
		$db = new Database("users");
		$this->db = $db;
	}

	function getUserById($id) {
		return $this->db->get("select * from users where id=?", $id);
	}

	function getGroupList() {
		return $this->db->get_all("select * from user_groups order by name asc");
	}

	function getUserGroups($id) {
		return $this->db->get_all("select ...");
	}
}
```

This makes any request just use very clear and very minimal database
APIs which can be tested under very small scope. A complex request may
perform hundreds of database interactions based on what's being rendered.

For example, for a news CMS website, you may fetch several sections of
news articles, get their comment counts, their images, the journalist
which posted the article, and the sections sorted by editorial system of
weights. Using the PHPscript runtime makes such code much shorter than
Go with it's type safety and explicit error handling.

Go shines for CRUD-like APIs that usually fetch a single bit of data.
Once this becomes a O(N) operation, the process to compose the returned
data together can involve a lot of error handling boilerplate.

## The view

The basics are simple. A view takes many model outputs and composes them
together into a HTML page. This may mean significant amount of database
or cache interactions.

Even a simple CMS may:

- fetch navigation items for header, sidebar, footer
- render some form of "main content" area (landing page)
- mix content sources and types on a single page

For example, an edit form for an user may:

```php
<?php

// @route GET /user/{id}/edit

include("UserStorage.php");

$id = $_GET['id'];

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

$id = $_GET['id'];

$db = new UserStorage();
$db->saveUserMemberships($user_id, $_POST['user_groups']);
```

All the database interactions, including transactionality details, are
left up to the `UserStorage` class. Keep this in mind as best practice.

## References

- [Routing](./routing.md)
