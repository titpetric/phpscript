# `phpscript lint <path>...`

Lint one or more PHP files or directories.

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below belong to this command.

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

A parse error fails the run. A redeclaration also fails it. The rest are warnings. The undefined-name checks compare against the file's own declarations plus the registered runtime bindings, and skip names the source guards with `function_exists` / `class_exists`. Without `--include`, a name arriving through an autoloader still warns, at warning level. It never fails the run. The redeclaration check reads the same set from the other side, and fails at error level: the runtime refuses a function declared twice in one file, or declared in place of a name it registers itself, whatever else the registry holds. A name the `--include` file declared is none of those - that file holds PHP like the file the linter checks. Comparing the two would report every function a bootstrap defines as a redeclaration of itself. A declaration the source guards with `function_exists` is the polyfill idiom and is not reported. The chained-assignment rule exists because phpscript arrays are handles, not values. Two names can share one array where PHP would keep a separate array per name. See [Value semantics](../reference/types/value-semantics.md#arrays-are-handles-not-values).

The linter reports the cases the parser leaves unfixed. The parser splits `$inlines = $blocks = array()` into a separate allocation per name. The linter reports neither that nor `$r['y'] = $r['m'] = '00'`, because no name can mutate a string through another. The linter does report a chain ending in a name, a call or a `new`: the value is either a handle the names really do share, or of a type the source does not settle.

The linter prints findings one to a row, with a row for each file without a finding, grouped the way `phpscript test` groups fixtures: one table per folder scanned, the folder naming the file column, closed by a per-folder summary line before the run's total. A terminal gets colored tables and redirected output gets Markdown. Use `--output FILE` (`-o`) to write the same tables to a file as Markdown while the terminal output continues as normal.

```bash
phpscript lint -o docs/lint.md tests/fixtures/...
```

Use `--flatstack` to also report whether the flat bytecode engine can run each file, which adds a failing row per file it cannot compile. See [Flat stack](../flatstack.md).

Use `--include FILE` to run a file against the name registry before checking. The checks then see the whole application, every file in scope. Without an include, every check runs against the standard library alone: a class composer autoloads, or a helper the application's bootstrap registers, are both unknown. Most findings are then false. No signal separates them from true ones. The class check autoloads only when `--include` names a file, because composer's autoloader declares nothing until a name reaches it.

```bash
phpscript lint --include vendor/autoload.php ./src/...
phpscript lint --include bootstrap.php ./...
```
