# The Fiber class

(PHP 8 >= 8.1.0)

## Introduction

Full-stack, interruptible function that may be suspended from anywhere in the call stack.

Execution inside a fiber pauses until something resumes it, which is what lets one call stack wait without blocking the one that started it.

## Class synopsis

```php
final class Fiber {

/* Methods */

public function __construct(callable $callback)

public function start(mixed ...$args): mixed

public function resume(mixed $value = null): mixed

public function throw(Throwable $exception): mixed

public function getReturn(): mixed

public function isStarted(): bool

public function isSuspended(): bool

public function isRunning(): bool

public function isTerminated(): bool

public static function suspend(mixed $value = null): mixed

public static function getCurrent(): ?Fiber

}
```

## Methods

| Method                                       | Description                                      |
|----------------------------------------------|--------------------------------------------------|
| `Fiber::__construct(callable $callback)`     | Creates a new Fiber instance                     |
| `Fiber::start(mixed ...$args): mixed`        | Start execution of the fiber                     |
| `Fiber::resume(mixed $value = null): mixed`  | Resumes execution of the fiber with a value      |
| `Fiber::throw(Throwable $exception): mixed`  | Resumes execution of the fiber with an exception |
| `Fiber::getReturn(): mixed`                  | Gets the value returned by the fiber             |
| `Fiber::isStarted(): bool`                   | Determines if the fiber has started              |
| `Fiber::isSuspended(): bool`                 | Determines if the fiber is suspended             |
| `Fiber::isRunning(): bool`                   | Determines if the fiber is running               |
| `Fiber::isTerminated(): bool`                | Determines if the fiber has terminated           |
| `Fiber::suspend(mixed $value = null): mixed` | Suspends execution of the current fiber          |
| `Fiber::getCurrent(): ?Fiber`                | Gets the currently executing fiber               |

## Status

- phpscript will not implement this class, for the reason [Generator](Generator.md) is not implemented: there is no coroutine model. See [Design decisions](../../../design.md).
- The name is not declared.
- Concurrency belongs to the embedding Go application. A script runs to completion on one goroutine, and the host decides how many run at once.
