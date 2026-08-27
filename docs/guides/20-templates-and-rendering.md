# Templates and rendering

A route file assembles data and has to turn it into a response. This chapter
installs the template engine, renders a page through a layout, renders a JSON
body, and gives both the same shape for an error. At the end of it you can write
a GET route that draws a screen and a route that answers `application/json`.

## Install minitpl with composer

The template engine is [titpetric/minitpl](https://github.com/titpetric/minitpl),
an ordinary composer package rather than a binding built into the interpreter:

```bash
composer require titpetric/minitpl
```

The interpreter resolves `composer.json` and `vendor/` on its own, so
`include "vendor/autoload.php"` reaches `MiniTPL\Template` the way it does under
PHP, and the package source runs unmodified.
`demos/common-phpscript/composer.json` is the whole manifest:

```json
{
    "name": "titpetric/phpscript-common",
    "description": "Users, groups, permissions, authentication and sessions for phpscript applications",
    "type": "library",
    "license": "MIT",
    "repositories": [{
        "packagist.org": false
    },{
        "name": "minitpl",
        "type": "vcs",
        "url": "https://github.com/titpetric/minitpl"
    }],
    "require": {
        "titpetric/minitpl": "^1.3"
    },
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

Both `repositories` entries are needed together. `{"packagist.org": false}`
switches the default repository off, so composer resolves nothing it was not
told about, and with it off there is nowhere left to find `titpetric/minitpl`,
so the `vcs` entry names the git repository to read it from. `composer.lock`
records the result: one package, `titpetric/minitpl v1.3.0`, from
`github.com/titpetric/minitpl.git`.

## Load a template and render it

Seven calls are the whole engine.

| Call                                        | Effect                                                                                                    |
|---------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| `new MiniTPL\Template($paths)`              | Constructs with a search path. The default is `array("templates/")`.                                      |
| `set_paths($paths)`                         | Replaces the search path. Takes an array, or several strings as separate arguments.                       |
| `set_compile_location($path, $is_absolute)` | Sets where compiled templates are written.                                                                |
| `load($file)`                               | Finds `$file` on the search path, compiles it if the cached copy is missing or older, and returns a bool. |
| `assign(array $values)`                     | Assigns a whole array. `assign($key, $value)` assigns one name.                                           |
| `render()`                                  | Includes the compiled file, writing to the response body.                                                 |
| `get()`                                     | Buffers `render()` and returns the markup as a string.                                                    |

A complete program is five statements after the autoloader:

```php
$tpl = new MiniTPL\Template(array("templates/"));
$tpl->set_compile_location("templates/cache/", true);
$tpl->load("hello.tpl");
$tpl->assign(array("title" => "Users", "rows" => $rows));
echo $tpl->get();
```

`load()` returns false when no directory on the search path holds the file,
which is what a fallback list is built on: `Common\Render\Html::get()` accepts an
array of names and renders the first that loads.

## Put the application's templates first

The search path is a list of directories and the earliest entry that holds the
file wins, which is how an application replaces one template a package ships
without forking the rest: name the file the same and put it in the directory
listed first. `demos/common-phpscript/bootstrap.php` lists two:

```php
$config = new Common\Config(array(
	"site_name" => "Common",
	"template_paths" => array("templates/", "vendor/titpetric/phpscript-common/templates/"),
	"template_cache" => "templates/cache/",
));

$tpl = new MiniTPL\Template($config->template_paths());
$tpl->set_compile_location($config->template_cache(), true);

$html = new Html($tpl);
```

The second argument to `set_compile_location()` is `$is_absolute`, and with two
search roots it has to be true. In relative mode the compile directory hangs off
each root, so the second one would have its cache written to
`vendor/titpetric/phpscript-common/templates/cache/`, a directory that belongs to
a dependency. Absolute mode mirrors both roots under the one location:

```text
templates/cache/templates/layout.tpl
templates/cache/vendor/titpetric/phpscript-common/templates/layout.tpl
```

An application with one search root can use the relative form:
`demos/example/bootstrap.php` calls `set_compile_location("cache/", false)` and
its cache is `templates/cache/`.

## Write the template

The table body of `templates/admin-users-list.tpl` uses most of the syntax the
shipped screens need:

```text
{foreach $users as $user}
	<tr>
		<td><a href="/admin/user/{user.id}">{user.username}</a></td>
		<td>{user.email}</td>
		<td>{if $user.is_admin}yes{else}no{/if}</td>
		<td>{if $user.is_active}yes{else}no{/if}</td>
	</tr>
{else}
	<tr><td colspan="4" class="empty">No users.</td></tr>
{/foreach}
```

| Construct                                                | Meaning                                                                                       |
|----------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| `{name}`                                                 | Prints the assigned value `name`. `{$name}` is the same tag.                                  |
| `{array.key}`                                            | Array index. `{a.b.0}` is `$a['b'][0]`, and dots nest as deep as the data does.               |
| `{if expr}` ... `{elseif expr}` ... `{else}` ... `{/if}` | PHP conditions, written over template variables.                                              |
| `{foreach $list as $item}` ... `{else}` ... `{/foreach}` | Loops. `{else}` runs when the list is empty, and `{foreach $list as $k => $v}` gives the key. |
| `{include file.tpl}`                                     | Pastes another template in at compile time.                                                   |
| `{load $var}`                                            | Loads the template named by a variable at render time.                                        |
| `{value                                                  | modifier}`                                                                                    |
| `{* text *}`                                             | A comment, removed at compile time.                                                           |

Every printed value goes through `htmlspecialchars($value, ENT_QUOTES)`. The
compiler tracks the markup context a tag sits in, so text, attributes and
comments are escaped and the body of a `<script>` or `<style>` is left alone.
`|unescape` (or its alias `|raw`) opts one tag out. With `title` assigned
`Hall & Oates <b>` and `note` assigned `<em>markup</em>`:

```text
<h1>{title}</h1>                    -> <h1>Hall &amp; Oates &lt;b&gt;</h1>
<p title="{title}">{note}</p>       -> <p title="Hall &amp; Oates &lt;b&gt;">&lt;em&gt;markup&lt;/em&gt;</p>
<p>{note|unescape}</p>              -> <p><em>markup</em></p>
```

A value that is already markup has to be printed with `|unescape` or it arrives
as visible tag text. A value a user supplied must not be: `|unescape` on it is a
stored cross-site scripting hole. Only markup the application built itself is
safe to opt out.

A template comment may not name `{include ...}` or `{load ...}`. Both are
substituted before comments are stripped, so a comment mentioning one has it
expanded.

## Recompile the parent after editing a partial

`{include}` is compile-time substitution. The partial is pasted into the parent
before the parent is compiled, so the compiled parent carries a copy of it. Edit
`admin-_nav.tpl` and the page keeps the old navigation, because `layout.tpl` is
unchanged and its cached copy is still newer than its source. Touch
`layout.tpl` or delete `templates/cache/` to pick the edit up.

The running server has a second cache. It keeps the compiled program of each
file it has executed, keyed by path, so a template already rendered once in the
process goes on rendering the version that was loaded: minitpl sees the newer
source and rewrites the file under `templates/cache/`, and the request still
serves the old output. Restart the server after editing a template.

## Give the cache somewhere to write

The compile location is the only directory the application writes to.
`config.yml` names it, next to the database file:

```yaml
runner:
  writable_paths:
    - templates/cache
    - common.db
```

A non-empty `runner.writable_paths` refuses every write outside the list, and a
refused write raises rather than returning false, so it is catchable.
`fopen("notes.txt", "w")` in this application throws
`fopen(notes.txt): writable_paths allows templates/cache, common.db`.

Leaving the list empty allows every write. A first run with no
`templates/cache/` directory is fine: the compiler creates the parents it needs,
inside the allowance.

## Render a page from a route

`Common\Render\Html` wraps the configured template instance. `get()` returns the
markup and `render()` echoes it; neither exits, so a route decides when the
request is over. `bootstrap.php` adds one function over it:

```php
function page($html, array $frame, $body, array $data)
{
	return $html->get("layout.tpl", array_merge($frame, $data, array("body" => $body)));
}
```

`$frame` is built once in `bootstrap.php` and holds what the layout needs on
every screen: the site name, the navigation, the session context and the CSRF
token. `$data` is merged over it, so a route overrides a frame value by naming
it, and `body` names the panel template. `$html` and `$frame` are parameters
because a function body does not see the includer's scope in this runtime and
`global` parses while doing nothing.

`routes/admin-users-list.php` is a complete GET route:

```php
<?php

// @route GET /admin/user

use Common\Render\Problem;

include "bootstrap.php";

try {
	require_login($session);
	require_can($html, $rules["user"], "user.list", array("0"));

	echo page($html, $frame, "admin-users-list.tpl", array(
		"users" => $users->all(),
		"message" => $flash->take(),
	));
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

`templates/layout.tpl` is the frame it renders, abridged here to the tags that
carry a directive:

```text
<head>
<title>{site_name}</title>
</head>
<body>
{include admin-_nav.tpl}
<main class="page">
{if $message}
<p class="notice">{message}</p>
{/if}
{load $body}
</main>
</body>
```

`{load $body}` resolves at render time, which is why the panel template is a
variable and the navigation is an include. Serve the application and read the
page:

```bash
phpscript -f config.yml server .
```

## Render JSON

`Common\Render\Json` has the same two methods. `get()` returns the encoded body
and `render()` sends it with `Content-Type: application/json`:

```php
$json->render(array("users" => $rows));
// {"users":[{"id":"6276d7da45d13f94aca76f80a7","username":"admin"}]}
```

Two things differ from PHP. `json_encode` takes one argument, so
`json_encode($data, JSON_PRETTY_PRINT)` fails with `json_encode() expects at most 1 argument, 2 given` and there is no pretty-printed variant for
development. The second is that `json_encode(array())` is `[]`: an empty PHP
array carries nothing that says whether it was a list or a map, so `Json::get()`
answers `{}` for an empty payload rather than handing an array to a client
reading an object.

There is no JSONP. A `callback` parameter concatenated into the body makes every
endpoint readable cross-origin with the caller's cookies attached; a
cross-origin API sets CORS headers with `header()` instead.

## Render an error

`Common\Render\Problem` is the one error payload both renderers accept. Both
methods are static:

```php
Problem::status($e)   // 400 to 599 from getCode(), 500 for anything else
Problem::of($e)       // array("error" => array("message" => ..., "code" => ...))
```

Every shipped route ends with the same catch. The HTML form is above; the JSON
form hands the whole payload to the other renderer:

```php
} catch (Exception $e) {
	http_response_code(Problem::status($e));
	$json->render(Problem::of($e));
}
```

A route has to catch. An exception that escapes ends the request as a 500
whatever code it carries, because the wrapper the runtime puts around a thrown
object does not answer `GetCode()`.

`Problem::of()` withholds the message at 500 and above and replaces it with
`internal error`, because the text of an internal failure names a DSN, a file
path or a column and a caller cannot act on any of it. A 404 keeps its message:

```text
404 {"error":{"message":"no such user","code":404}}
500 {"error":{"message":"internal error","code":500}}
```

`Common\Render\Html::get()` throws `Common\ConfigError` when none of its
candidate templates load, and `catch (Exception $e)` does not take it: a catch
clause is answered by the tail of the class name, and a name ending in `Error`
belongs to `catch (Error $e)` and `catch (Throwable $e)`. A missing template
therefore ends the request as a plain 500 from the runtime rather than drawing
the error page. Catch `Throwable` in a route that reports its own wiring faults.

[A JSON API](50-a-json-api.md) covers the error envelope for the API surface, and
[docs/use-cases/error-handling.md](../use-cases/error-handling.md) covers how a
Go binding failure becomes a PHP exception.

Next: [Structuring an application](25-structuring-an-application.md).
