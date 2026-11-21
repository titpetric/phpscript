# Functions

| PHP language-reference feature | Status                | Notes                                                                                                                                                    |
|--------------------------------|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| User-defined functions         | Compatibility         | phpscript supports named functions, parameters, defaults, calls, and returns.                                                                            |
| Anonymous functions            | Compatibility         | Closures, `use (...)` captures, `static function`, and calling a callable value.                                                                         |
| Arrow functions                | Incompatible syntax   | `fn` introduces a normal block-bodied function/closure; PHP's `fn (...) => ...` form is unavailable.                                                     |
| First-class callable syntax    | Compatibility         | `callable(...)` takes the callable a call site names, as a Closure. `new C(...)` is a parse error.                                                       |
| Arguments by value             | Partial compatibility | A call binds names in a new local scope, but mutable arrays do not have PHP copy-on-write behavior.                                                      |
| References and variadics       | Partial compatibility | Variadic parameters and first-class callables work. Named arguments and argument unpacking `f(...$args)` are unavailable; `&` parses but binds by value. |
| Type declarations              | Parsed, not enforced  | The parser accepts parameter and return types and discards them; nothing checks or coerces values.                                                       |

## Defining functions

The portable spelling is `function`:

```php
function greet($name = "world") {
    return "Hello " . $name;
}
```

phpscript also accepts `func` and `fn` as aliases. All three require a brace-delimited body. See [Function keyword aliases](../extensions/README.md#function-keyword-aliases) for the incompatibility with PHP arrow functions.

## Anonymous functions

```php
$callback = function ($value) {
    return $value * 2;
};
```

A `use (...)` list captures by value: creating the closure value snapshots the named variables. A later write to the enclosing variable is not visible inside the closure.

```php
$greeting = "Hello";
$greet = function ($name) use ($greeting) {
    return $greeting . ", " . $name;
};
$greeting = "Goodbye";
echo $greet("world");                     // Hello, world
```

`&$name` parses but binds by value like the plain form: the runtime has no reference cells. A closure cannot write back into the frame it came from. The same is true of a `&$x` parameter. See [Value semantics](../types/value-semantics.md).

A closure declared inside a method captures `$this`. `static function () {}` declares one that does not.

The `use (...)` list defines what comes along. An invocation gets a frame holding those values, the parameters, `__FILE__`, `__DIR__`, and the bound `$this`: a variable of the enclosing frame that the list did not name is unset inside the body, neither shadowed nor inherited. A nested closure names again whatever it needs. `get_defined_vars()` inside a closure reads that frame back, as [tests/fixtures/functions/closure_capture_scope.phpt](../../../tests/fixtures/functions/closure_capture_scope.phpt) states for both engines. Creating the closure value takes the capture. A later write to a captured variable is not visible. The invocation binds a parameter sharing a name with a capture last. The parameter wins. Php rejects that program outright, as "Cannot use lexical variable $x as a parameter name".

`static $x` in a closure body belongs to the closure value. Two closures built by one factory count independently. A call on another runtime gets a separate bag. The bytecode engine does not compile that form, leaving the program on the interpreter.

## Calling a callable value

A call site invokes a callable held in a value directly, whatever holds it:

```php
$fn($argument);
$handlers["render"]($argument);
$this->callback($argument);
```

Every PHP callable spelling resolves: a closure, `"function_name"`, `"Class::method"`, `array($object, "method")`, `array("Class", "method")`, and an object with `__invoke`. `Closure::fromCallable()` turns any of them into a closure. `Closure::bind()` accepts a null `$newThis` and returns the closure unchanged, since phpscript enforces no property visibility that a scope change could affect. Rebinding `$this` raises an error. It never becomes a silent no-op.

## First-class callable syntax

`callable(...)` is the call site with the call not made: it answers the callable the site names. The ellipsis is literal, and each of the four call forms takes it.

```php
$fn = greet(...);                 // a declared function, or a registered binding
$shout = Greeter::shout(...);     // a static method, self:: and static:: included
$method = $obj->method(...);      // a method bound to the receiver it was read off
$again = $closure(...);           // a value that is callable already
```

The value is php's `Closure`: `get_class()` answers `Closure`, `is_callable()` returns true, and any parameter an API declares as callable takes it. Resolution matches the call's. A name reaches a declared function, a registered binding or a host static registered with the whole `Class::method` spelling. Taking the callable resolves the name a variable holds. The call does not. Taking the callable reports a name nothing answers for, with the message the call reports: `nosuch(...)` raises "call to undefined function nosuch()".

`(...)` over an already-callable value answers that value, without a wrapper, and `$fn(...) === $fn` for a Closure the syntax produced, as in php. A closure literal is the only value the two engines disagree about. See [Closure identity](../../flatstack.md#closure-identity).

`new C(...)` is a parse error, as in php: a construction names no callable to take.

Both engines compile the syntax, with nothing about it sending a program to the interpreter. [tests/fixtures/functions/first_class_callable.phpt](../../../tests/fixtures/functions/first_class_callable.phpt) holds php's own output for every spelling.

`$obj->method` without parentheses answers the same bound callable in a shorter spelling that php does not have. See [Known divergences](../../README.md#known-divergences-from-php).

## Calling functions

Unqualified calls in a namespace first resolve in that namespace, then fall back to a global function. Wherever an API takes a callable, every spelling above fills it, `usort()`, `array_map()` or a Go binding's callback alike. See [Callbacks](../extensions/bindings.md#callbacks) for the declaration a binding makes to receive one.
