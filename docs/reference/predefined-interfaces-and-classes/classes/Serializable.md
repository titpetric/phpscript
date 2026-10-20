# The Serializable interface

(PHP 5 >= 5.1.0, PHP 7, PHP 8)

## Introduction

Interface for customized serializing.

A class implementing it drops `__sleep()` and `__wakeup()`: PHP calls `serialize()` when an instance needs serializing, and `unserialize()` replaces the constructor when PHP reads one back. As of PHP 8.1.0, a class that implements it without also declaring `__serialize()` and `__unserialize()` raises a deprecation notice.

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

- phpscript has no `Serializable`. The name is not declared.
- PHP's own serialization format has no implementation here, so there is no call site that would dispatch through the interface.
- PHP 8.1.0 deprecated the interface in favour of `__serialize()` and `__unserialize()`, which are magic methods and also unavailable. Encode with `json_encode()` and decode with `json_decode()` instead.
