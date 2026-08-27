# Testing

An application built the way this book builds one is tested at three levels, and the first
one covers most of the code. This chapter shows how to write a `.phpt` fixture, how to run a
component that normally talks to a database without one, where an `include` inside a fixture
resolves, what a fixture cannot express, and how to drive the whole application over HTTP.
At the end you can add a fixture beside a source file and have it run on three runtimes.

## The three levels

| Level                             | Tool              | What it covers                                                         |
|-----------------------------------|-------------------|------------------------------------------------------------------------|
| A unit of behaviour               | a `.phpt` fixture | one script, run once, output compared exactly                          |
| Anything needing several requests | a Go test         | request sequences, route registration, uploads, state between requests |
| The application over HTTP         | a venom suite     | real routes, real database, status codes and headers                   |

Reach for the fixture first. `demos/common-phpscript` ships 28 of them and no Go test of its
own, because every component there takes its storage as a constructor argument and is
therefore reachable from one script.

## Writing a fixture

A `.phpt` file has three sections separated by a line holding only `---`: YAML frontmatter,
PHP source, expected output. `demos/common-phpscript/src/Path.phpt` is the whole format:

```phpt
name: path join puts exactly one slash between fragments
description: >
  Fragments are joined with one slash whether or not they carry their own, an
  empty fragment in the middle is skipped, and the first fragment is kept as
  given so a leading slash survives. The arguments are read with func_get_args,
  because a declared variadic binds only the first surplus argument here.
---
<?php

include "Path.php";

use Common\Path;

echo Path::join("/var/www", "admin/", "/user.php"), "\n";
echo Path::join("/var/www/", "/admin"), "\n";
echo Path::join("assets", "css", "site.css"), "\n";
echo Path::join("/var/www", "", "admin"), "\n";
echo Path::join("/var/www"), "\n";
echo "[", Path::join(), "]\n";
---
/var/www/admin/user.php
/var/www/admin
assets/css/site.css
/var/www/admin
/var/www
[]
```

There is no assertion library and no `assert()`. A fixture prints, and the printed text is
compared against the third section exactly, with a trailing newline difference ignored.

The frontmatter fields:

| Field         | Required | What it does                                                                                      |
|---------------|----------|---------------------------------------------------------------------------------------------------|
| `name`        | yes      | The subtest name.                                                                                 |
| `description` | yes      | The behaviour the fixture states, in a sentence or two.                                           |
| `runner`      | no       | Runtimes the fixture opts out of: `php: false`, `flatstack: false`.                               |
| `root`        | no       | Include root, relative to the fixture's own directory.                                            |
| `error`       | no       | Substring an uncaught error must contain.                                                         |
| `request`     | no       | `args`, `get`, `post`, `cookie`, `env`, `headers`, `stdin`, `body`, filled into the superglobals. |
| `response`    | no       | `headers` the script is expected to have set.                                                     |

`request` and `response` are what let a fixture cover a route file's body with no server
running. `tests/fixtures/bindings/request_and_response_handling.phpt` uses both.

## A fixture beside every source file

`demos/common-phpscript/src` holds 49 `.php` files and 28 `.phpt` files, and each fixture is
named after the source it covers: `Users.php` and `Users.phpt`, `Render/Json.php` and
`Render/Json.phpt`. The 21 sources without one are the 10 interfaces, the 5 SQL stores that
need a database, 4 memory stores covered through the component they serve, and `Config.php`
and `SessionIdentity.php`, which read host state.

That convention buys two things. A reader opening `Rules.php` can open `Rules.phpt` beside
it for a worked example of every method, and a source file with no fixture is visible in a
directory listing rather than in a coverage report. Run the package with one command:

```bash
phpscript test ./src/...
```

The run prints one table per directory with a duration and a GC count per fixture, then the
total:

```text
## Summary

| Area       | Fixtures | Passed | Failed |
| ---------- | -------- | ------ | ------ |
| src        | 21       | 21     | 0      |
| src/Mock   | 3        | 3      | 0      |
| src/Render | 3        | 3      | 0      |
| src/Store  | 1        | 1      | 0      |
| **Total**  | 28       | 28     | 0      |
```

