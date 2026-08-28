# The Serializable interface

(PHP 5 >= 5.1.0, PHP 7, PHP 8)

## Introduction

Interface for customized serializing.

A class implementing it no longer supports `__sleep()` and `__wakeup()`: `serialize()` is called when an instance needs serializing, and `unserialize()` replaces the constructor when one is read back. As of PHP 8.1.0, a class that implements it without also declaring `__serialize()` and `__unserialize()` raises a deprecation notice.

## Interface synopsis

```php
interface Serializable {

/* Methods */

public function serialize(): ?string

public function unserialize(string $data): void

}
```

## Methods

| Method                                          | Description                     |
|-------------------------------------------------|---------------------------------|
| `Serializable::serialize(): ?string`            | String representation of object |
| `Serializable::unserialize(string $data): void` | Constructs the object           |

## Status

- phpscript does not implement this interface. The name is not declared.
- PHP's own serialization format has no implementation here, so there is no call site that would dispatch through the interface.
- The interface is deprecated as of PHP 8.1.0 in favour of `__serialize()` and `__unserialize()`, which are magic methods and therefore also unavailable. Encode with `json_encode()` and decode with `json_decode()` instead.
