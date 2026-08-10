# Functions

| PHP language-reference feature | Status                | Notes                                                                                                |
|--------------------------------|-----------------------|------------------------------------------------------------------------------------------------------|
| User-defined functions         | Compatibility         | Named functions, parameters, defaults, calls, and returns are supported.                             |
| Anonymous functions            | Partial compatibility | Closures work, but `use (...)` captures are parsed and ignored.                                      |
| Arrow functions                | Incompatible syntax   | `fn` introduces a normal block-bodied function/closure; PHP's `fn (...) => ...` form is unavailable. |
| Arguments by value             | Partial compatibility | Names are bound in a new local scope, but mutable arrays do not have PHP copy-on-write behavior.     |
| References and variadics       | Not implemented       | `&`, `...`, named arguments, and argument unpacking are unavailable.                                 |
| Type declarations              | Not implemented       | Parameter and return types are unavailable.                                                          |

## Defining functions

The portable spelling is `function`:

```php
function greet($name = "world") {
    return "Hello " . $name;
}
```

phpscript also accepts `func` and `fn` as aliases. All three require a
brace-delimited body. See [Function keyword aliases](../extensions/README.md#function-keyword-aliases)
for the incompatibility with PHP arrow functions.

## Anonymous functions

```php
$callback = function ($value) {
    return $value * 2;
};
```

Closure capture is not implemented. Although `use ($name)` is consumed by the
parser for source compatibility, `$name` is not copied into the closure scope.

## Calling functions

Unqualified calls in a namespace first resolve in that namespace and then fall
back to a global function. Supported callbacks include closures, function names,
and bound methods where an API accepts a callable.
