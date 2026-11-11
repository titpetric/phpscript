# Runtime areas

The runtime's bindings belong to namespaces called areas. An area is a group of functions, classes and constants that one Go installer registers, under a name that is a namespace path. Three namespaces hold them:

| Namespace      | Holds                                                                    | Installed                      |
|----------------|--------------------------------------------------------------------------|--------------------------------|
| `Runtime\PS`   | phpscript's own surface: php's standard library and the extensions       | by default, first              |
| `Runtime\PX`   | the phpx overlay: the php compatibility work installed over `Runtime\PS` | by default, after `Runtime\PS` |
| `Runtime\Go`   | a subset of go's standard library, written in php                        | by `use`, per file             |
| `Runtime\None` | no area: the language alone                                              | when `runner.areas` names it   |

`phpscript list --areas` prints the areas of a build with what each defines.

## Folders map to areas

A folder of `stdlib` is an area, and a file of a large folder is a sub-area:

| Folder               | Area                                                        |
|----------------------|-------------------------------------------------------------|
| `stdlib/ps/core`     | `Runtime\PS\Core\<File>`, such as `Runtime\PS\Core\Strings` |
| `stdlib/ps/compat`   | `Runtime\PS\Compat`                                         |
| `stdlib/ps/crypto`   | `Runtime\PS\Crypto`, with `Hash`, `Random`, `Identifiers`   |
| `stdlib/ps/files`    | `Runtime\PS\Files`                                          |
| `stdlib/ps/http`     | `Runtime\PS\HTTP`, with `SAPI`                              |
| `stdlib/ps/pexec`    | `Runtime\PS\Exec`                                           |
| `stdlib/px/<folder>` | `Runtime\PX\<Folder>`, the overlay a host adds              |
| `stdlib/go/<pkg>`    | `Runtime\Go\<Pkg>`, such as `Runtime\Go\Strings`            |

The directory tree is the namespace tree: `stdlib/ps` holds `Runtime\PS`, `stdlib/px` holds `Runtime\PX` and `stdlib/go` holds `Runtime\Go`. A package keeps the import path its area name spells.

The remaining folders (`auth`, `database`, `gd`, `info`, `internals`, `mail`, `regexp`, `session`, `span`, `time`) map to the area of the same name under `Runtime\PS`. A file named `phpx_<name>.go` inside a `stdlib/ps` folder registers `Runtime\PX\<Folder>\<Name>`: it overlays the folder's area and installs with the rest of `Runtime\PX`.

`stdlib` itself installs two areas no folder maps to, ahead of every binding package: `Runtime\PS\Exceptions` holds `Exception` and the SPL exception classes, and `Runtime\PS\Errors` holds `Error`, `TypeError`, `ValueError`, `ArithmeticError`, `DivisionByZeroError` and `ArgumentCountError`. They install on every mount, `Runtime\None` included, because a runtime with no binding still throws.

The [implemented API inventory](implemented-apis.md) groups every function and class by area.

## The overlay installs last

Every `Runtime\PS` area installs first, in registration order. Every `Runtime\PX` area installs after them, in registration order. A `Runtime\PX` binding that registers a name `Runtime\PS` already defines replaces it, and the area listing counts it under Replaces. The order follows the namespace and does not depend on which Go package initialised first.

## Calling a function through its area

An area is a namespace. A function answers under it as well as under its bare name:

```php
echo \Runtime\PS\Core\Strings\strlen("four");   // 4
var_dump(function_exists('Runtime\PS\Core\Strings\strlen')); // bool(true)
```

The area has to be the one that defines the function, or the one that replaces it. `Runtime\PS\Core\Arrays\strlen` names nothing. Both runtimes resolve the name the same way. php has no `Runtime` namespace, and source that must also run on php calls the bare name.

## Reaching a class through its area

A class answers under its area the same way. The area's last segment and the class name decide the spelling:

| Area                    | Class                       | Also answers to                          |
|-------------------------|-----------------------------|------------------------------------------|
| `Runtime\PS\Database`   | `Database`                  | `Runtime\PS\Database`                    |
| `Runtime\PS\Database`   | `Database\Migrate`          | `Runtime\PS\Database\Migrate`            |
| `Runtime\PS\Auth`       | `Auth\Storage\PasswordFile` | `Runtime\PS\Auth\Storage\PasswordFile`   |
| `Runtime\PS\Core\JSON`  | `JSON\Encoder`              | `Runtime\PS\Core\JSON\Encoder`           |
| `Runtime\PS\Exceptions` | `RuntimeException`          | `Runtime\PS\Exceptions\RuntimeException` |
| `Runtime\PS\Errors`     | `TypeError`                 | `Runtime\PS\Errors\TypeError`            |

`runner.AreaName` builds the spelling. A class named exactly like the area's last segment takes the area. A class whose name opens on that segment drops it. Everything else hangs off the area whole.

Both comparisons are exact where the lookup that follows folds case. A folded comparison would resolve `Runtime\PX\Datetime` to the class `DateTime`, and the area a namespace page documents would be a class page as well. `DateTime`, `DateInterval`, `DateTimeImmutable` and `DateTimeZone` each keep a segment under the area for that reason.

