# phpscript-go-runtime-server

The same routes as [`phpscript-http-bridge`](../phpscript-http-bridge/README.md),
written against the go runtime namespace. `phpscript.yml` sets
`runner.runtime: go`, so the source writes no `use Runtime\Go;` line and every
file reads `import "net/http"` and the go builtins directly.

```sh
phpscript -f testdata/phpscript-go-runtime-server/phpscript.yml \
    run testdata/phpscript-go-runtime-server/server.php
```

`TESTSERVER_ADDR` defaults to `127.0.0.1:8099` and `127.0.0.1:0` asks the
kernel for a port. `TESTSERVER_WORKERS` defaults to the logical CPU count,
never below 2, and `TESTSERVER_QUEUE` to 64. The process runs until Ctrl-C, SIGTERM or SIGHUP; the go runtime has no
`set_time_limit`, so `TESTSERVER_LIMIT` is unused.

The handlers take the raw `net/http` request and a writer: `$r->Method`,
`$r->URL->Path`, `$r->URL->Query()->Get($name)`, `$r->PathValue($name)`,
`$r->FormValue($name)` and `$w->Header()->Set($name, $value)`. A handler writes
its body with `echo`, which reaches the response writer. `/` is an HTML page
mixing tags and evaluated code, with a fragment captured through
`ob_start()` and `ob_get_clean()`. The JSON routes write with
`json.NewEncoder($w)->Encode($body)`, which converts a php array the way
`json_encode` does. `/slow` uses `time.Sleep("100ms")` and `http.Aborted($r)`
where the bridge uses `usleep` and `connection_aborted()`.

The methods carry go's type hints: `config(): array`, `mount():
http.ServeMux`, `http.ResponseWriter` and `*http.Request` on the handler
parameters, `void` on every handler. The parser keeps a hint and the formatter
prints it back, and no engine enforces one.

The runtime never falls back to php: `runtime: go` installs `Runtime\Go` and no
area under `Runtime\PS` or `Runtime\PX`, so `phpinfo`, `intval` and
`set_time_limit` are absent by design.
