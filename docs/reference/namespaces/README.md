# Namespaces

| PHP language-reference feature     | Status                | Notes                                                                             |
|------------------------------------|-----------------------|-----------------------------------------------------------------------------------|
| Defining namespaces                | Partial compatibility | Only one semicolon-delimited namespace at the start of a file is supported.       |
| Sub-namespaces                     | Compatibility         | Qualified names with `\` are supported.                                           |
| Multiple namespaces per file       | Not implemented       | Braced namespaces and repeated declarations are unavailable.                      |
| Relative and fully-qualified names | Partial compatibility | Current-namespace and leading-`\` names resolve; `namespace\Name` is unavailable. |
| Aliasing and importing             | Not implemented       | The `use` import statement is unavailable.                                        |
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

## Autoloading

There is no implicit autoloader. Register one with `spl_autoload_register()`.
Calling it without a callback installs phpscript's default loader, which
lowercases the class name and resolves it as a `.php` file on the include path.
