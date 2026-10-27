# The Throwable interface

(PHP 7, PHP 8)

## Introduction

Base interface for any object a `throw` statement can throw, including `Error` and `Exception`.

A PHP class implements it only by extending `Exception` or `Error`.

## Interface synopsis

```php
interface Throwable extends Stringable {

/* Methods */

public function getMessage(): string

public function getCode(): int

public function getFile(): string

public function getLine(): int

public function getTrace(): array

public function getTraceAsString(): string

public function getPrevious(): ?Throwable

public function __toString(): string

/* Inherited methods */

public function Stringable::__toString(): string

}
```

## Methods

| Method                                  | Description                                       |
|-----------------------------------------|---------------------------------------------------|
| `Throwable::getMessage(): string`       | Gets the message                                  |
| `Throwable::getCode(): int`             | Gets the exception code                           |
| `Throwable::getFile(): string`          | Gets the file holding the object's construction   |
| `Throwable::getLine(): int`             | Gets the line holding the object's construction   |
| `Throwable::getTrace(): array`          | Gets the stack trace                              |
| `Throwable::getTraceAsString(): string` | Gets the stack trace as a string                  |
| `Throwable::getPrevious(): ?Throwable`  | Returns the previous Throwable                    |
| `Throwable::__toString(): string`       | Gets a string representation of the thrown object |

## Status

- phpscript supports the name in a catch clause and nowhere else. `catch (Throwable $e)` takes any failure, which is the common use of the name.
- The name is not declared as an interface, and `instanceof Throwable` is false, even inside a `catch (Throwable $e)` block that just caught the value.
- The interface uses `extends`. `extends Stringable` is where PHP's Throwable declares its `__toString()`; [Stringable](Stringable.md) is not declared here, and phpscript supports no magic method beyond `__construct` and `__invoke`, and the inherited method has no path to arrive through.
- There is no exception hierarchy: a catch clause filters on the class name recorded on the value, and walks no parent chain. See [Design decisions](../../../design.md).
