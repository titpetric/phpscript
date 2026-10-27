# The BackedEnum interface

(PHP 8 >= 8.1.0)

## Introduction

Interface the engine applies to every backed enumeration, that is, one whose cases carry a scalar value.

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

- phpscript has no `BackedEnum`. The name is not declared.
- The interface uses `extends`. The parser accepts `extends UnitEnum` and records it for `instanceof`; the clause confers nothing, because the implementing class would have to declare `cases()` itself.
- Enumerations are unavailable, and nothing would carry the interface. See [UnitEnum](UnitEnum.md) for what to write instead.
