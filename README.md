# phpscript

This is an experimental PHP interpreter. It supports the basic php
expression syntax and some parts of the standard library. It's currently
very rudimentary and only enables limited functionality.

Several key parts of the project are:

- Using [expr-lang/expr](https://github.com/expr-lang/expr) for expression evaluation,
- A non-OOP subset of PHP syntax (no extends, no namespaces,...)
- Custom exception handling (throw new Exception)

If I'd have to put a version and ignore huge missing swaths of the
standard library, this would be a minimalist PHP without OOP constructs.

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
work, as well as writing files.

## A quick CLI benchmark

Going into `test/fixtures` and running `test-minitpl.php`:

- phpscript takes 0.015s per run
- php 8.5.4 takes 0.026s per run

Running `phpscript server`:

```bash
$ wrk http://localhost:8080/test-minitpl.php
Running 10s test @ http://localhost:8080/test-minitpl.php
  2 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    33.76ms   11.07ms 128.30ms   72.60%
    Req/Sec   148.90     19.84   190.00     72.00%
  2973 requests in 10.02s, 389.04KB read
Requests/sec:    296.63
Transfer/sec:     38.82KB
```

```bash
$ wrk http://localhost:8080/test-hello-world.php
Running 10s test @ http://localhost:8080/test-hello-world.php
  2 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   620.16us  537.23us   8.53ms   87.48%
    Req/Sec     9.14k   827.16    17.21k    82.59%
  182794 requests in 10.10s, 22.31MB read
Requests/sec:  18099.61
Transfer/sec:      2.21MB
```

I think there's some optimization room for phpscript here. Execution
currently repeats the parsing steps when loading files, so there should
be at least a third of overhead to optimize away by caching this.
