# phpscript - A custom PHP-flavoured runtime written in Go

This is a PHP interpreter written in Go, with a bytecode backend beside it and an HTTP server of its own. It runs a subset of the language and of the standard library - `phpscript list --bindings` prints what this build carries, and `phpscript info` counts it - and the parts it does not run are decisions as often as gaps: see [Design decisions](./docs/design.md) for what it will not implement and [the language reference](./docs/reference/README.md) for what is not implemented yet.

## One API, five runtimes

One set of endpoints written five ways, each its own directory under `testdata/`, all driven by one load generator. `GET /hello` at concurrency 1, four workers a side, four cores, six segments of 3s, measured on `669f5c9` by `scripts/bench-http.sh`:

| Runtime                                                                                                    | `/hello`               | p90 us | p99 us |
|------------------------------------------------------------------------------------------------------------|------------------------|--------|--------|
| [`goserver`](./testdata/goserver/README.md), net/http and no VM, the floor                                 | 26719 req/s, 35 us/req | 41     | 106    |
| [`phpscript-http-bridge`](./testdata/phpscript-http-bridge/README.md), a PHP program that is the server    | 19441 req/s, 47 us/req | 56     | 161    |
| [`phpscript-service`](./testdata/phpscript-service/README.md), `@route` endpoints under `phpscript server` | 13089 req/s, 70 us/req | 87     | 291    |
| [`php-vanilla`](./testdata/php-vanilla/README.md), Slim on nginx with php-fpm                              | 2525 req/s, 373 us/req | 466    | 725    |
| [`php-vanilla`](./testdata/php-vanilla/README.md), Slim on apache with mod_php                             | 2017 req/s, 472 us/req | 562    | 801    |

Each directory's README states what its numbers mean and what its stack costs per request. `phpscript-http-bridge` and `phpscript-service` answer under two different execution models, Path B and Path A, and [the comparison](./docs/agents/performance.md) sets out which figures belong in a table together. This one goes stale the moment either path changes, so it carries the commit it describes, and these two commands write it again:

```sh
BENCH_TARGETS="goserver bridge service vanilla-apache vanilla-nginx" scripts/bench-http.sh after
scripts/benchmark.php
```

- [About phpscript](./docs/README.md)
- [Design decisions](./docs/design.md)
- [Language reference and PHP compatibility](./docs/reference/README.md)
- [Installation and CLI reference](./docs/cli/README.md)
- [Configuration](./docs/configuration.md)
- [Security](./docs/security.md)
- [Testing and extending tests](./docs/testing.md)
- [Test fixture results](https://github.com/titpetric/phpscript/releases/latest/download/test-fixtures.md)
- [Code coverage](https://github.com/titpetric/phpscript/releases/latest/download/phpscript.md)
- [Agent sprints and the measurement protocol](./docs/agents/README.md)
- [Naming conventions](./docs/naming-conventions.md)
- [Adding a binding, start to finish](./docs/bindings-regexp.md)
- [Glossary](./docs/GLOSSARY.md)
- [Go Reference](https://pkg.go.dev/github.com/titpetric/phpscript)
- [Building an application](./docs/use-cases/application.md)
- [Use cases](./docs/use-cases)

## Current state

Behaviour is settled by `.phpt` fixtures: each one is written by running the source through real `php` first, and `phpscript test --matrix ./tests/...` checks the runtime against that output on both engines and on `php` itself. The [test fixture results](https://github.com/titpetric/phpscript/releases/latest/download/test-fixtures.md) are generated from that run and hold every fixture, its area and the three columns. A table copied into this page goes stale the moment an area grows, as this one did.

The Go test run collects a coverage profile, and `atkins cover` folds it into splint's document of the tree to render the generated [code coverage](https://github.com/titpetric/phpscript/releases/latest/download/phpscript.md) report, per package, with [the detail](https://github.com/titpetric/phpscript/releases/latest/download/phpscript-detail.md) per function. Each row carries the cognitive complexity and the line count beside the percentage, so a low number says whether there is a branch left to cover or nothing to cover at all. The dbadmin demo is measured the same way from a running server, in [dbadmin.md](https://github.com/titpetric/phpscript/releases/latest/download/dbadmin.md).

Those five documents change on every run, so they are gitignored: `atkins gen` writes them and `atkins publish` attaches them to the release, where the links above resolve to.

## Building a docker image

You can build a docker image from source as follows:

```
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [./compose.yml](./compose.yml) for usage.

## Contributing

Contributions welcome. Open an issue to discuss before opening PRs.
