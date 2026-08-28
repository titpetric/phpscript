# Namespaces

| PHP language-reference feature     | Status                | Notes                                                                             |
|------------------------------------|-----------------------|-----------------------------------------------------------------------------------|
| Defining namespaces                | Partial compatibility | Only one semicolon-delimited namespace at the start of a file is supported.       |
| Sub-namespaces                     | Compatibility         | Qualified names with `\` are supported.                                           |
| Multiple namespaces per file       | Not implemented       | Braced namespaces and repeated declarations are unavailable.                      |
| Relative and fully-qualified names | Partial compatibility | Current-namespace and leading-`\` names resolve; `namespace\Name` is unavailable. |
| Aliasing and importing             | Compatibility         | `use A\B\C;` and `use A\B\C as D;` alias a name for the rest of the file.         |
| `__NAMESPACE__`                    | Compatibility         | Resolves to the current namespace name.                                           |
| Function fallback                  | Compatibility         | Unqualified calls fall back from the current namespace to global functions.       |

## Defining a namespace

The declaration must be the first PHP statement. A namespaced file may contain
only class and function declarations.

```php
<?php
namespace App\Model;

class User
{
}
```

## Name resolution

Class names are relative to the current namespace unless they begin with `\`.
Unqualified function calls try the current namespace first and global scope
second.

## Importing

`use` aliases a fully-qualified name to its last segment, or to an explicit
alias. The alias applies to every name resolved in the rest of the file,
including the head of a longer name.

```php
<?php
namespace App;

use Composer\Autoload\ClassLoader;
use Acme\Greeting\Formal as Greeter;

class Bootstrap
{
    function loader() {
        return new ClassLoader();          // Composer\Autoload\ClassLoader
    }

    function greeter() {
        return new Greeter();              // Acme\Greeting\Formal
    }
}
```

`use function` and `use const` are accepted and alias the same way.

## Autoloading

An `autoload/` directory at the application root loads classes on first
reference, with nothing registered and nothing included. The namespace is the
directory path below it and the class is the file, case for case, so
`Acme\Thing` is `autoload/Acme/Thing.php`; the namespace is optional, so a class
declaring none is `autoload/Bare.php`. This is a phpscript convention with no
equivalent in php, configured by the
[`runner.autoload`](../../configuration.md#autoload-folder) key and disabled by
not having the directory. Files there are expected to declare and nothing else.

The folder is consulted last. Everything below still comes first, so a script
that registers its own loader keeps the resolution order it wrote.

Register a loader with `spl_autoload_register()`,
which accepts every PHP callable spelling: a closure, a function name,
`"Class::method"`, or `array($object, "method")`. `spl_autoload_unregister()`
removes it again, matching by what the callable names rather than by identity.
Calling `spl_autoload_register()` without a callback installs phpscript's
default loader, which lowercases the class name and resolves it as a `.php` file
on the include path.

composer's generated autoloader is interpreted as-is: `require "vendor/autoload.php"`
runs `vendor/composer/ClassLoader.php` through the same interpreter as any other
PHP, registers its `loadClass` method, and resolves PSR-4 and classmap entries
from `composer.json`. Nothing about composer is special-cased in the runtime;
see [demos/dbadmin](../../../demos/dbadmin) for a working application.
