# Reserved interfaces and classes

The interfaces and classes PHP predefines, in the order the PHP manual lists them at
[Predefined Interfaces and Classes](https://www.php.net/manual/en/reserved.interfaces.php),
with what phpscript does about each. Every row links to a document describing the name, its
synopsis as PHP declares it, and a `Status` section recording where phpscript differs.

One thing is true of every row and is not repeated in each: **phpscript declares none of these
names except `stdClass`, and nothing dispatches through an interface.** A class may write
`implements Countable` and it will load, because a name no `interface` declaration in the same
file defines is not a contract and is not checked; but `count()` will not call its `count()`
method. Declaring an interface is a claim about shape, and here it stays one.

## Table of contents

- [Traversable](classes/Traversable.md)
- [Iterator](classes/Iterator.md)
- [IteratorAggregate](classes/IteratorAggregate.md)
- [InternalIterator](classes/InternalIterator.md)
- [Throwable](classes/Throwable.md)
- [Countable](classes/Countable.md)
- [ArrayAccess](classes/ArrayAccess.md)
- [Serializable](classes/Serializable.md)
- [Closure](classes/Closure.md)
- [stdClass](classes/stdClass.md)
- [Generator](classes/Generator.md)
- [Fiber](classes/Fiber.md)
- [WeakReference](classes/WeakReference.md)
- [WeakMap](classes/WeakMap.md)
- [Stringable](classes/Stringable.md)
- [UnitEnum](classes/UnitEnum.md)
- [BackedEnum](classes/BackedEnum.md)
- [SensitiveParameterValue](classes/SensitiveParameterValue.md)
- [__PHP_Incomplete_Class](classes/__PHP_Incomplete_Class.md)

## Reserved names

| Name                      | Type      | Added in                            | phpscript            | Document                                                         |
|---------------------------|-----------|-------------------------------------|----------------------|------------------------------------------------------------------|
| `Traversable`             | Interface | PHP 5, PHP 7, PHP 8                 | Not implemented      | [Traversable.md](classes/Traversable.md)                         |
| `Iterator`                | Interface | PHP 5, PHP 7, PHP 8                 | Not implemented      | [Iterator.md](classes/Iterator.md)                               |
| `IteratorAggregate`       | Interface | PHP 5, PHP 7, PHP 8                 | Not implemented      | [IteratorAggregate.md](classes/IteratorAggregate.md)             |
| `InternalIterator`        | Class     | PHP 8                               | Not implemented      | [InternalIterator.md](classes/InternalIterator.md)               |
| `Throwable`               | Interface | PHP 7, PHP 8                        | Catch clause only    | [Throwable.md](classes/Throwable.md)                             |
| `Countable`               | Interface | PHP 5 >= 5.1.0, PHP 7, PHP 8        | Not implemented      | [Countable.md](classes/Countable.md)                             |
| `ArrayAccess`             | Interface | PHP 5, PHP 7, PHP 8                 | Not implemented      | [ArrayAccess.md](classes/ArrayAccess.md)                         |
| `Serializable`            | Interface | PHP 5 >= 5.1.0, PHP 7, PHP 8        | Not implemented      | [Serializable.md](classes/Serializable.md)                       |
| `Closure`                 | Class     | PHP 5 >= 5.3.0, PHP 7, PHP 8        | Value only, no class | [Closure.md](classes/Closure.md)                                 |
| `stdClass`                | Class     | PHP 4, PHP 5, PHP 7, PHP 8          | Implemented          | [stdClass.md](classes/stdClass.md)                               |
| `Generator`               | Class     | PHP 5 >= 5.5.0, PHP 7, PHP 8        | Won't implement      | [Generator.md](classes/Generator.md)                             |
| `Fiber`                   | Class     | PHP 8 >= 8.1.0                      | Won't implement      | [Fiber.md](classes/Fiber.md)                                     |
| `WeakReference`           | Class     | PHP 7 >= 7.4.0, PHP 8               | Not implemented      | [WeakReference.md](classes/WeakReference.md)                     |
| `WeakMap`                 | Class     | PHP 8                               | Not implemented      | [WeakMap.md](classes/WeakMap.md)                                 |
| `Stringable`              | Interface | PHP 8                               | Not implemented      | [Stringable.md](classes/Stringable.md)                           |
| `UnitEnum`                | Interface | PHP 8 >= 8.1.0                      | Not implemented      | [UnitEnum.md](classes/UnitEnum.md)                               |
| `BackedEnum`              | Interface | PHP 8 >= 8.1.0                      | Not implemented      | [BackedEnum.md](classes/BackedEnum.md)                           |
| `SensitiveParameterValue` | Class     | PHP 8 >= 8.2.0                      | Not implemented      | [SensitiveParameterValue.md](classes/SensitiveParameterValue.md) |
| `__PHP_Incomplete_Class`  | Class     | PHP 4 >= 4.0.1, PHP 5, PHP 7, PHP 8 | Not implemented      | [__PHP_Incomplete_Class.md](classes/__PHP_Incomplete_Class.md)   |

## Reading the phpscript column

| Value                | Meaning                                                                                                              |
|----------------------|----------------------------------------------------------------------------------------------------------------------|
| Implemented          | The name is declared and behaves as PHP's does, within the divergences the document records.                         |
| Catch clause only    | The name is recognised in one position and is not a declared type.                                                   |
| Value only, no class | The runtime value exists; the class wrapping it in PHP does not.                                                     |
| Won't implement      | A decision recorded in [Design decisions](../../design.md), not a gap.                                               |
| Not implemented      | The name is not declared. Some are gaps, some rest on a feature that is itself a decision; each document says which. |

## Interfaces that use extends

Four of the interfaces above are declared with `extends`:
[Iterator](classes/Iterator.md) and [IteratorAggregate](classes/IteratorAggregate.md) extend
[Traversable](classes/Traversable.md), [Throwable](classes/Throwable.md) extends
[Stringable](classes/Stringable.md), and [BackedEnum](classes/BackedEnum.md) extends
[UnitEnum](classes/UnitEnum.md).

`interface A extends B` parses here and the extended names are recorded, so `instanceof` answers
them; what it does not do is carry a method across. A class implementing the child must declare
every method both name. Where the parent declares no method, as Traversable does, flattening the
two costs nothing, and [Iterator](classes/Iterator.md#status) shows the flattened form.

## See also

- [Predefined interfaces and classes](README.md), the compatibility summary for this chapter
- [Classes and objects](../classes-and-objects/README.md), for what a class declaration supports
- [Design decisions](../../design.md), for the names that are decisions rather than gaps
