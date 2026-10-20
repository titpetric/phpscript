# The Countable interface

(PHP 5 >= 5.1.0, PHP 7, PHP 8)

## Introduction

The `count()` function accepts an object whose class implements Countable.

`count()` calls the single method it names when its argument is an object of a class that declares the interface.

## Interface synopsis

```php
interface Countable {

/* Methods */

public function count(): int

}
```

## Methods

| Method                    | Description                 |
|---------------------------|-----------------------------|
| `Countable::count(): int` | Count elements of an object |

## Status

- phpscript has no `Countable`. The parser accepts `implements Countable` and checks nothing, because no `interface` declaration in the same file defines the name.
- Nothing dispatches through it: `count()` does not call a `count()` method on an object. Call the method by name, or count the array the object holds.
