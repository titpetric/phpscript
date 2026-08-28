# The Generator class

(PHP 5 >= 5.5.0, PHP 7, PHP 8)

## Introduction

Objects returned from generators, implementing [Iterator](Iterator.md).

A generator is a function containing `yield`. Calling it returns a Generator rather than running the body; the body advances as the sequence is read. Generators cannot be constructed with `new`.

## Class synopsis

```php
final class Generator implements Iterator {

/* Methods */

public function current(): mixed

public function getReturn(): mixed

public function key(): mixed

public function next(): void

public function rewind(): void

public function send(mixed $value): mixed

public function throw(Throwable $exception): mixed

public function valid(): bool

public function __wakeup(): void

}
```

## Methods

| Method                                          | Description                           |
|-------------------------------------------------|---------------------------------------|
| `Generator::current(): mixed`                   | Get the yielded value                 |
| `Generator::getReturn(): mixed`                 | Get the return value of a generator   |
| `Generator::key(): mixed`                       | Get the yielded key                   |
| `Generator::next(): void`                       | Resume execution of the generator     |
| `Generator::rewind(): void`                     | Rewind the iterator                   |
| `Generator::send(mixed $value): mixed`          | Send a value to the generator         |
| `Generator::throw(Throwable $exception): mixed` | Throw an exception into the generator |
| `Generator::valid(): bool`                      | Check if the iterator has been closed |
| `Generator::__wakeup(): void`                   | Serialize callback                    |

## Status

- phpscript will not implement this class. `yield`, generators and Fibers are a decision rather than a gap: there is no coroutine model, and none is planned. See [Design decisions](../../../design.md).
- The name is not declared and `yield` is not a keyword here.
- Build the sequence into an array and return it. Where the point was to avoid holding the whole sequence, read it in batches with an explicit offset.
