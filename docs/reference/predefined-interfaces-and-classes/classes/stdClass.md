# The stdClass class

(PHP 4, PHP 5, PHP 7, PHP 8)

## Introduction

Generic empty class with no methods and no properties of its own.

It is the class an object gets when nothing else names one: `new stdClass`, a cast with `(object)`, and in PHP the object shape returned by `json_decode()` and by database fetch methods. It is not a universal base class.

## Class synopsis

```php
#[\AllowDynamicProperties]
class stdClass {

}
```

## Status

- phpscript implements this class. `new stdClass` and `new stdClass()` both build one, `class_exists("stdClass")` is true, and `get_class()` answers `stdClass`.
- Every property is added by assignment and reads back in the order it was added, which is what `json_encode()`, `print_r()`, `var_dump()`, `var_export()`, `get_object_vars()`, the `(array)` cast and `foreach` all show.
- The `(object)` cast builds one from an array, a scalar or null. See [Type casting](../../types/README.md#type-casting) for the two divergences: the cast shares rather than copies, and `(array)` does not mangle private property names.
- A script may declare its own `class stdClass`, which shadows the built-in for that program.
- `json_decode()` does not return one: decoding into objects is not implemented, and `$associative` must be true or omitted.