The `...` suffix is what makes the walk recursive. A bare directory path matches only the
fixtures sitting directly in it.

## Running a fixture on three runtimes

`--matrix` runs every fixture through the compatibility interpreter, the flatstack bytecode
engine and the real `php` binary, and gives each runtime a column:

```bash
phpscript test --matrix ./src/...
```

```text
## src/Render

| src/Render   | Flat stack | Runtime | PHP  |
| ------------ | ---------- | ------- | ---- |
| Html.phpt    | PASS       | PASS    | SKIP |
| Json.phpt    | PASS       | PASS    | PASS |
| Problem.phpt | PASS       | PASS    | SKIP |
```

A fixture that passes the `PHP` column is a compatibility check as well as a test: the
expected output is what PHP itself prints for the same source, so a place where phpscript
diverges from the language fails a column instead of producing a wrong answer in production.
Write the fixture in that order. Run the source through `php` first, paste that output into
the expected section, then make phpscript produce it.

A `SKIP` is a fixture that opted the runtime out in its frontmatter:

```yaml
runner:
  php: false
```

Opt out only where `php` has nothing to say, and record the reason in the `description`.
There are two reasons in this package. The first is a host binding whose name does not exist
in PHP: `Database`, `SharedMemory`, `Session\Manager`, `start_span`. `src/Log.phpt` is opted
out for that one, because `Common\Log` opens a telemetry span per event.

The second is a phpscript contract PHP does not share. `src/Errors.phpt`,
`src/Render/Problem.phpt` and `src/Render/Html.phpt` all throw one of the six classes in
`Common\Errors`, and none of them implements `Throwable`, because there is no `extends` in
this runtime. phpscript throws any object that declares `getMessage()`. Real PHP refuses:

```text
PHP Fatal error:  Uncaught Error: Cannot throw objects that do not implement Throwable
```

## Testing a component with no database

This is what the store interfaces from
[Structuring an application](25-structuring-an-application.md) are for. `Common\Users` never
sees a connection. It holds a `UserStore`, which names one method per operation and no SQL:

```php
interface UserStore
{
	public function find($id);
	public function find_many(array $ids);
	public function find_by_username($username);
	public function insert(array $fields);
	public function update($id, array $fields);
	public function delete($id);
	public function is_admin($id);
	public function all();
	public function count();
}
```

`Common\Store\SqlUserStore` implements it against a database. `Common\Mock\MemoryUserStore`
implements it against an array, and lives in the package rather than in a test directory,
because a fixture in the same tree has to be able to `include` it. One of its nine methods:

```php
public function insert(array $fields)
{
	$this->next = $this->next + 1;
	$id = "u" . $this->next;

	$row = array("id" => $id);
	foreach ($this->defaults() as $column => $value) {
		$row[$column] = array_key_exists($column, $fields) ? $fields[$column] : $value;
	}

	$this->rows[$id] = $row;

	return $id;
}
```

The other eight are written out in the file. Nothing is shared with the SQL store, because
there is no inheritance to share it through, and the lint pass flags a class that implements
an interface without declaring one of its methods.

The minted id is `"u1"` rather than `1` on purpose. An array key that looks like an integer
becomes one, and a fixture would then pass on integer keys where the SQL store hands back 26
character strings.

The fixture constructs the component the way `bootstrap.php` does, with the memory store in
place of the SQL one:

