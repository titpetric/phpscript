| Example | What it does |
|---|---|
| `phpscript test ./...` | Runs every `.phpt` below the current directory, one table per folder. A bare path is not recursive. |
| `phpscript test --matrix ./tests/...` | Runs each fixture on flatstack, the interpreter and `php`, and reports a column per runtime. |
| `phpscript test -v ./...` | Adds the failure of each runtime below its fixture. |
| `phpscript test --cover=file ./tests/...` | Writes `phpscript.cov` and prints per-file coverage in the format `go tool cover -func` prints. |
| `phpscript test --coverfile=cover/php.cov --split ./tests/...` | Writes the merged profile there and each fixture's own beside it as `<fixture>.cov`. |
| `phpscript test -p 8 ./...` | Runs eight fixtures at a time. |
| `phpscript test -c 100 --profile ./tests/arrays/...` | Runs each fixture 100 times and reports allocations and bytes per run. |
| `phpscript test -o docs/test-fixtures.md ./tests/...` | Writes the markdown report while the table still goes to the terminal. |
