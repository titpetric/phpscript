# Functions

| PHP language-reference feature | Status                | Notes                                                                                                |
|--------------------------------|-----------------------|------------------------------------------------------------------------------------------------------|
| User-defined functions         | Compatibility         | Named functions, parameters, defaults, calls, and returns are supported.                             |
| Anonymous functions            | Compatibility         | Closures, `use (...)` captures, `static function`, and calling a callable value.                     |
| Arrow functions                | Incompatible syntax   | `fn` introduces a normal block-bodied function/closure; PHP's `fn (...) => ...` form is unavailable. |
| Arguments by value             | Partial compatibility | Names are bound in a new local scope, but mutable arrays do not have PHP copy-on-write behavior.     |
| References and variadics       | Not implemented       | `...`, named arguments and unpacking are unavailable; `&` parses but binds by value.                 |
| Type declarations              | Parsed, not enforced  | Parameter and return types are accepted and discarded; values are never checked or coerced.          |

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

A `use (...)` list captures by value: the named variables are snapshotted when
the closure value is created, so a later write to the enclosing variable is not
visible inside the closure.

```php
$greeting = "Hello";
$greet = function ($name) use ($greeting) {
    return $greeting . ", " . $name;
};
$greeting = "Goodbye";
echo $greet("world");                     // Hello, world
```

`&$name` is accepted but binds by value like the plain form: the runtime has no
reference cells, so a closure cannot write back into the frame it came from. The
same is true of a `&$x` parameter; see [Value semantics](../types/value-semantics.md).

A closure declared inside a method captures `$this`; `static function () {}`
declares one that does not.

## Calling a callable value

A callable held in a value is invoked directly, whatever holds it:

```php
$fn($argument);
$handlers["render"]($argument);
$this->callback($argument);
```

Every PHP callable spelling resolves: a closure, `"function_name"`,
`"Class::method"`, `array($object, "method")`, `array("Class", "method")`, and an
object with `__invoke`. `Closure::fromCallable()` turns any of them into a
closure; `Closure::bind()` accepts a null `$newThis` and returns the closure
unchanged, since phpscript enforces no property visibility for a scope change to
affect. Rebinding `$this` is reported as an error rather than silently ignored.

## Calling functions

Unqualified calls in a namespace first resolve in that namespace and then fall
back to a global function. Supported callbacks include closures, function names,
and bound methods where an API accepts a callable.
