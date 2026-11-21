# The BackedEnum interface

(PHP 8 >= 8.1.0)

## Introduction

Interface the engine applies to backed enumerations: one whose cases hold a scalar value.

It adds the two mappings from a scalar to a case, and inherits `cases()` from [UnitEnum](UnitEnum.md).

## Interface synopsis

```php
interface BackedEnum extends UnitEnum {

/* Methods */

public static function from(int|string $value): static

public static function tryFrom(int|string $value): ?static

/* Inherited methods */

public static function UnitEnum::cases(): array

}
```

## Methods

| Method                     | Description                                     |
|----------------------------|-------------------------------------------------|
| `BackedEnum::from(int      | string $value): static`                         |
| `BackedEnum::tryFrom(int   | string $value): ?static`                        |
| `UnitEnum::cases(): array` | Generates a list of cases on an enum, inherited |

## Status

- phpscript declares no `BackedEnum` anywhere.
- The interface uses `extends`. The parser accepts `extends UnitEnum` and records it for `instanceof`, but the clause confers no member: the implementing class would have to declare `cases()` itself.
- Enumerations are unavailable and nothing declares the interface. [UnitEnum](UnitEnum.md) names the replacement.
