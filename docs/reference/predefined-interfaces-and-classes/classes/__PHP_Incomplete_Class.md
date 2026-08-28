# The __PHP_Incomplete_Class class

(PHP 4 >= 4.0.1, PHP 5, PHP 7, PHP 8)

## Introduction

Object produced by `unserialize()` for a class that is not defined, or that is not listed in the `allowed_classes` option.

The object carries a `__PHP_Incomplete_Class_Name` property naming the class that could not be built. Since PHP 7.2.0, `is_object()` answers true for one.

## Class synopsis

```php
#[\AllowDynamicProperties]
final class __PHP_Incomplete_Class {

}
```

## Status

- phpscript does not implement this class. The name is not declared.
- Nothing would produce one: `unserialize()` is not implemented, so there is no path that needs a placeholder for a class it could not build.
- Decode with `json_decode()`, which raises an error on input it cannot read rather than handing back a value that stands for the failure.
