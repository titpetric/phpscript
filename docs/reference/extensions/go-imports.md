# Go imports

`use Runtime\Go;` binds go's builtins (see [the Go runtime](go-runtime.md)). It also binds one statement:

```php
<?php

use Runtime\Go;

import "net/http";
import "io";

$client = new http.Client;
$req = http.NewRequest("GET", "https://example.com/a", null);
$req->Header->Set("Accept", "application/json");

$resp = $client->Do($req);
echo $resp->StatusCode, " ", io.ReadAll($resp->Body);
```

One path per statement, quoted, the path go writes. The statement is the go spelling of the `use` of an area: `import "net/http"` installs `Runtime\Go\Net\HTTP` on the runtime that runs the file, and `http.NewRequest` resolves to `Runtime\Go\Net\HTTP\NewRequest` while the file parses. The name a script addresses the package by is the path's last segment.

A file that writes no `use Runtime\Go;` reads `import` as an ordinary name, and the statement, the dot and the packages stay out of php's grammar. php rejects the file either way: keep the import out of source that must also run on php.

## The dot

A dot beside a package the file imported is a name. Everywhere else it concatenates, as php's does:

```php
import "strings";

echo strings.ToUpper("go");      // the package's function
echo "a" . "b";                  // php's concatenation
echo strings . "x";              // php's too: no member follows the dot
```

The import takes the name for the file that wrote it. A file holding a constant named `http` and an `import "net/http"` reads `http . FOO` as the member `FOO` of the package; name the constant something else, or reach the area by its own name.

## What an import binds

| A script writes           | It resolves to                   | The name holds                      |
|---------------------------|----------------------------------|-------------------------------------|
| `http.NewRequest()`       | `Runtime\Go\Net\HTTP\NewRequest` | a function of the package           |
| `http.StatusOK`           | `Runtime\Go\Net\HTTP\StatusOK`   | a constant of the package           |
| `new http.Request`        | `Runtime\Go\Net\HTTP\Request`    | a type the area enumerated          |
| `make([]http.Request, 2)` | `[]Runtime\Go\Net\HTTP\Request`  | the same type, as a slice's element |

The area answers under its own name as well, and `Runtime\Go\Net\HTTP\StatusText(404)` needs no import. `function_exists` reports the resolved name. The dotted spelling belongs to the parser. The runtime answers for the area name alone.

## The packages

| Import          | Area                       | Binds                                                                      |
|-----------------|----------------------------|----------------------------------------------------------------------------|
| `fmt`           | `Runtime\Go\Fmt`           | `Sprintf`, `Sprint`, `Sprintln`                                            |
| `io`            | `Runtime\Go\IO`            | `ReadAll`, `ReadFull`, over a reader a binding answered with               |
| `net/http`      | `Runtime\Go\Net\HTTP`      | `Get`, `Head`, `NewRequest`, `StatusText`, the method and status constants |
| `strings`       | `Runtime\Go\Strings`       | `Fields`, `Split`, `Contains`, `Index`, `Repeat`, `Trim` and the rest      |
| `slices`        | `Runtime\Go\Slices`        | `Contains`, `Index`, `Sort`, `Reverse`, `Max`, `Min`, `Compact`, `Concat`  |
| `maps`          | `Runtime\Go\Maps`          | `Keys`, `Values`, `Clone`, `Copy`, `Equal`                                 |
| `path`          | `Runtime\Go\Path`          | `Base`, `Dir`, `Ext`, `Clean`, `Join`, `Match`                             |
| `encoding/json` | `Runtime\Go\Encoding\JSON` | `Marshal`, `Unmarshal`, `Valid`                                            |
| `database/sql`  | `Runtime\Go\Database\SQL`  | `Open`, on a connection the host registered                                |

[Runtime areas](runtime-areas.md) lists each area's names. A path the mapping does not spell is a parse error naming the line: the import reaches the standard library, and a module path reaches nothing.

The `net/http` area reaches any address the host can route to, as `HTTP\Client` does. The server half stays out: a php handler answers through `HTTP\Server`, which owns the re-entry the runtime needs ([an HTTP server](../../use-cases/http-server.md)).

## Writing a package

A binding package names what it binds, one entry per name, and `stdlib/go/bindings` does the rest:

```go
package http

import (
	gohttp "net/http"

	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

var Set = bindings.For("net/http").
	Bind("NewRequest", gohttp.NewRequest).
	Bind("StatusOK", gohttp.StatusOK).
	Type(gohttp.Client{})

func init() {
	Set.Install()
}
```

An entry is a triple: the import path of the set, the name go spells, the value. A func binds as a function of the area, and every other value as one of its constants. `Install` contributes the area, and the import installs it on the runtime that reads the file.

The folder is the area, as every binding folder is: `stdlib/go/net/http` is `Runtime\Go\Net\HTTP`. A new package lands as a folder of its import path, with its names in one file and its test beside it.

### The types come out of the signatures

`Bind` names functions. The types a script reaches are the ones those signatures already imply. The set walks them: `http.NewRequest` names `*http.Request`, whose fields name `http.Header` and the url behind it, and whose methods name `*http.Cookie`. Every struct of the package the walk reaches registers as a class of the area, and `new http.Request` and `make([]http.Request, 2)` resolve to one.

The walk stops six levels from a bound signature; past that it pulls in the transitive closure of the standard library for no gain. A struct the walk misses, `http.Client` among them, is one line: `Type(gohttp.Client{})` seeds the walk from the type itself. An interface, a named map and a func type register no class, because `new` would answer a nil of one. The technique is [gozero](https://github.com/titpetric/gozero)'s, which discovers the types behind its bindings the same way.

### A package this build does not link

A binding binds a package the binary already links. Binding one it does not grows the binary and the dependency surface of every host that embeds the runtime, which is a decision and not a side effect of adding a function. `bindings.Linked` holds the list, read off the released binary's symbols and sanitised; `bindings.For` panics on a path the list omits. Adding a path to the list belongs in the change that binds it, with the reason in the commit.

```bash
go tool nm $(which phpscript) \
  | awk '{print $3}' \
  | grep -oE "^[a-z][a-z0-9]*(/[a-z0-9_]+)*\." \
  | sed 's/\.$//' | sort -u
```

## What stays out

| Not bound                                | Why                                                                                        |
|------------------------------------------|--------------------------------------------------------------------------------------------|
| An aliased import, `import x "net/http"` | The path's last segment is the name. An area reached under another name is the `use` of it |
| A grouped import, `import ( ... )`       | One path per statement, as the go spelling of a `use` is one name per statement            |
| `fmt.Println`, `fmt.Printf`              | A script writes its output with `echo`, past the output buffer the runtime holds           |
| `http.ListenAndServe`, `http.HandleFunc` | `HTTP\Server` owns the re-entry a php handler needs                                        |
| A channel, `go`, `select`                | [No channels](go-runtime.md#no-channels); the decision stands                              |

`phpscript lint` reports a call into an opt-in area as an undefined function, as it does for a `use` of one: the linter reads the file and the areas install on the runtime that runs it.
