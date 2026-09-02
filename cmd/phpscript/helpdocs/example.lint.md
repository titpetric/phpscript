| Example | What it does |
|---|---|
| `phpscript lint ./...` | Reports undefined names, unreachable code and unused variables, one table per folder. |
| `phpscript lint --flatstack ./...` | Adds why a file cannot compile to bytecode. |
| `phpscript lint --include vendor/autoload.php ./...` | Registers what the autoloader declares first, so composer classes are not reported undefined. |
| `phpscript lint -o LINT.md ./...` | Writes the findings as markdown. |
