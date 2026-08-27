# Structuring an application

Coming from PHP you will reach for three things phpscript does not have: a base class, a
trait and a service container. This chapter shows the shape that replaces them: what the
object model gives you, how an interface behaves when nothing is inherited, how a component
splits from its storage, and how `bootstrap.php` builds the object graph in one file you can
read top to bottom. At the end you can add a component to the application and say where each
of its collaborators came from.

## What the runtime gives you

A class holds properties, methods and class constants. `__construct` runs on `new`, `$this`
is the instance, `__invoke` makes the object callable, static members are reached through
`self::` and the class name, and `Class::class` is the name as a string.

```php
class Counter
{
	const START = 0;
	public static $built = 0;
	private $n;

	public function __construct($n = self::START)
	{
		$this->n = $n;
		self::$built++;
	}

	public function __invoke()
	{
		return ++$this->n;
	}
}

$c = new Counter();
echo $c(), $c(), " ", Counter::$built, " ", Counter::class, "\n";
```

```text
12 1 Counter
```

That is the whole object model. [docs/design.md](../design.md) lists what is absent and
records that it is not coming: no inheritance, no `parent::`, no traits, no `abstract`,
`final` or `readonly` semantics, and no magic method beyond `__construct` and `__invoke`.
`__toString` is never called, so `echo $obj` prints the empty string and raises nothing. An
empty `trait T {}` parses; give it a method and the parser stops at the `function`.

`extends` does parse. It is recorded on the declaration so the formatter prints the file back
and the linter can see it, and nothing in the runtime reads it. A class gets no member from
the class it names:

```php
class Base
{
	public function greet()
	{
		return "hello";
	}
}

class Child extends Base
{
}

$c = new Child();
echo $c->greet(), "\n";     // call to undefined method Child::greet()
```

`parent::hi()` inside `Child` fails the same way, with `call to undefined method Child::hi()`. Declare the method on the class that uses it.

## An interface is a contract, not a parent

An interface names method signatures and confers nothing. A class that says `implements`
declares every one of those methods itself. `Common\ModuleInfo` is what a module declares
about the permissions it manages:

```php
namespace Common;

interface ModuleInfo
{
	public function name();
	public function title();
	public function icon();
	public function roles();
	public function sections_by_role($role);
}
```

`Common\UsersAdmin` implements it and writes out all five:

```php
class UsersAdmin implements ModuleInfo, AdminPanel
{
	public function name()
	{
		return "user";
	}

	public function roles()
	{
		return array(
			"user" => array("list" => "List users", "edit" => "Edit users"),
		);
	}

	// title(), icon(), sections_by_role() and the four AdminPanel methods follow.
}
```

Leave one out and `phpscript lint` reports it before the file is run:

```text
| WARN | iface.php | 9 | class Hello does not declare method name() required by interface Greeter |
```

The runtime makes the same check at load, before any statement executes, and raises that same
text, so a class failing the contract is never constructed.

`instanceof` compares the names a class declared. A class that happens to have the same
methods is not an instance:

```php
$a = new UsersAdmin();      // implements ModuleInfo
$b = new Loose();           // the same method, no implements

echo $a instanceof ModuleInfo ? "yes\n" : "no\n";   // yes
echo $b instanceof ModuleInfo ? "yes\n" : "no\n";   // no
```

## Give a component its collaborators through the constructor

A class receives what it needs as constructor arguments and keeps them in private
properties. Anything touching the database uses two layers: a component holding the
behaviour, and a store interface with one method per operation the component performs.

```php
interface UserStore
{
	public function find($id);
	public function find_many(array $ids);
	public function find_by_username($username);
	public function insert(array $fields);
	public function is_admin($id);
	// update(), delete(), all() and count() complete the list.
}
```

No name in it mentions SQL. `Common\Users` takes one, plus an optional decorator applied to
every row it reads:

```php
class Users
{
	private $store;
	private $decorator;

	public function __construct(UserStore $store, ?UserDecorator $decorator = null)
	{
		$this->store = $store;
		$this->decorator = $decorator;
	}

	public function find($id)
	{
		$row = $this->store->find($id);
		if ($row === false) {
			return false;
		}

		return $this->decorate($row);
	}
}
```

Two classes implement the interface. `Common\Store\SqlUserStore` issues statements through a
`Common\Store\Connection`, and `Common\Mock\MemoryUserStore` keeps rows in an array:

```php
// Common\Store\SqlUserStore
public function find($id)
{
	return $this->conn->db()->get("SELECT * FROM user WHERE id = ?", $id);
}

// Common\Mock\MemoryUserStore
public function find($id)
{
	if (!isset($this->rows[$id])) {
		return false;
	}

	return $this->rows[$id];
}
```