```phpt
name: users are created and read back without a database
description: >
  Users is constructed over MemoryUserStore, so create, exists, find,
  find_by_username and is_admin all run with no connection and no SQL. A taken
  username is refused, which is the check create() makes before it writes.
---
<?php

include "UserStore.php";
include "Mock/MemoryUserStore.php";
include "Users.php";

$users = new Common\Users(new Common\Mock\MemoryUserStore());

$tit = $users->create(array("username" => "tit", "is_admin" => 1));
$ana = $users->create(array("username" => "ana", "email" => "ana@example.com"));

echo $users->count(), "\n";
echo $users->find($tit)["username"], "\n";
echo $users->find_by_username("ana@example.com")["id"] === $ana ? "by email" : "no", "\n";
echo $users->exists("tit") === $tit ? "taken" : "free", "\n";
echo $users->is_admin($tit) ? "admin" : "user", "\n";

try {
	$users->create(array("username" => "tit"));
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
---
2
tit
by email
taken
admin
Users::create(): username is taken: tit
```

That fixture passes on all three runtimes, `php` included, because nothing in it is a
binding. `src/Users.phpt` in the package is the same shape with `find_many`, `update`,
`remove` and a `PropertiesDecorator` added.

Permissions, groups and sessions are covered the same way. `src/Rules.phpt` builds a rule set
over `MemoryRuleStore` and `MemoryIdentity` and asks `can()` questions, and `src/Csrf.phpt`
starts a session through `MemorySessionManager` and checks a token. No connection is opened,
and the whole suite runs in under a second.

## Where an include inside a fixture resolves

`phpscript test` runs a fixture against a filesystem rooted at the fixture's own directory.
Three consequences cost a reader a debugging cycle each.

`include "X.php"` resolves against that directory, so a fixture in `src/Store/` includes its
neighbour by bare name: `include "Connection.php";`.

`__DIR__` is the fixture's directory as it was named on the command line, not an absolute
path. Running `phpscript test ./src/...` makes it `src`, so prefixing it asks for a path
below a root that is already `src`:

```text
error including src/Path.php: include "src/Path.php": load "src/Path.php":
open src/Path.php: no such file or directory
```

`include "../X.php"` fails too, because `..` is not a valid path in the filesystem the
fixture is handed:

```text
error including ../UserStore.php: include "../UserStore.php":
load "../UserStore.php": readfile ../UserStore.php: invalid argument
```

A fixture in a subdirectory that needs the package root sets `root:` in its frontmatter and
includes from there instead. `src/Mock/MemoryUserStore.phpt` is one directory down:

```phpt
name: MemoryUserStore answers the user store interface from an array
description: >
  The memory store mints an id per insert and returns rows the same shape a SQL
  store does, including the columns an insert did not name.
root: ..
---
<?php

include "UserStore.php";
include "Mock/MemoryUserStore.php";
```

`root` is resolved against the fixture's own directory and it moves the root for the `php`
runner as well, so all three runtimes still read the same files.

## A fixture that writes cleans up after itself

Includes are rooted at the fixture's directory, but a file the script creates lands in the
directory `phpscript test` was invoked from. `src/Log.phpt` writes a log stream and removes
it in the same script:

```php
$handle = fopen("events.jsonl", "w");
$log = new Common\Log("users", $handle);
$log->info("user updated", array("id" => "01H8"));
fclose($handle);

$lines = explode("\n", trim(file_get_contents("events.jsonl")));

// The fixture writes into the directory it runs from, so it clears up after
// itself rather than leaving a file behind for the next run.
unlink("events.jsonl");
```

Without the `unlink`, running `phpscript test ./src/...` from the package root leaves
`events.jsonl` next to `composer.json`.

## What a fixture cannot do

A fixture runs one script once. It cannot express "the eleventh request is refused", it
cannot show that a rate limiter resets, and it cannot assert anything about routing, because
no route table is built. Those are Go tests. `tests/route_test.go` is the harness: it mounts
the annotated PHP files on an `http.ServeMux`, drives `httptest` requests through it, and
puts one `SharedMemory` value in every runtime so state survives from one request to the
next.

```bash
go test ./tests -run 'TestRoute' -v
```

```text
=== RUN   TestRouteSharedMemoryFixture
--- PASS: TestRouteSharedMemoryFixture (0.00s)
=== RUN   TestRouteFileUpload
--- PASS: TestRouteFileUpload (0.00s)
PASS
ok  	github.com/titpetric/phpscript/tests	0.007s
```

