# The Iterator interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface for external iterators or objects that can be iterated themselves internally.

A class implementing it declares the five methods `foreach` calls to walk it: `rewind()` to start, `valid()` to ask whether there is anything at the current position, `current()` and `key()` to read it, and `next()` to advance.

## Interface synopsis

```php
interface Iterator extends Traversable {

/* Methods */

public function current(): mixed

public function key(): mixed

public function next(): void

public function rewind(): void

public function valid(): bool

}
```

## Methods

| Method                       | Description                              |
|------------------------------|------------------------------------------|
| `Iterator::current(): mixed` | Return the current element               |
| `Iterator::key(): mixed`     | Return the key of the current element    |
| `Iterator::next(): void`     | Move forward to next element             |
| `Iterator::rewind(): void`   | Rewind the iterator to the first element |
| `Iterator::valid(): bool`    | Checks if current position is valid      |

## Predefined iterators

phpscript declares none of them. PHP ships `ArrayIterator`, `DirectoryIterator`, `SplObjectStorage` and the rest of SPL; none of those names is defined here. Build the sequence in an array, or write the walk out with the methods below.

## Examples

In PHP, `foreach` calls the interface methods in a fixed order: `rewind()`, then `valid()`, `current()` and `key()` on each pass, then `next()`. Writing that order out by hand is what the workaround below does, because phpscript will not call them for you.

## See also

[Object iteration](../../control-structures/README.md), and [Value semantics](../../types/value-semantics.md) for what `foreach` binds.

## Status

- phpscript does not implement this interface. The name is not declared, so `instanceof Iterator` is false and `class_exists("Iterator")` returns false.
- The interface uses `extends`. `interface Iterator extends Traversable` parses here and the extended name is recorded, so `instanceof Traversable` follows it; but nothing arrives through it, because an interface contributes no method body, property or constant in either language. Since Traversable declares no method, flattening the two loses nothing.
- There is no interface-based dispatch. This is the incompatibility that matters: a class can declare all five methods and satisfy the contract, and `foreach` still will not call them. `foreach` over an object reads its properties, so a loop over an iterator yields the object's internal state instead of the sequence it meant to expose.
- `IteratorAggregate`, `yield` and the [Generator](Generator.md) class are all unavailable, so there is no second route to a lazy sequence either.

A workaround, without `extends` and without relying on dispatch:

```php
<?php

// Iterator without `extends`: Traversable declares no method, so the flattened
// interface is Iterator's own five and nothing is lost by dropping the parent.
//
// In phpscript the name is free, because no built-in declares it. Real PHP already
// defines Iterator and would refuse this file with "Cannot redeclare interface
// Iterator", so name it Sequence, or guard it, in code that has to run on both.
interface Iterator {
	public function current();
	public function key();
	public function next();
	public function rewind();
	public function valid();
}

class Rows implements Iterator {
	private $items;
	private $keys;
	private $pos = 0;

	public function __construct($items) {
		$this->items = $items;
		$this->keys = array_keys($items);
	}

	public function current() {
		return $this->items[$this->keys[$this->pos]];
	}

	public function key() {
		return $this->keys[$this->pos];
	}

	public function next() {
		$this->pos = $this->pos + 1;
	}

	public function rewind() {
		$this->pos = 0;
	}

	public function valid() {
		return $this->pos < count($this->keys);
	}
}

// `foreach ($rows as $k => $v)` would read the properties $items, $keys and $pos,
// so write the walk the interface describes instead. This is the whole workaround:
// the contract still documents the shape, the caller drives it.
$rows = new Rows(array("a" => 1, "b" => 2));
for ($rows->rewind(); $rows->valid(); $rows->next()) {
	echo $rows->key(), "=", $rows->current(), "\n";
}

// a=1
// b=2
```
