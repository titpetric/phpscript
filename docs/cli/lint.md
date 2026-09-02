# `phpscript lint <path>...`

Lint one or more PHP files or directories.

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`,
`--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`.
The flags below are this command's own.

```bash
phpscript lint tests/fixtures/...
phpscript lint path/to/file.php
```

The lint pass reports these shapes:

| Finding                                               | Example                              |
|-------------------------------------------------------|--------------------------------------|
| `assignment in conditional statement`                 | `if (($row = fn()) !== false)`       |
| `chained assignment binds one value to several names` | `$dba = $dbb = new Database();`      |
| `global is a no-op`                                   | `global $x;`                         |
| `extends is a no-op`                                  | `class Dog extends Animal {}`        |
| `abstract is a no-op`                                 | `abstract class Shape {}`            |
| `magic method ... is never called implicitly`         | `function __call($name, $args)`      |
| `reference & is a no-op` / `returns by value`         | `$a = &$b;`, `function &f()`         |
| `call to undefined function`                          | `undefined_function()`               |
| `new: undefined class` and the `unknown class` forms  | `new ReflectionClass($c);`           |
| `Cannot redeclare function`                           | `function strlen($s) {}`             |
| `JSON_* is not defined and the argument is ignored`   | `json_encode($v, JSON_PRETTY_PRINT)` |
| a class missing a method its `implements` names       | see [design.md](../design.md)        |
| an `@route` path the router cannot answer for         | `// @route /users/{id=}`             |

A parse error and a redeclaration fail the run; the rest are warnings. The
undefined-name checks compare against the file's own declarations plus the
registered runtime bindings, and skip names the source guards with
`function_exists` / `class_exists`; without `--include` a name that arrives
through an autoloader still warns, which is why it warns rather than fails.
The redeclaration check reads the same set from the other side, and fails
rather than warns: a function declared twice in one file, or declared over a
name the runtime itself provides, is refused at run time whatever else is
registered. A name the `--include` file declared is not one of those - it is
PHP like the file being checked, and comparing the two would report every
function a bootstrap defines as a redeclaration of itself. A declaration the source guards with
`function_exists` is the polyfill idiom and is not reported. The chained-assignment rule
exists because phpscript arrays are handles rather than values, so two names can
end up sharing one array where PHP would give each its own. See
[Value semantics](../reference/types/value-semantics.md#arrays-are-handles-not-values).

It reports what the parser could not already fix. `$inlines = $blocks = array()`
is split into one allocation per name and is not reported; neither is
`$r['y'] = $r['m'] = '00'`, because no name can mutate a string through another.
A chain ending in a name, a call or a `new` is reported: the value is either a
handle the names really do share, or of a type the source does not settle.

Findings are printed one per row, with a row per file that had none, grouped
the way `phpscript test` groups fixtures: one table per folder scanned, the
folder naming the file column, closed by a per-folder summary line before the
run's total. A terminal gets colored tables and redirected output gets
Markdown. Use `--output FILE` (`-o`) to write the same tables to a file as
Markdown while the terminal output continues as normal.

```bash
phpscript lint -o docs/lint.md tests/fixtures/...
```

Use `--flatstack` to also report whether the flat bytecode engine can run each
file, which adds a failing row per file it cannot compile. See
[Flat stack](../flatstack.md).

Use `--include FILE` to run a file against the name registry before checking,
which is what lets the checks see an application rather than one file of it.
Without it every check runs against the standard library alone: a class
composer autoloads and a helper the application's bootstrap registers are both
unknown, so most findings are false and a true one cannot be told from them.
The class check autoloads only when an include was given, because composer's
autoloader declares nothing until something asks for it.

```bash
phpscript lint --include vendor/autoload.php ./src/...
phpscript lint --include bootstrap.php ./...
```
