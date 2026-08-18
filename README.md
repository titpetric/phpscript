# phpscript - A custom PHP-flavoured runtime written in Go

This is an experimental PHP interpreter. It supports the basic php expression syntax and some parts of the standard library. It's currently very rudimentary and only enables limited functionality.

- [About phpscript](./docs/README.md)
- [Language reference and PHP compatibility](./docs/reference/README.md)
- [Installation and usage](./docs/usage.md)
- [Configuration](./docs/configuration.md)
- [Testing and extending tests](./docs/testing.md)
- [Naming conventions](./docs/naming-conventions.md)
- [Go Reference](https://pkg.go.dev/github.com/titpetric/phpscript)
- [Building an application](./docs/use-cases/application.md)
- [Use cases](./docs/use-cases)

## Current state

Several unit focused test fixtures exists, namely:

1. interfacing Go code from the PHP VM for callbacks,
2. database client and usage from PHP side (bring your own database/sql driver)
3. loading and executing template engine code

This means that phpscript is currently able to use a generic Database client. Our own standard library implements a `Database` class which is integration tested with [./tests/fixtures/test-database.php](./tests/fixtures/test-database.php) over several databases (pgx, mysql, sqlite).

## Building a docker image

You can build a docker image from source as follows:

```
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [./compose.yml](./compose.yml) for usage.

## Contributing

Contributions welcome. Open an issue to discuss before opening PRs.
