# A worked example, and the walkthrough that builds it

**2026-08-17.** `demos/example` is a bookmark list: three annotated endpoints, a
schema applied at startup, a compiled template and a static asset — the smallest
application that still uses every part of the server. [Building an
application](../use-cases/application.md) builds it from an empty directory.
Every snippet in the walkthrough is taken from the demo, and the demo has a
venom suite, so the documented code is code that runs.

## `phpscript list` names startup files

The inventory had no entry point for a file like `migrate.php`: it carries no
`@route`, so it was listed with an empty Route column, next to files that are
only reachable through an `include`. A file the server runs before it listens is
an entry point, and the column now says so:

```text
  | Route                       | Filename                                     |
  |-----------------------------|----------------------------------------------|
  | POST /bookmarks             | [bookmark-create.php](./bookmark-create.php) |
  | GET /                       | [index.php](./index.php)                     |
  | @startup                    | [migrate.php](./migrate.php)                 |
```

`startup.annotated` became `startup.Annotated` for it, so the rule for what
counts as an annotation lives in one place instead of being restated by the
`list` package.

## The pipeline

`compose.yml` gained an `example` service, and the root pipeline runs both demo
suites through `test:demos:dbadmin` and `test:demos:example` after the image is
built. Each demo keeps its own `tests/atkins.yml`, because the suite has to
resolve the address of its own container first.

The pipeline also clears `GOFLAGS`. A `-mod=mod` inherited from the environment
makes every go command fail with `-mod may only be set to readonly or vendor when in workspace mode` in a checkout that sits inside a `go.work`, which is how
this repository is developed alongside `pdo` and `oida`.
