# The Iterator interface

(PHP 5, PHP 7, PHP 8)

## Introduction

Interface for external iterators, or for objects that walk themselves internally.

A class implementing it declares five methods `foreach` calls to walk it: `rewind()` to start, `valid()` to report whether a value exists at the current position, `current()` and `key()` to read it, plus `next()` to advance.

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

phpscript declares none of them. PHP ships `ArrayIterator`, `DirectoryIterator`, `SplObjectStorage` and the rest of SPL. None of those names exists here. Build the sequence in an array, or write the walk out with the methods below.

## Examples

In PHP, `foreach` calls the interface methods in a fixed order: `rewind()`, then `valid()`, `current()` and `key()` on each pass, then `next()`. Writing that order out by hand is the workaround below, because phpscript will not call them for you.

## See also

[Object iteration](../../control-structures/README.md), plus [Value semantics](../../types/value-semantics.md), covering what `foreach` binds.

## Status

- phpscript declares no `Iterator`. PHP's class name stays undeclared here. `instanceof` answers false, and `class_exists` returns false.
- The interface uses `extends`. `interface Iterator extends Traversable` parses here, and the parser records the extended name. `instanceof Traversable` follows that name. The relation delivers no members: an interface contributes no method body, property or constant in either language. Traversable declares no method. Flattening the two loses nothing.
- Interface-based dispatch does not exist. This is the incompatibility that matters: a class can declare all five methods and meet the contract, yet `foreach` still will not call them. `foreach` reads an object's properties, ignoring the sequence those five methods describe.
- `IteratorAggregate`, `yield` and the [Generator](Generator.md) class are all unavailable. No other route to a lazy sequence exists.

A workaround, without `extends` and without relying on dispatch:

```php
<?php

// Iterator without `extends`: Traversable declares no method, and the flattened
// interface is Iterator's own five and nothing is lost by dropping the parent.
//
// In phpscript the name is free, because no built-in declares it. Real PHP already
// defines Iterator and would refuse this file with "Cannot redeclare interface
// Iterator", and name it Sequence, or guard it, in code that has to run on both.
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
