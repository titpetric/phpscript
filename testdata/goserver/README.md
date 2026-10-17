# goserver

`testdata/phpscript-http-bridge/server.php` written in Go, route for route, so the two can be put through the same load generator and the difference read off.

It is the same work: the same patterns on the same `net/http` router, the same JSON objects encoded with the same encoder, the same reply bodies. What it does not have is a VM, which is the whole of what the comparison measures.

## Running it

```sh
docker compose up -d --wait
curl http://127.0.0.1:8094/users/42
docker compose down
```

Or without docker, from the repository root:

```sh
go run testdata/goserver/main.go
TESTSERVER_ADDR=127.0.0.1:0 TESTSERVER_WORKERS=5 go run testdata/goserver/main.go
```

It is `//go:build ignore`, so `go build ./...` does not reach it and it runs as the file. The compose file mounts a binary built from it, so the floor is the build under test and no image carries a stale one:

```sh
CGO_ENABLED=0 go build -o bin/testserver-go testdata/goserver/main.go
```

## The floor, not a target

`TESTSERVER_WORKERS` bounds how many requests are answered at once, the way the bridge's worker count does. Go does not need it, `net/http` answering each request on a goroutine and the scheduler sorting it out, so unset is unbounded and is the Go server anyone would actually write. Set, it is a semaphore of that depth, so a side by side against a bounded phpscript server compares the same arrangement on both sides.

This column is read as the floor and not as a number to reach. [../../docs/agents/performance.md](../../docs/agents/performance.md) owns the comparison.
