# Composer

phpscript resolves composer projects natively. A project with a `composer.json`
and an installed `vendor/` can name its dependencies' classes directly, with no
copies of the dependency inside the project:

```php
<?php

include "vendor/autoload.php";

$tpl = new MiniTPL\Template("templates/");
```

The same file runs under stock PHP unchanged. That is the point: dependencies
are declared once, in `composer.json`, and both runtimes load them the same way.

## Why the autoloader is reimplemented

composer's generated `vendor/autoload.php` bootstraps a ~600 line `ClassLoader`
through static properties, closure binding and generated class names. phpscript
does not implement enough of that to run it.

The data behind it is plain JSON, though. `composer.json` and
`vendor/composer/installed.json` describe every PSR-4 and PSR-0 prefix and every
`autoload.files` entry, so phpscript reads that metadata directly and installs
its own class loader. Including `vendor/autoload.php` runs the Go
implementation; the generated file is never parsed.

## When the autoloader is installed

Only when a script includes it. `include "vendor/autoload.php"` is what turns
autoloading on, exactly as in PHP — a script that never includes it sees no
vendor classes, and one that includes a missing `vendor/autoload.php` fails the
way PHP would. In practice that means one include in `bootstrap.php`, which
every endpoint already includes.

Discovery walks up from the script's directory to the nearest `composer.json`,
so entrypoints in subdirectories resolve the same project.

## What is supported

| autoload type           | supported | notes                                           |
|-------------------------|-----------|-------------------------------------------------|
| `psr-4`                 | yes       | longest matching prefix wins                    |
| `psr-0`                 | yes       | underscores in the class name expand to folders |
| `files`                 | yes       | included when the autoloader is included        |
| `classmap`              | no        | composer generates it as PHP, not as JSON       |
| `exclude-from-classmap` | n/a       | follows from `classmap`                         |

Packages are found through `vendor/composer/installed.json`. A vendor tree
without it — assembled by hand, or partially generated — falls back to reading
each `vendor/<vendor>/<package>/composer.json`.

`vendor/` is skipped when scanning for `@route` and `@startup` annotations and
by `phpscript list`: a dependency does not get to publish endpoints into the
application.

## Local development against an unreleased package

composer path repositories work as usual. To test a dependency from a sibling
checkout rather than from packagist:

```json
{
    "repositories": [
        {
            "type": "path",
            "url": "../../../minitpl",
            "options": { "symlink": false }
        }
    ],
    "require": {
        "titpetric/minitpl": "@dev"
    }
}
```

`symlink: false` mirrors the sources into `vendor/` instead of symlinking them,
which is what the demos need: each demo is mounted into its container on its
own, and a symlink pointing outside that directory would dangle.

This is how [tests/fixtures](../tests/fixtures/composer.json) and
[demos/example](../demos/example/composer.json) consume
[titpetric/minitpl](https://github.com/titpetric/minitpl). Run
`composer install` in either directory (or `atkins composer:install` for both)
before running their tests.

## References

- [Templating](./use-cases/templating.md)
- [Building an application](./use-cases/application.md)
