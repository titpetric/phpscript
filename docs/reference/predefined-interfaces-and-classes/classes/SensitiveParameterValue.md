# The SensitiveParameterValue class

(PHP 8 >= 8.2.0)

## Introduction

Wrapper that protects a sensitive value against accidental exposure.

A parameter carrying the `SensitiveParameter` attribute is replaced by one of these in a stack trace, so the value does not appear in the trace a failure prints.

## Class synopsis

```php
final class SensitiveParameterValue {

/* Properties */

private readonly mixed $value;

/* Methods */

public function __construct(mixed $value)

public function __debugInfo(): array

public function getValue(): mixed

}
```

## Methods

| Method                                               | Description                                              |
|------------------------------------------------------|----------------------------------------------------------|
| `SensitiveParameterValue::__construct(mixed $value)` | Constructs a new SensitiveParameterValue object          |
| `SensitiveParameterValue::__debugInfo(): array`      | Protects the sensitive value against accidental exposure |
| `SensitiveParameterValue::getValue(): mixed`         | Returns the sensitive value                              |

## Status

- phpscript does not implement this class. The name is not declared.
- It has nothing to attach to: attributes are unavailable, so the `SensitiveParameter` attribute that produces one cannot be written.
- `__debugInfo()` is a magic method, which is unavailable as well, and `readonly` is parsed and not enforced.
- Keep the secret out of the argument list: read it from configuration inside the function that needs it.
