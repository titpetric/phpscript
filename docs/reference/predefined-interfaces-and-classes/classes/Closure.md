# The Closure class

(PHP 5 >= 5.3.0, PHP 7, PHP 8)

## Introduction

Class used to represent anonymous functions.

An anonymous function produces an object of this type. The class exists so a closure can rebind to another object or scope after PHP creates it. `new` cannot construct one.

## Class synopsis

```php
final class Closure {

/* Methods */

private function __construct()

public static function bind(Closure $closure, ?object $newThis, object|string|null $newScope = "static"): ?Closure

public function bindTo(?object $newThis, object|string|null $newScope = "static"): ?Closure

public function call(object $newThis, mixed ...$args): mixed

public static function fromCallable(callable $callback): Closure

public static function getCurrent(): Closure

}
```

## Methods

| Method                                                    | Description                              |
|-----------------------------------------------------------|------------------------------------------|
| `Closure::__construct()`                                  | Constructor that disallows instantiation |
| `Closure::bind(Closure $closure, ?object $newThis, object | string                                   |
| `Closure::bindTo(?object $newThis, object                 | string                                   |
| `Closure::call(object $newThis, mixed ...$args): mixed`   | Binds and calls the closure              |
| `Closure::fromCallable(callable $callback): Closure`      | Converts a callable into a closure       |
| `Closure::getCurrent(): Closure`                          | Returns the executing closure            |

## Status

- phpscript implements the value. It declares no class. `function () { ... }` produces a callable a script can store, pass, and send to `usort()` or `call_user_func()`.
- php 8.1's [first-class callable syntax](../../functions/README.md#first-class-callable-syntax) produces the same value: `strlen(...)`, `Greeter::shout(...)`, `$obj->method(...)`, `$fn(...)` each answer the callable without calling it. `get_class()` answers `Closure` for the callable. `Closure::fromCallable()` takes the same spellings as an argument.
- The name is not declared. `instanceof Closure` is false. No method above exists. Rebinding `$this` motivates them. Rebinding needs a `$this`, which does not exist here.
- A closure captures with `use ($x)`. An invocation receives that list, the parameters, and any bound `$this`. See [Anonymous functions](../../functions/README.md#anonymous-functions).
- `use (&$x)` is a documented won't-build, along with the rest of `&` outside `foreach`. It binds by value. The bytecode engine refuses the spelling. A program containing it runs on the interpreter.
