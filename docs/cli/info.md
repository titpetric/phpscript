# `phpscript info [path...]`

Print the runtime environment, the way `phpinfo()` does in a terminal.

It accepts the [global flags](README.md#global-flags) and nothing else: `-f`,
`-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and
`--coverfile`.

```bash
phpscript info
phpscript info -v
phpscript info ./src
```

With no path, the command prints the built-in runtime. `-v` / `--verbose`
adds bound classes (with constructors and methods) and internal functions.
A path argument uses the same file expansion as `list` and prints markdown
docs for classes and functions found in that tree.