The memory store is what lets `src/Users.phpt` run with no database, on the interpreter, the
flat stack engine and real PHP alike:

```php
$store = new Common\Mock\MemoryUserStore();
$tit = $store->insert(array("username" => "tit", "is_admin" => 1));

$users = new Common\Users($store, new Common\PropertiesDecorator());

echo $users->exists("tit") === $tit ? "taken" : "free", "\n";
echo $users->is_admin($tit) ? "admin" : "user", "\n";
```

The fixture asserts that a taken username is refused and that `is_admin` answers off the
stored column. It mentions no select list, so adding a column to `SqlUserStore` does not
break it. Every method of the interface is written out in the memory store; there is no
shared implementation to inherit, and a mock answering through a parent would stop being the
thing you check the interface against.

## Give each class one responsibility

`Common\Users` reads and writes user records. It knows no URL, no permission key and no
template, and it does not hash passwords: `Common\Auth` owns hashing, and `Users::create()`
stores whatever arrives under the `"password"` key exactly as given.

`Common\UsersAdmin` sits beside it and declares the `/admin/user` surface: the module name
and title, the permission keys the route files check, the sections those keys are scoped to,
the navigation entries, and the route and template stem. It holds no request state, reads no
superglobal and contains no handler. The handlers are the files in `routes/`.

Both reasons come from the shipped code. An `/api`-only consumer constructs `Users` and never
constructs the panel, so the panel's dependency on `Common\Groups` is not forced on it. And
the two change for different reasons: a column added to the user table changes `Users`, a
permission key added to the admin screen changes `UsersAdmin`.

## Build the graph in bootstrap.php

`demos/common-phpscript/bootstrap.php` is the composition root. Every entrypoint includes it:
the route files in `routes/`, the scheduled job in `jobs/` and the startup file
`migrate.php`. A top level `include` shares the includer's scope, so the variables built here
are the route file's variables and no route file constructs anything.

It declares no namespace, because a namespaced file may only declare classes and functions:

```text
line 5: a namespaced file may only declare classes and functions, because it is scanned for
the symbols it declares at include time instead of being run; move this statement into a
function, or into a file that declares no namespace
```

The file is built leaves first. Configuration and the host bindings, then the connection and
the stores that take it:

```php
include "vendor/autoload.php";

$config = new Common\Config(array(
	"site_name" => "Common",
	"template_paths" => array("templates/", "vendor/titpetric/phpscript-common/templates/"),
	"template_cache" => "templates/cache/",
	"session_ttl" => 86400,
	"password_cost" => 12,
));

$db = new Database("common");
$conn = new Common\Store\Connection($db, "sqlite");

$user_store = new Common\Store\SqlUserStore($conn);
$group_store = new Common\Store\SqlGroupStore($conn);
$rule_store = new Common\Store\SqlRuleStore($conn);
$module_store = new Common\Store\SqlModuleStore($conn);
$session_store = new Common\Store\SqlSessionStore($conn);
```

Then the components, each naming what it needs:

```php
$users = new Common\Users($user_store, new Common\PropertiesDecorator());
$groups = new Common\Groups($group_store);

$session = new Common\Session(
	new \Session\Manager(new \Session\Storage\Disk()),
	$session_store,
	$config->session_ttl()
);
$csrf = new Common\Csrf($session);
$flash = new Common\Flash($session, $session_store);
$identity = new Common\SessionIdentity($session, $users);
$auth = new Common\Auth($users, $config->password_cost());
```

Then the panels, with one `Common\Rules` built per panel, and last the frame, which is the
variable set every layout render starts from. `$nav` comes from a second pass over `$panels`
that keeps the ones the visitor may list:

```php
$panels = array(
	new Common\UsersAdmin($users, $groups),
	new Common\GroupsAdmin($groups),
	new Common\RulesAdmin($groups),
);

$modules = new Common\Modules($module_store);

$rules = array();
foreach ($panels as $panel) {
	$modules->declare_module($panel);
	$rules[$panel->name()] = new Common\Rules($rule_store, $identity, $panel);
}

$frame = array(
	"site_name" => $config->site_name(),
	"nav" => $nav,
	"identity" => $session->current(),
	"csrf_token" => $csrf->token(),
);
```

What is left in a route file is the request:

```php
include "bootstrap.php";

require_login($session);
require_can($html, $rules["user"], "user.list", array("0"));

echo page($html, $frame, "admin-users-list.tpl", array(
	"users" => $users->all(),
	"message" => $flash->take(),
));
```

## Pass collaborators into the free functions