## Driving the application over HTTP with venom

The last level runs the application as it ships. `demos/example/tests/venom.yml` covers every
route the example registers; this is one of its six testcases:

```yaml
vars:
  host: http://example.localhost

testcases:
  - name: a bookmark can be added and deleted
    steps:
      - type: http
        method: POST
        url: "{{.host}}/bookmarks"
        headers:
          Content-Type: application/x-www-form-urlencoded
        body: "title=Venom&url=https://github.com/ovh/venom"
        assertions:
          - result.statuscode ShouldEqual 200
          - result.body ShouldContainSubstring Bookmark saved.
          - result.body ShouldContainSubstring Venom
        extracts:
          result.body: "action=\"/bookmarks/(?P<added>[0-9]+)/delete\""
      - type: http
        method: POST
        url: "{{.host}}/bookmarks/{{.added}}/delete"
        assertions:
          - result.statuscode ShouldEqual 200
          - result.body ShouldNotContainSubstring Venom
```

```bash
venom run venom.yml --var=host=http://127.0.0.1:8080
```

```text
 • example (venom.yml)
 	• the-migration-seeded-the-first-bookmark PASS
 	• a-bookmark-can-be-added-and-deleted PASS
 	• input-is-required PASS
 	• a-missing-bookmark-is-reported PASS
 	• markup-is-escaped PASS
 	• static-assets-are-served-from-the-web-root PASS
final status: PASS
```

Three rules make a suite worth keeping. Write it so it can run twice: the case above adds a
bookmark and deletes it again, so a second run starts from the state the first one left.
Read back the id the application generated rather than assuming one; `extracts` pulls it out
of the response body with a named capture group and the later step spells it `{{.added}}`.
Clean up inside the testcase that created the record, so a failure halfway through the suite
does not leave rows for the next case to count.

The host comes from `--var`, so the same file runs against a container address in the
pipeline and against a local `phpscript server` while you are writing it.

## The lint pass

`phpscript lint` reads the same tree and applies three rules. Here is what it says about a
scratch file that breaks all three:

```bash
phpscript lint ./...
```

```text
| Status | File    | Line | Message                                                                  |
| ------ | ------- | ---: | ------------------------------------------------------------------------ |
| WARN   | Bad.php |    9 | class Rows does not declare method reset() required by interface Counter |
| WARN   | Bad.php |   13 | chained assignment binds one value to several names                      |
| WARN   | Bad.php |   15 | assignment in conditional statement                                      |
```

Chained assignment is flagged because arrays are handles here: `$a = $b = array()` binds one
array to two names, and a write through either is visible through the other.

The interface rule is the reason an interface is worth declaring in a runtime with no
dispatch. Nothing checks at call time that a method exists, so the lint pass is the check.

Assignment inside a conditional is why `while ($row = $db->fetch($query))` is not the idiom
in this book. Read the result and loop over it:

```php
public function all()
{
	return $this->conn->db()->get_all("SELECT * FROM user ORDER BY username");
}
```

`.phpt` files are linted too, so a fixture is held to the same three rules as the source it
covers. The same command in `demos/common-phpscript` reads all 103 of them and finds
nothing:

```text
Passing 103, with 0 warnings, 0 failing
```

## Wiring it into a pipeline

The repository runs both commands as one task in `atkins.yml`:

```yaml
  test:phpscript:common:
    dir: demos/common-phpscript
    tty: true # enable ansi
    passthru: true
    cmds:
      - phpscript lint ./...
      - phpscript test --matrix -v ./src/...
```

```bash
atkins --final test:phpscript:common
```

```text
Matrix summary: 28 passed, 0 failed out of 28 fixtures (1396ms)
```

The task needs no database, no server and no compose service, which is why it sits before the
demo suites in the default pipeline rather than inside the block that brings MySQL and
PostgreSQL up. `-v` adds the failing runtime's diff to the table, which is the output worth
having in a CI log.

Next: [Running in production](60-running-in-production.md).
