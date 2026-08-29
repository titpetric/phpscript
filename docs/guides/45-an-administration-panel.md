# An administration panel

`/admin` is a dashboard of tiles, one per panel the visitor may see, and each panel owns a
group of routes under its own prefix. This chapter builds that surface: what a panel class
declares, how the navigation and the dashboard are assembled, and how each route decides
whether it is public or private. At the end of it you can add a panel of your own and drive
the whole thing with curl.

The working code is `demos/common-phpscript`. Start it with
`phpscript -f config.yml server .` and sign in at `/admin/login` with the seeded `admin`
account, which `migrate.php` prints on the first start.

## The shape of the surface

Three panels are registered: users, groups and permissions. Each declares its own URL prefix
and carries its own routes below it. `phpscript list ./routes/...` prints the whole table,
and it is the table to read when you want to know what the panel answers.

| Method | Path                                       | File                                  |
|--------|--------------------------------------------|---------------------------------------|
| GET    | `/admin`                                   | `routes/admin-dashboard.php`          |
| GET    | `/admin/login`                             | `routes/admin-auth-form.php`          |
| POST   | `/admin/login`                             | `routes/admin-auth-login.php`         |
| POST   | `/admin/logout`                            | `routes/admin-auth-logout.php`        |
| GET    | `/admin/user`                              | `routes/admin-users-list.php`         |
| GET    | `/admin/user/new`                          | `routes/admin-users-new.php`          |
| POST   | `/admin/user`                              | `routes/admin-users-create.php`       |
| GET    | `/admin/user/{id}`                         | `routes/admin-users-edit.php`         |
| POST   | `/admin/user/{id}`                         | `routes/admin-users-save.php`         |
| POST   | `/admin/user/{id}/delete`                  | `routes/admin-users-delete.php`       |
| POST   | `/admin/user/{id}/password`                | `routes/admin-users-password.php`     |
| POST   | `/admin/user/{id}/group`                   | `routes/admin-users-group-add.php`    |
| POST   | `/admin/user/{id}/group/{group_id}/delete` | `routes/admin-users-group-remove.php` |
| GET    | `/admin/group`                             | `routes/admin-groups-list.php`        |
| GET    | `/admin/group/new`                         | `routes/admin-groups-new.php`         |
| POST   | `/admin/group`                             | `routes/admin-groups-create.php`      |
| GET    | `/admin/group/{id}`                        | `routes/admin-groups-edit.php`        |
| POST   | `/admin/group/{id}`                        | `routes/admin-groups-save.php`        |
| POST   | `/admin/group/{id}/delete`                 | `routes/admin-groups-delete.php`      |
| GET    | `/admin/permission`                        | `routes/admin-permissions-list.php`   |
| GET    | `/admin/permission/{module}/{group_id}`    | `routes/admin-permissions-edit.php`   |
| POST   | `/admin/permission/{module}/{group_id}`    | `routes/admin-permissions-save.php`   |

Every write is a POST, including the deletes, because an HTML form emits GET and POST and
nothing else. A GET route renders `templates/<stem>.tpl`; a POST route has no template, it
writes, sets a flash message and redirects.

## Decide public and private one route at a time

`/admin/login` is public. It has to be: it is the screen that turns an anonymous visitor
into a signed-in one, and a guard in front of it would have nothing to let anyone past.
Every other admin route calls `require_login($session)` as its first statement inside the
`try`, then `require_can(...)` for the key it needs.

Put the two openings side by side. The public one, `routes/admin-auth-form.php`:

```php
// @route GET /admin/login

include "bootstrap.php";

try {
	$remote_addr = isset($_SERVER["REMOTE_ADDR"]) ? $_SERVER["REMOTE_ADDR"] : "";
	$user_agent = isset($_SERVER["HTTP_USER_AGENT"]) ? $_SERVER["HTTP_USER_AGENT"] : "";
	$context = $session->ensure($remote_addr, $user_agent);
```

The private one, `routes/admin-users-list.php`:

```php
// @route GET /admin/user

include "bootstrap.php";

try {
	require_login($session);
	require_can($html, $rules["user"], "user.list", array("0"));
```

The line that differs is `require_login($session)`. It reads the session context, and when
`is_authenticated` is false it sends a 302 to `/admin/login?next=<the current URI>` and
exits. `require_can()` comes second and answers 403 through the error template when the
permission engine denies the key. The login form calls neither, and it calls `ensure()`
rather than `current()` because a CSRF token lives on a session row, a visitor with no row
would be handed the empty string, and `Common\Csrf::check()` refuses that.

The guard is a call in the file, not configuration. Nothing in the runtime binds a route to
a policy, so a route file that does not call `require_login()` is public. Delete that one
line from a panel route and the route serves anyone:

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/admin/page
200
```

A forgotten guard is a silent hole: no error, no warning, no lint finding. This application
has 22 routed files and 49 guard calls, and the mapping between them exists only inside the
files. `../../demos/common-report/proposal-annotation-auth.md` specifies the fix: an `@auth`
declaration the tooling can read, plus the `phpscript lint` rule that makes an undeclared
route a finding. Until that exists, review the first three lines of every route file you add.

## Add a panel

A panel is one class implementing `Common\ModuleInfo` and `Common\AdminPanel`, registered in
`bootstrap.php`, plus a route file and a template per screen. Here is a complete small one:

```php
<?php

