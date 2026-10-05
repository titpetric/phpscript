# Collect: phpscript run

A pprof cpu or heap profile, or a PHP coverage profile, of one script with no test harness inside the profile.

Read this when the question is which function the cost is in. It runs under the contract in [README.md](README.md); that document is not restated here.

## What this collects

One script, one profile, and nothing else in the frames.

That is what makes this the per-function tool. A profile of `phpscript test` carries fixture discovery, table rendering, the `php` subprocess and the JSON writer on top of the interpreter; a profile of `phpscript run` carries the script.

## Preconditions

The contract's lock and private binary.

The profile is written when the command ends. A script killed with `SIGKILL` loses it. For a server, that means arranging for the process to end on its own: `testdata/testserver.php` ends on its `set_time_limit`, and `TESTSERVER_LIMIT` is what sets how long the profile covers.

Two parsing traps:

- `-t` before a command name is `--testconfig` and runs that instead of the command. Write global flags where their meaning is unambiguous.
- `run` is the default command, so `phpscript app.php` and `phpscript run app.php` are the same thing. Both accept the global flags.

`run` has no flags of its own. It reads the global set: `--cover`, `--coverfile`, `--cpuprofile`, `--memprofile`, `-f`, `--include`, `-v`, `-w`.

## The run

A cpu profile of one script:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  phpscript --cpuprofile=cpu-after.pprof run tests/fixtures/strings/heavy.php
'
```

A heap profile, which is taken after a forced collection when the command ends:

```sh
phpscript --memprofile=mem-after.pprof run path/to/script.php
```

Both at once is allowed and is one run of the script.

The server, where the time limit is what ends the process and therefore writes the profile:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  TESTSERVER_ADDR=127.0.0.1:8099 TESTSERVER_LIMIT=60 \
    phpscript --cpuprofile=cpu-testserver.pprof run testdata/testserver.php &
  sleep 2
  hey -n 20000 -c 1 http://127.0.0.1:8099/hello | tee bench-http-after.txt
  wait
'
```

The load has to finish inside the limit, and the profile covers the whole lifetime including the idle seconds either side of it.

PHP statement coverage of one script:

```sh
phpscript --cover=file --coverfile=cover/script.cov run path/to/script.php
```

## The artifact

`cpu-<label>.pprof` and `mem-<label>.pprof`, already ignored by `*.pprof`.

`cover/<label>.cov`, already ignored by `/cover/*` and `*.cov`.

`bench-http-<side>.txt` when a load generator ran, already ignored by `bench*.txt`.

`bench-manifest-<side>.txt` alongside, as the contract requires. For a server run it also records `TESTSERVER_WORKERS` and `TESTSERVER_LIMIT`, because both change what was measured.

## Reading it

The top frames:

```sh
go tool pprof -top -nodecount=40 cpu-after.pprof
```

Before against after, which is the form that produces a finding rather than two lists to squint at:

```sh
go tool pprof -top -nodecount=40 -diff_base=cpu-before.pprof cpu-after.pprof
```

For allocation counts rather than bytes:

```sh
go tool pprof -sample_index=alloc_objects -top -nodecount=40 mem-after.pprof
```

What a one-shot run includes that a benchmark loop does not: process start, the parse, and the whole of `stdlib.Register` - 322 functions, 41 classes and 152 constants. On a short script those frames are most of the profile. That is the honest cost of a single invocation and it is the wrong profile for a hot loop.

A single run is one sample. It has no interval, so it ranks frames and does not compare two commits on its own. Where a delta is the claim, the delta comes from a benchmark.

## What this cannot answer

| Question                                          | Document                                               |
|---------------------------------------------------|--------------------------------------------------------|
| A repeated measurement with a confidence interval | [collect-go-benchmarks.md](collect-go-benchmarks.md)   |
| What the fixture suite costs                      | [collect-phpscript-test.md](collect-phpscript-test.md) |
| Whether the tree still passes                     | [collect-go-tests.md](collect-go-tests.md)             |
| How the three layers fit together                 | [profile.md](profile.md)                               |
