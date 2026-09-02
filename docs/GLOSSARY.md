# Glossary

The words this project uses, and what they mean here. Most are ordinary web
development terms; the entries that matter are the ones php, this runtime and
the Go code below it spell the same and mean differently.

## The request

| Term         | What it means                                                                                                                                                                                                 |
|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Request      | One HTTP exchange. A server builds one `runner.Runtime` for it, executes the entrypoint, and drops the runtime when the response is written. Nothing a request declared outlives it.                          |
| Entrypoint   | The one PHP file a request executes. Everything else it runs arrives through `include` or `require` from there. `phpscript run app.php` has one too; the file named on the command line.                      |
| Prelude      | The file `runner.include` or `--include` names, included ahead of every entrypoint. It is where a composer autoloader or a bootstrap goes, so the entrypoint and everything it includes see what it declared. |
| Superglobal  | A variable php binds in every scope without a `global` declaration: `$_GET`, `$_POST`, `$_SERVER`, `$_FILES`, `$_COOKIE`, `$_REQUEST`, `$_ENV`, `$_SESSION`. One binding per request.                         |
| SAPI         | Server API, the php name for what is running the interpreter. `php_sapi_name()` answers `cli` for a command line run and `cgi-phpscript` for a served request.                                                |
| Session      | php's `$_SESSION`, a per-visitor bag keyed by a cookie and stored by `Session\Manager`. It spans many requests, which is what makes it a session.                                                             |
| ResetSession | A Go method on `runner.Runtime`, and a different thing: it clears one runtime's declarations and globals so the runtime can execute another program. It has nothing to do with `$_SESSION`.                   |

## Where files live

| Term              | What it means                                                                                                                                                                                            |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Application root  | The directory a site is served from, the one `phpscript server` is pointed at. Includes resolve against it, and nothing above it is reachable.                                                           |
| Document root     | The directory below the application root that is served over HTTP, `public/` by default. A `.php` file in it runs when its name is requested; a file outside it does not.                                |
| Working directory | Where a relative path resolves from, inside the application root. `chdir()` moves it for the length of a request; `runner.work_dir` is where each one starts. `-w` moves the process before any of that. |
| Include root      | The directory a fixture's own relative includes resolve against, which is the directory holding the `.phpt` file.                                                                                        |
| Writable path     | A directory under `writable_paths` that scripts may write to. A `.php` file that lands in one is served as bytes and never executed, because an upload directory is content and not code.                |

## Loading names

| Term             | What it means                                                                                                                                                                                      |
|------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Include, require | Evaluate another PHP file here, in this scope. `require` is fatal when the file is missing and `include` is not; the `_once` forms consult a per-request record of what has already run.           |
| Autoloader       | A callback registered with `spl_autoload_register()` that is asked for a class the first time something names one that is not declared. composer's generated autoloader is one, interpreted as-is. |
| Namespace        | The prefix a declaration carries, `Acme\Thing`. It is a name, not a directory: what maps one onto a path is an autoloader.                                                                         |
| Binding          | A Go function, class or constant registered onto the runtime, which a script calls by the name it was registered under. `phpscript list --stdlib` prints every one this build carries.             |

## Serving

| Term              | What it means                                                                                                                                                                        |
|-------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Route             | A `// @route GET /path/{param}` comment on a PHP file outside the document root. The server scans for them at startup and mounts each on the router.                                 |
| Virtual host      | One of several sites in one process, each with its own root, its own `phpscript.yml`, its own database connections and its own environment. Requests reach one by the `Host` header. |
| Startup job       | A file carrying `// @startup`, executed once in path order before the server listens. Migrations go here.                                                                            |
| Scheduled job     | A file carrying `// @schedule`, started after the server listens and running until shutdown.                                                                                         |
| Error page        | A file in the document root named after a status, `public/404.php`. It answers a browser; a program gets the plain status.                                                           |
| Autoindex         | Answering a directory that holds no index page with a listing of what is in it, as nginx's `autoindex on;` does. Off by default.                                                     |
| Graceful shutdown | The path a server takes on SIGINT or SIGTERM: stop accepting, finish what is in flight, then stop the modules in registration order. It is when a coverage profile is written.       |

## Running the code

| Term       | What it means                                                                                                                                                                            |
|------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Runtime    | `runner.Runtime`, the tree-walking interpreter and the state one request executes against: the declarations, the globals, the output buffer, the registered bindings.                    |
| Flat stack | The compile-once bytecode backend. It compiles a whole program or delegates the whole program to the interpreter; there is no partial execution, because that would repeat side effects. |
| Fixture    | A `.phpt` file: metadata, PHP source, and the output php itself produces for it. `phpscript test` runs one and compares.                                                                 |
| Matrix     | Running each fixture on all three runtimes, flat stack, interpreter and the `php` binary, and reporting a column per runtime. A divergence between any two is a failure.                 |

## Coverage

| Term       | What it means                                                                                                                                                                                                |
|------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Statement  | The unit counted. Declarations are not executable and are not counted; a compound statement is charged for its header line only, so an untaken branch does not read as covered because its `if` was reached. |
| Block      | One line range of a profile, with how many statements it holds and how many times it ran. Statements sharing a line range merge into one.                                                                    |
| Profile    | The `.cov` file, in the format `go test -coverprofile` writes. `mode: count` and one line per block, so `go tool cover -html` renders it.                                                                    |
| Collector  | What one run reports statements to. It keys them by the syntax tree node the parser produced, so it belongs to the program it was registered against.                                                        |
| Aggregator | What a server folds each request's collector into. It keys blocks by the profile line they will be written as, so it is bounded by the size of the application however often a file is parsed.               |
| Mode       | `--cover=line` writes the profile, `func` and `file` also print a per-symbol report in the format `go tool cover -func` prints.                                                                              |