namespace App;

use Common\AdminPanel;
use Common\ModuleInfo;

class PagesAdmin implements ModuleInfo, AdminPanel
{
	private $pages;

	public function __construct(Pages $pages)
	{
		$this->pages = $pages;
	}

	public function name() { return "page"; }
	public function title() { return "Pages"; }
	public function icon() { return "file"; }

	public function roles()
	{
		return array(
			"page" => array(
				"list" => "List pages",
				"edit" => "Edit pages",
			),
		);
	}

	public function sections_by_role($role) { return array(); }

	public function base() { return "/admin/page"; }
	public function prefix() { return "admin-pages"; }

	public function entries()
	{
		return array(
			array("title" => "All pages", "href" => "/admin/page", "permission" => "page.list"),
		);
	}

	public function summary()
	{
		return array(
			array("label" => "Pages", "value" => $this->pages->count()),
		);
	}
}
```

`roles()` is the permission keys the route files check, and an empty `sections_by_role()`
means every key is evaluated module wide. Declare all nine methods: there is no inheritance
here and an interface carries no method body, so a missing one is a lint failure rather than
an inherited default. See [Structuring an application](25-structuring-an-application.md).

Register it by writing the class name down, in the composition root:

```php
$pages = new App\Pages();

$panels = array(
	new App\PagesAdmin($pages),
	new Common\UsersAdmin($users, $groups),
	new Common\GroupsAdmin($groups),
	new Common\RulesAdmin($groups),
);
```

The name has to be literal. `new $class` is a parse error here, so a filesystem scan cannot
construct anything it finds. The registry is a list for that reason, and the list is also
where a panel's collaborators are named, which a scan could not have supplied either.

Then add one route file and one template per screen, named after `prefix()`:
`routes/admin-pages-list.php` renders `templates/admin-pages-list.tpl`, guarded by
`require_login($session)` and `require_can($html, $rules["page"], "page.list", array("0"))`.
The shape is the one walked in "One screen end to end" below.

The panel now appears in the navigation, on the dashboard with its tile rows, and at its own
URL. `migrate.php` writes the new module row on the next start, which is what the count in
`migrate: 4 modules registered` reports.

## The three names a panel carries

Three methods return names, and none is derived from another:

| Method     | Value         | What it is                                                                                                                                                                           |
|------------|---------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name()`   | `user`        | The module name, singular. The key in the `module` table, the `b.name` bind in the permission query, and the first segment of every permission key the panel declares (`user.edit`). |
| `base()`   | `/admin/user` | The URL prefix the panel answers under. The dashboard tile and the navigation link point at it.                                                                                      |
| `prefix()` | `admin-users` | The route file and template stem, plural, named after the component class, so `routes/admin-users-edit.php` renders `templates/admin-users-edit.tpl`.                                |

Deriving `prefix()` from `name()` would put a pluralisation rule in the middle of the routing
table, and deriving `base()` from either would fix the URL space to the module name. Each is
declared, and each is one string in one method you can read.

## Build the navigation once

A first loop in `bootstrap.php` declares each panel to `Common\Modules` and builds one
`Common\Rules` per panel, keyed by module name, so a route file can index it with the literal
name of its own panel as in `$rules["user"]`. The menu is the loop after it:

```php
$nav = array();
foreach ($panels as $panel) {
	if (!$rules[$panel->name()]->can($panel->name() . ".list", array("0"))) {
		continue;
	}

	$nav[] = array(
		"name" => $panel->name(),
		"title" => $panel->title(),
		"icon" => $panel->icon(),
		"href" => $panel->base(),
		"entries" => $panel->entries(),
	);
}
```

Filtering happens here rather than in the template, so a panel the visitor cannot list is
absent from the menu instead of being drawn as a link that answers 403.

The section list is `array("0")`, the module-wide default, written out rather than left off.
`Common\Rules` refuses to default the section of a dotted key; see
[Groups and permissions](40-groups-and-permissions.md). Listing a panel is the module-wide
question, and every list route repeats the same check.

## Draw the dashboard

`AdminPanel::summary()` costs a query per panel, so the composition root does not call it.
`routes/admin-dashboard.php` does, once per tile, and only for the panels `$nav` kept:

```php
	$tiles = array();
	foreach ($nav as $entry) {
		$panel = $modules->find($entry["name"]);
		if ($panel === false) {
			continue;
		}

		$tiles[] = array(
			"title" => $entry["title"],
			"icon" => $entry["icon"],
			"href" => $entry["href"],
			"entries" => $entry["entries"],
			"rows" => $panel->summary(),
		);
	}
```

`Modules::find()` returns the object that was declared, which is the same instance that
implements `AdminPanel`, so `summary()` is reachable off it without a second registry.

## One screen end to end

