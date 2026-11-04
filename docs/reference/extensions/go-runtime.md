# Go runtime

`use Runtime\Go;` binds go's builtins for the rest of the file: `make`, `append`, `len`, `cap`, `delete`, `clear`, `panic`, `println` and go's seventeen builtin type names answer by their bare spelling, and the same bindings answer qualified as `Runtime\Go\make` or `Go\make` under the import's alias. The import also admits the `[]T` type, which php has no spelling for.

The names arrive with the import and no other way. A file that imported none of them resolves `make` and `len` the way php does, `function_exists("make")` reports false, `get_defined_functions()` lists neither, and the [implemented API inventory](implemented-apis.md) covers the surface php defines. An embedding host installs the area without an import by calling `rt.UseNamespace("Runtime\\Go")`.

php rejects a file that uses the syntax: `make([]string, 0, 10)` is a parse error under php and `len($s)` resolves to no php function. Keep the import out of source that must also run on php.

## Builtins

| Call                        | Behaviour                                                              |
|-----------------------------|------------------------------------------------------------------------|
| `make([]T, $len, $cap)`     | A go slice of the element type, the length and the capacity given.     |
| `append($dst, ...$values)`  | Appends to a go slice or a php array and returns the collection.       |
| `len($value)`               | A string's bytes, a php array's entries, a go collection's length.     |
| `cap($value)`               | A go slice's capacity; a php array reports its length.                 |
| `delete($collection, $key)` | Removes a key from a php array or a go map.                            |
| `clear($collection)`        | Empties a php array or a go map, and zeroes a go slice of its length.  |
| `panic($value)`             | Throws a `RuntimeException` carrying `panic: ` and the rendered value. |
| `println(...$values)`       | Writes the values spaced, with a newline, to the script's output.      |
| `int64($value)`             | One of the seventeen type conversions below.                           |

Four more carry go's meaning under the namespace alone, php owning the bare name: `Go\copy($dst, $src)` copies into `$dst` up to the shorter length and returns the count, `Go\min` and `Go\max` read values where php's read an array, and `Go\print` writes without the spacing `println` adds.

## Slices

`make`'s first argument is a type and the one argument in the language written as one. The element type is `any`, `bool`, `string`, `int`, `int64`, `float64`, a class name, or a `[]` nesting of those; a class name allocates the element php carries an instance in, which is `any`.

```php
use Runtime\Go;

$s = make([]string, 0, 10);
$s = append($s, "a", "b");
echo len($s), ",", cap($s);     // 2,10

$rows = make([][]int64, 2);     // two empty slices
$users = make([]User, 1);       // one null, under the class's element type
```

A length over the capacity, and an element type the list above leaves out, each throw a `RuntimeException` naming the type spelling.

A `[]T` type hint declares the same types on a parameter, on a return type and on a property:

```php
class Row
{
    public []int64 $counts;

    public function fill([]int64 $counts, ?[]Row $rows): []int64
    {
        return append($counts, int64(1));
    }
}
```

No engine reads a type hint, here as everywhere else in phpscript: the parser records the spelling and the formatter prints it back, and a value of another type reaches the body unchallenged.

## Conversions

Each of go's builtin type names converts one value: `any`, `bool`, `string`, `int`, `int8`, `int16`, `int32`, `int64`, `rune`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `byte`, `float32` and `float64`. php reads the value through its own semantics first, where `"12"` becomes 12, and go's width truncates what comes out: `int8(300)` is 44 and `uint8(-1)` is 255. The carrier stays php's, whose int is 64 bits wide and whose float is the wide one, and `float32(0.1)` reads back as the float64 nearest that float32. A call with no argument converts null, which is the type's zero value.

A conversion of a literal folds where the file parses and reaches no function table at run time, under every one of the seventeen names. The parser and the binding read one table, and `int8(300)` and `int8($n)` over a `$n` of 300 answer the same 44.

Each name is also a class, and the constructor runs the same conversion:

```php
use Runtime\Go;

var_dump(class_exists('Runtime\Go\int64'));   // bool(true)
var_dump(new Go\int64("7"));                  // int(7)
```

## What stays php's

The import binds go's spelling and no variant of it. `MAKE()` and `Make()` reach php, which folds a function's case; `\make()` and any other qualified call resolve as written. `copy`, `min`, `max` and `print` keep php's meaning in an importing file, and the namespace is how a script names go's four.

A sized width converts a value and allocates no slice: `make([]int8, 1)` throws, because php renders no sized numeric and a slice of them would read back through `var_dump` as the wide type anyway. `make([]int64, 1)` and a per-element `int8()` are the pair that holds today.

## Fixtures

`tests/fixtures/stdlib/go/` runs the area on both engines. Each file states the expected output on its own, php running none of them.
