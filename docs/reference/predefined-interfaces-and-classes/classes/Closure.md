# The Closure class

(PHP 5 >= 5.3.0, PHP 7, PHP 8)

## Introduction

Class used to represent anonymous functions.

Anonymous functions evaluate to objects of this type. The class exists so a closure can be rebound to another object or scope after it is created; it is not constructible with `new`.

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

- phpscript implements the value and no class. `function () { ... }` produces a callable a script can store, pass, and send to `usort()` or `call_user_func()`.
- php 8.1's [first-class callable syntax](../../functions/README.md#first-class-callable-syntax) produces the same value: `strlen(...)`, `Greeter::shout(...)`, `$obj->method(...)` and `$fn(...)` each answer the callable without calling it, and `get_class()` answers `Closure` for it. `Closure::fromCallable()` takes the same spellings as an argument.
- The name is not declared, so `instanceof Closure` is false and none of the methods above exists. Rebinding `$this` is what they are for, and there is nothing here to rebind to.
- A closure captures with `use ($x)`, and that list is the whole of what an invocation carries, together with the parameters and a bound `$this`. See [Anonymous functions](../../functions/README.md#anonymous-functions).
- `use (&$x)` is a documented won't-implement, along with the rest of `&` outside `foreach`. It binds by value, and the bytecode engine refuses the spelling, so a program containing it runs on the interpreter.
