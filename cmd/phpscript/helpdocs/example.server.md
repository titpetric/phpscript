| Example | What it does |
|---|---|
| `phpscript server` | Serves the current directory on `:8080`: `public/` over HTTP, `@route` files anywhere below. |
| `phpscript -f config.yml server .` | Serves one tree with a configuration read over the built-in defaults. |
| `phpscript --include vendor/autoload.php server` | Includes the autoloader ahead of every request entrypoint and every `@startup` job. |
| `phpscript --cover --coverfile=cover/site.{time}.cov server` | Counts every statement the site runs and writes the profile when the server stops. `{time}` becomes a UTC timestamp. |
| `curl localhost:8080/debug/phpscript/coverage` | Reads the profile out of a running server. `?mode=file` and `?mode=func` return the report instead. |
