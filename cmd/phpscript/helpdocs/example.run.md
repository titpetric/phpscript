| Example | What it does |
|---|---|
| `phpscript run app.php` | Runs the script. `run` is the default command, so `phpscript app.php` is the same thing. |
| `phpscript -w demos/example run public/index.php` | Runs from another tree without changing directory first. |
| `phpscript --include vendor/autoload.php run app.php` | Pulls composer in ahead of the script, so its classes resolve. |
| `phpscript --cover --coverfile=cover/app.cov run app.php` | Writes a statement profile of the run. `go tool cover -html=cover/app.cov` renders it. |
| `phpscript --cpuprofile=cpu.pprof run app.php` | Writes a pprof CPU profile of the whole run. |
