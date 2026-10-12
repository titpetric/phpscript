# `phpscript run <file.php>`

Parse and execute a PHP script in CLI mode.

It accepts the [global flags](README.md#global-flags) and nothing else: `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`.

```bash
phpscript run tests/fixtures/test-hello-world.php
phpscript tests/fixtures/test-hello-world.php
```

Use this command for normal script execution and shebang scripts:

```php
#!/usr/bin/env phpscript
<?php
echo "Hello world\n";
```

Everything after the script is the script's own, and arrives in `$argv` and `$argc` the way the php CLI passes it. Flags for phpscript go before the script: in `phpscript --stdlib secure app.php -v src`, `$argv` is `["app.php", "-v", "src"]`.
