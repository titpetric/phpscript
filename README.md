# phpscript - A custom PHP-flavoured runtime written in Go

This is a PHP interpreter written in Go, with a bytecode backend beside it and an HTTP server of its own. It runs a subset of the language and of the standard library - `phpscript list --stdlib` prints what this build carries, and `phpscript info` counts it - and the parts it does not run are decisions as often as gaps: see [Design decisions](./docs/design.md) for what it will not implement and [the language reference](./docs/reference/README.md) for what is not implemented yet.

- [About phpscript](./docs/README.md)
- [Design decisions](./docs/design.md)
- [Language reference and PHP compatibility](./docs/reference/README.md)
- [Installation and CLI reference](./docs/cli/README.md)
- [Configuration](./docs/configuration.md)
- [Testing and extending tests](./docs/testing.md)
- [Test fixture results](./docs/test-fixtures.md)
- [Code coverage](./docs/coverage/phpscript.md)
- [Naming conventions](./docs/naming-conventions.md)
- [Glossary](./docs/GLOSSARY.md)
- [Go Reference](https://pkg.go.dev/github.com/titpetric/phpscript)
- [Building an application](./docs/use-cases/application.md)
- [Use cases](./docs/use-cases)

## Current state

Behaviour is settled by `.phpt` fixtures: each one is written by running the source through real `php` first, and `phpscript test --matrix ./tests/...` checks the runtime against that output on both engines and on `php` itself. The [test fixture results](./docs/test-fixtures.md) are generated from that run and hold every fixture, its area and the three columns, which is where the counts live: a table copied into this page goes stale the moment an area grows, as this one did.

The Go test run collects a coverage profile, and `atkins cover` renders it into the generated [code coverage](./docs/coverage/phpscript.md) report, per package, with [the detail](./docs/coverage/phpscript-detail.md) per file. The dbadmin demo is measured the same way from a running server, in [dbadmin.md](./docs/coverage/dbadmin.md).

## Building a docker image

You can build a docker image from source as follows:

```
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [./compose.yml](./compose.yml) for usage.

## Contributing

Contributions welcome. Open an issue to discuss before opening PRs.
