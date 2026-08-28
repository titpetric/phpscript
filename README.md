# phpscript - A custom PHP-flavoured runtime written in Go

This is a PHP interpreter written in Go. It supports the basic php expression syntax and some parts of the standard library. It's currently rudimentary and only enables limited functionality.

- [About phpscript](./docs/README.md)
- [Design decisions](./docs/design.md)
- [Language reference and PHP compatibility](./docs/reference/README.md)
- [Installation and usage](./docs/usage.md)
- [Configuration](./docs/configuration.md)
- [Testing and extending tests](./docs/testing.md)
- [Test fixture results](./docs/test-fixtures.md)
- [Code coverage](./docs/code-coverage.md)
- [Naming conventions](./docs/naming-conventions.md)
- [Go Reference](https://pkg.go.dev/github.com/titpetric/phpscript)
- [Creating phpscript applications](./docs/guides/creating-phpscript-applications.md)
- [Building an application](./docs/use-cases/application.md)
- [Use cases](./docs/use-cases)

## Current state

Behaviour is settled by `.phpt` fixtures: each one is written by running the
source through real `php` first, and `phpscript test tests/fixtures/...`
checks the runtime against that output. The generated
[test fixture results](./docs/test-fixtures.md) hold the per-fixture matrix;
the bird's-eye view per area:

| Area                       | Fixtures | Passed | Failed |
|----------------------------|----------|--------|--------|
| tests/fixtures/arithmetic  | 21       | 21     | 0      |
| tests/fixtures/arrays      | 18       | 18     | 0      |
| tests/fixtures/autoloading | 8        | 8      | 0      |
| tests/fixtures/bindings    | 23       | 23     | 0      |
| tests/fixtures/errors      | 2        | 2      | 0      |
| tests/fixtures/exceptions  | 9        | 9      | 0      |
| tests/fixtures/flatstack   | 7        | 7      | 0      |
| tests/fixtures/functions   | 9        | 9      | 0      |
| tests/fixtures/includes    | 3        | 3      | 0      |
| tests/fixtures/namespaces  | 2        | 2      | 0      |
| tests/fixtures/oop         | 23       | 23     | 0      |
| tests/fixtures/output      | 4        | 4      | 0      |
| tests/fixtures/paths       | 2        | 2      | 0      |
| tests/fixtures/regex       | 7        | 7      | 0      |
| tests/fixtures/runtime     | 11       | 11     | 0      |
| tests/fixtures/stdlib      | 16       | 16     | 0      |
| tests/fixtures/strings     | 15       | 15     | 0      |
| tests/fixtures/syntax      | 8        | 8      | 0      |
| **Total**                  | 188      | 188    | 0      |

The Go test run collects a coverage profile (about 84% of statements at the
time of writing), and `atkins cover` renders it into the generated
[code coverage](./docs/code-coverage.md) report, per package and per function.

## Building a docker image

You can build a docker image from source as follows:

```
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [./compose.yml](./compose.yml) for usage.

## Contributing

Contributions welcome. Open an issue to discuss before opening PRs.
