# goserver

`testdata/phpscript-http-bridge/server.php` written in Go, route for route and one load generator drives both and the difference reads off directly.

The work is the same: the same patterns on the same `net/http` router, the same JSON objects encoded with the same encoder, the same reply bodies. What it does not have is a VM, and that absence is the whole of what the comparison measures.

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

The file carries `//go:build ignore` and `go build ./...` never reaches it and it runs as a file. The compose file mounts a binary built from it and the floor is the build under test and no image carries a stale one:

```sh
CGO_ENABLED=0 go build -o bin/testserver-go testdata/goserver/main.go
```

## The floor, not a target

`TESTSERVER_WORKERS` bounds how many requests the server answers at once, the way the bridge's worker count does. Go needs no such bound, `net/http` answering each request on a goroutine and the scheduler taking it from there and leaving it unset bounds nothing, which is the Go server anyone would actually write. Set, it becomes a semaphore of that depth and a side by side against a bounded phpscript server compares the same arrangement on both sides.

This column is the floor and no number to reach. [../../docs/agents/performance.md](../../docs/agents/performance.md) owns the comparison.
