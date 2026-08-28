# The UnitEnum interface

(PHP 8 >= 8.1.0)

## Introduction

Interface the engine applies to every enumeration.

It cannot be implemented by a user-defined class and its method cannot be overridden: the engine supplies the implementation, and the name exists so a parameter can be typed against it.

## Interface synopsis

```php
interface UnitEnum {

/* Methods */

public static function cases(): array

}
```

## Methods

| Method                     | Description                          |
|----------------------------|--------------------------------------|
| `UnitEnum::cases(): array` | Generates a list of cases on an enum |

## Status

- phpscript does not implement this interface. The name is not declared.
- Enumerations are unavailable: `enum` is not a keyword here, so there is nothing for the engine to apply the interface to.
- Declare class constants and validate against them. A class constant is checked at the point it is read, and `Class::NAME` is supported.