The class table gains no entry. `get_class()`, `get_declared_classes()` and `phpscript list --classes` report the registered name, and `class_exists('Runtime\PS\Errors\TypeError')` answers true through `runner.areaClass` on the miss.

`Runtime\Router::Bind` is the one root definition no area name reaches. It names a static method, and nothing registers a constructor for `Runtime\Router`. The class table holds no entry to resolve to.

## Configuring the default runtime

The `runner.areas` key of the configuration names the namespaces the default runtime installs. The key is empty by default, which installs every area:

```yaml
runner:
  areas:
    - Runtime\PS
```

That runtime holds `Runtime\PS` and no overlay. A deeper name narrows further: `Runtime\PS\Core\Strings` installs the string functions and no other part of the standard library, which suits a runtime for a template engine that never calls a file function. A host sets `Options.Areas` on `runner.Options` for the same effect.

`Runtime\None` installs no area. The runtime keeps the language and the exception classes, and no function resolves, `function_exists` included. It suits a plugin host whose scripts compute over values the host passes in, and a test that proves a script needs no binding: `gotest.RunAreas(t, name, []string{runner.NamespaceNone}, cases)`. A host that needs bindings back names them (`Runtime\PS\Core\Strings`), and the default, an empty list, is `Runtime\PS`: php's standard library in the global function table.

A name outside the installed areas stays undefined, and `function_exists` answers false. The `--stdlib` flag is the other axis: it selects profiles, such as `secure` and `exec`, and the two compose.

## Go's standard library, opt-in

`Runtime\Go` holds go's builtins (`make`, `len`, `append`; see [the Go runtime](go-runtime.md)) and the areas below it map go's import paths:

| Area                       | Go package      | Names                                                                              |
|----------------------------|-----------------|------------------------------------------------------------------------------------|
| `Runtime\Go\Strings`       | `strings`       | `Fields`, `Split`, `Contains`, `HasPrefix`, `Index`, `Repeat`, `Trim`              |
| `Runtime\Go\Path`          | `path`          | `Base`, `Dir`, `Ext`, `Clean`, `Join`, `Match`                                     |
| `Runtime\Go\Slices`        | `slices`        | `Contains`, `Index`, `Sort`, `Reverse`, `Max`, `Min`, `Compact`, `Concat`, `Equal` |
| `Runtime\Go\Maps`          | `maps`          | `Keys`, `Values`, `Clone`, `Copy`, `Equal`, in the order the array keeps           |
| `Runtime\Go\Fmt`           | `fmt`           | `Sprintf`, `Sprint`, `Sprintln`                                                    |
| `Runtime\Go\IO`            | `io`            | `ReadAll`, `ReadFull`, over a reader a binding answered with                       |
| `Runtime\Go\Net\HTTP`      | `net/http`      | `Get`, `Head`, `NewRequest`, `StatusText`, the methods and the status codes        |
| `Runtime\Go\Encoding\JSON` | `encoding/json` | `Marshal`, `Unmarshal`, `Valid`                                                    |
| `Runtime\Go\Database\SQL`  | `database/sql`  | `Open` a connection the host registered, never a DSN from the script               |

A script imports one by its name, and the alias calls the function:

```php
use Runtime\Go\Strings;

echo implode("|", Strings\Fields("  a b   c "));   // a|b|c
```

The default runtime defines none of these names. The import installs the area on the runtime that runs the file, and `function_exists('Runtime\Go\Strings\Fields')` is false before it.

- `Strings\Repeat` throws a `RuntimeException` when the result would pass 256 MiB.

The mapping continues down go's import paths: `net/http` is `Runtime\Go\Net\HTTP`, `path/filepath` is `Runtime\Go\Path\Filepath`. A package that reaches outside the sandbox, such as a network client, registers under its own profile area, as `stdlib/ps/pexec` does. A host serving untrusted scripts leaves that profile out.

A file that bound the go runtime writes the go spelling of the same import, which resolves to the same area: `import "net/http";` then `http.Get($url)`. [Go imports](go-imports.md) is that statement, the packages it reaches and how a binding package declares one.

## Writing an area

A binding package registers each installer under a name in its `init`:

```go
func init() {
	runner.RegisterArea(`Runtime\PS\Core\Strings`, Register)
}
```

`RegisterArea` takes the name and the installer; `RegisterProfileArea` adds the profile mask for an area that leaves the sandbox. More than one file may register under one name. An installer that registers no name goes under `Runtime\PS\Unnamed`, which `runner.RegisterBinding` still does for a host.

An opt-in area registers with `runner.RegisterNamespace(name, installer)`, and names its functions with the full path (`Runtime\Go\Strings\Fields`), which the `use` alias shortens.

## Listing the areas

```bash
phpscript list --areas                 # Every area, in install order
phpscript list --areas 'Runtime\PX'    # The overlay only
phpscript list --areas --json          # The rows as data
phpscript list --bindings              # Every name, with the area that defines it
```

The Replaces column counts the names an area registers over. `stdlib.Areas` returns the same data to a Go program, and `stdlib.Owners` maps each name to its first definer.