Take the user edit screen. `routes/admin-users-edit.php` is the GET half, minus the `catch`
block every route file repeats:

```php
// @route GET /admin/user/{id}

include "bootstrap.php";

try {
	require_login($session);

	$user = $users->find($_REQUEST["id"]);
	if ($user === false) {
		fail($html, 404, "No such user.");
	}

	$member_of = $groups->group_ids_of($_REQUEST["id"]);
	require_can($html, $rules["user"], "user.edit", $member_of);

	echo page($html, $frame, "admin-users-edit.tpl", array(
		"user" => $user,
		"member_of" => $groups->groups_of($_REQUEST["id"]),
		"all_groups" => $groups->all(),
		"message" => $flash->take(),
	));
```

The annotation names the path and `$_REQUEST["id"]` is the parameter. The row is read before the
permission check, because 404 for a user that does not exist is the more useful answer; the
trade is that a signed-in administrator learns whether an id exists. The section list is
`group_ids_of($id)`, so the question the grant answers is "may this administrator edit users
in group X", and a target in no group is evaluated against the module-wide default. `page()`
merges the route's data over `$frame` and returns the layout with `admin-users-edit.tpl` as
its body. `$flash->take()` is called here and not in the composition root, because taking a
message clears it.

`templates/admin-users-edit.tpl` is four forms on one page, one per key the panel declares,
each posting to the route that checks that key. The first of them, trimmed to the fields it
sends:

```html
<form class="card" method="post" action="/admin/user/{user.id}">
	<input type="hidden" name="csrf_token" value="{csrf_token}">
	<input id="username" name="username" value="{user.username}" required>
	<label><input type="checkbox" name="is_active" value="1"{if $user.is_active} checked{/if}> Active</label>
	<button type="submit">Save</button>
</form>
```

The POST twin, `routes/admin-users-save.php`, is CSRF, the same two guards and the same 404,
then validate, write, flash, redirect:

```php
	require_login($session);
	require_csrf($html, $csrf);

	// the find(), the 404 and the require_can() above, unchanged

	$username = isset($_POST["username"]) ? trim($_POST["username"]) : "";
	if ($username === "") {
		fail($html, 422, "A username is required.");
	}

	$users->update($_REQUEST["id"], array(
		"username" => $username,
		"email" => isset($_POST["email"]) ? trim($_POST["email"]) : "",
		"is_admin" => isset($_POST["is_admin"]) ? 1 : 0,
		"is_active" => isset($_POST["is_active"]) ? 1 : 0,
	));

	$flash->set("Saved " . $username . ".");

	redirect_to("/admin/user/" . $_REQUEST["id"]);
```

The password is not among the columns written here. Setting one is its own key,
`user.password`, and its own route, because an administrator trusted with a display name is
not automatically trusted with a credential. The answer is a redirect, so a reload does not
repeat the write, and the flash is displayed by the GET that follows.

Drive it. The cookie jar carries the session and the CSRF token comes off the rendered form:

```bash
curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" http://localhost:8080/admin
302 http://localhost:8080/admin/login?next=%2Fadmin

curl -s -c jar.txt -o login.html http://localhost:8080/admin/login
token=$(grep -o 'csrf_token" value="[^"]*"' login.html | head -1 | cut -d'"' -f3)

curl -s -b jar.txt -c jar.txt -o /dev/null -w "%{http_code} %{redirect_url}\n" \
	-d "csrf_token=$token" -d "username=admin" -d "password=admin" -d "next=/admin" \
	http://localhost:8080/admin/login
302 http://localhost:8080/admin

curl -s -b jar.txt -o /dev/null -w "%{http_code}\n" \
	-d "csrf_token=$token" -d "username=admin" -d "email=admin@example.com" \
	-d "is_active=1" -d "is_admin=1" http://localhost:8080/admin/user/$id
302

curl -s -b jar.txt http://localhost:8080/admin/user/$id | grep notice
<p class="notice">Saved admin.</p>
```

The failure paths answer from the same file: a wrong token is 403, a missing user is 404, an
empty username is 422.

## The handlers live in the route files

A panel class holds no request state, reads no superglobal and contains no route handler. A
handler reads `$_GET`, `$_POST` and `$_REQUEST`, and those reads belong in the annotated file,
so the input to a route is visible from the route: open `routes/admin-users-save.php` and the
four fields it writes are in front of you, next to the `@route` line that says where they
arrive from.

A panel class that also handled requests would have to be reached by a name built at runtime,
from a URL segment to a class or a method. `new $class` and `$this->$method()` are both parse
errors here, so that dispatch does not exist.

## Route paths are written literally

One file per action. A path takes `{name}` for one segment, `{name...}` for the remaining
segments joined, and `{name:regex}` for a constrained segment, each arriving in
`$_REQUEST` under the name it declares. `/admin/{module=users}/{path}` does not work:
there is no default-value syntax, and the route is refused at boot rather than registered
with a parameter literally named `module=users`. See
[Routing and endpoints](10-routing-and-endpoints.md).

Next: [A JSON API](50-a-json-api.md).
