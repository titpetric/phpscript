# The UnitEnum interface

(PHP 8 >= 8.1.0)

## Introduction

Interface the engine applies to every enumeration.

PHP rejects `implements UnitEnum` on a user-defined class, and an enum cannot override `cases()`: the engine supplies the implementation, and the name exists so a parameter can name it as a type.

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

- phpscript has no `UnitEnum`. The name is not declared.
- Enumerations are unavailable: `enum` is not a keyword here, and the engine has no enum to apply the interface to.
- Declare class constants and check a value against them. The runtime checks a class constant where a script reads it, and resolves `Class::NAME`.