`bootstrap.php` also declares the helpers every route uses, and each takes its collaborators
as parameters:

```php
function require_login($session)
{
	$context = $session->current();
	if (!$context["is_authenticated"]) {
		redirect_to("/admin/login?next=" . rawurlencode($_SERVER["REQUEST_URI"]));
	}

	return $context;
}
```

A function body does not see the includer's scope, so `$session` inside `require_login` is
undefined unless it is an argument. `global $session` parses and does nothing:

```php
$session = "the session";

function show()
{
	global $session;
	var_dump(isset($session));     // bool(false)
}

show();
```

There is no third way in. A collaborator that is not an argument is not reachable from a
function body, which is why `fail($html, ...)`, `require_can($html, $rules, ...)`,
`require_csrf($html, $csrf)` and `page($html, $frame, ...)` all carry theirs in the
signature.

## What replaces a service container

Nothing does. The legacy code this package replaces held a static map of factory closures and
reached entries through `__call` accessors resolved off a base class. Neither half works
here. There are no magic methods beyond `__construct` and `__invoke`, so `__call` never fires
and every accessor built on it resolves to nothing; and `new $className` does not parse, so a
factory map cannot instantiate what it is handed. The replacement is the file above: one
`new` per object in dependency order, and the answer to "where does `$users` come from" is a
line you can point at.

The `$rules` array keyed by module name is not a container either. Nothing looks a name up in
it at runtime except the `foreach` that filled it. A route file indexes it with the literal
name of its own panel, written into the source:

```php
require_can($html, $rules["user"], "user.list", array("0"));
```

`Common\Path` is the one static class in the package, for the same rule read the other way:
it joins path fragments, holds no state and has no collaborator to inject, so an instance
would be an object constructed at every call site carrying nothing.

## Autoloading

Classes load through Composer PSR-4, and `bootstrap.php` includes `vendor/autoload.php` once.
One class per file, and the file is named after the class: `Common\Users` is `src/Users.php`,
`Common\Store\SqlUserStore` is `src/Store/SqlUserStore.php`.

```json
{
    "autoload": {
        "psr-4": {
            "Common\\": "src/"
        },
        "classmap": [
            "src/Errors.php"
        ]
    }
}
```

`src/Errors.php` is the exception, and it is why `demos/common-phpscript/composer.json`
carries a classmap. It declares six classes, `NotFound`, `ValidationFailed`,
`Unauthenticated`, `PermissionDenied`, `Conflict` and `ConfigError`, because with no
inheritance a shared base class would confer nothing on any of them. PSR-4 would look for
`Common\NotFound` in `src/NotFound.php` and not find it, so the classmap names the file and
Composer indexes every class in it.

## What to write instead of dynamic dispatch

Both spellings of dynamic dispatch are parse errors, reported before anything runs:

```text
$o = new $className();     line 4: expected class name after new
return $this->$method();   line 8: expected member name
```

Construct the instances up front, keep them in an array, and call through `call_user_func`:

```php
$panels = array();
$panel = new UsersAdmin();
$panels[$panel->name()] = $panel;

$target = $panels["user"];
$name = "title";

if (!is_callable(array($target, $name))) {
	echo "no such method\n";
	exit();
}

echo call_user_func(array($target, $name)), "\n";                 // Users
echo is_callable(array($target, "missing")) ? "yes\n" : "no\n";   // no
```

`is_callable(array($obj, "missing"))` answers correctly, so a dispatcher can still refuse an
unknown method instead of failing at the call. For an argument list held in an array, use
`call_user_func_array(array($obj, "get"), $args)`; `f(...$array)` at a call site does not
parse.

## Arrays are handles

Assigning an array does not copy it, and a function taking `array $a` edits the caller's
array. This diverges from PHP, and it is a silent wrong answer rather than an error:

```php
function touch_it(array $a)
{
	$a["added"] = true;
}

$a = array("k" => 1);
$b = $a;
$b["k"] = 2;
echo "a[k] = ", $a["k"], "\n";

touch_it($a);
echo "added = ", isset($a["added"]) ? "yes" : "no", "\n";
```

```text
a[k] = 2
added = yes
```

Real PHP prints `a[k] = 1` and `added = no`, so `phpscript test --matrix` catches the
divergence. When you mean a copy, build one with a `foreach`:

```php
$copy = array();
foreach ($a as $k => $v) {
	$copy[$k] = $v;
}
```

This is why a component returning rows returns freshly built arrays, and why
`Common\Users::find_many()` appends to `$rows` inside a loop instead of handing back what the
store gave it.

Next: [Users and authentication](30-users-and-authentication.md).
