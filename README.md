# phpscript

This is an experimental PHP interpreter. It supports the basic php
expression syntax and some parts of the standard library. It's currently
very rudimentary and only enables limited functionality.

Several key parts of the project are:

- Using [expr-lang/expr](https://github.com/expr-lang/expr) for expression evaluation,
- A non-OOP subset of PHP syntax (no extends, no namespaces,...)
- Custom exception handling (throw new Exception)

There is no stated PHP version compatibility here, but it should feel
most similar to PHP4 syntax with some PHP5 features sprinkled around.

Various stuff is missing and a compatibility table is currently not maintained.

## Go-PHP bridge

Aside from PHP language compatibility, the VM allows to inject structs
into the PHP VM and then use them. The methods automatically get the
request context, and any error returned is explicitly handled and
thrown. To give you a basic idea from a passing test fixture:

```php
<?php

$storage = new Storage;
$storage->set("greeting", "hello");
$storage->set("name", "world");

$greeting = $storage->get("greeting");
$name = $storage->get("name");
$count = $storage->len();
```

This code works with the following implementation:

```go
type Storage interface {
        Set(ctx context.Context, key, value string)
        Get(ctx context.Context, key string) (Record, error)
        All(ctx context.Context) ([]Record, error)
        Len() int64
        Tenant() string
}

// NewStorage is the constructor registered for `new Storage`. Its first
// parameter is a context.Context, filled in automatically by the runner, so PHP
// calls `new Storage` with no arguments.
func NewStorage(ctx context.Context) (Storage, error) {
        if ctx == nil {
                return nil, errors.New("storage: nil context")
        }
        tenant, _ := ctx.Value(tenantKey).(string)
        return &memStorage{data: map[string]string{}, tenant: tenant}, nil
}
```

The binding to enable interfacing with `Storage` as if it were a PHP
class is a simple one-liner:

```go
rt := runner.New(os.Stdout)
rt.RegisterConstructor("Storage", NewStorage)
```

The context value is filled from the request, and the errors returned
are promoted to a request error. The request can be handled either by
the VM instance, or by using `try` and `catch` statements in the script.

## PHP Compatibility

While not much of PHP is directly compatible due to the missing standard
library APIs, there is some coverage to pass loading a PHP template
engine and using it. In order for that to work, several low level PHP
functions needed to be implemented:

- `token_get_all`
- `include`, `require`, `eval` ...
- filesystem abstracted to enable bundling of an embedded FS

The implementation surface is essentially a stub right now. In order to
use it, there is `stdlib.Register` and `stdlib.RegisterFS`. If you don't
register a FS with the interpreter, `include` functionality will not
work, as well as other file APIs like `file_get_contents`,
`file_put_contents`, `filemtime`, `fopen`,...

## A quick HTTP benchmark

Used: PHP 8.5.4

- `php -S 0:8080` from tests/fixtures/
- `phpscript server` from tests/fixtures/

|                    | php 8.5.4     | phpscript      | multiplier    |
|--------------------|---------------|----------------|---------------|
| hello-world.php    | ~8000 req/sec | ~18000 req/sec | 2.25x better  |
| test-minitpl.php   | ~5500 req/sec | ~350 req/sec   | 15x worse     |

More tests are needed. Currently, the project doesn't have any caching
optimisations to skip repeating work related to parsing the source.

Ideally `minitpl` can get to a comparable or better request rate than
PHP. To consider a 30% slowdown same as with stdlib php, the expected
request rate for phpscript is about 12,600 request per second once the
underlying performance issue is adressed.

No go benchmarks have been added yet. Ideally test fixtures are also
reused as benchmarks, to compare a native go implementation against the
same implementation running via phpscript.

## Contributing

Contributions welcome. Open an issue to discuss before opening PRs.
