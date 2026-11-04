# Go runtime

A binding is a Go function, and the values it takes and returns are Go's: a slice, a map, an integer of a stated width. `use Runtime\Go;` writes those values in php source. `make` allocates the slice a binding reads, `uint8($n)` stays a uint8 across an assignment and a return, and `len` counts either side's collection.

The import binds go's builtin functions and type names, which [godoc](https://pkg.go.dev/builtin) documents: `make`, `append`, `len`, `cap`, `copy`, `delete`, `clear`, `min`, `max`, `panic`, `print`, `println`, and go's type names for booleans, strings and numbers. They answer bare for the rest of the file, and qualified as `Go\make` or `Runtime\Go\make` wherever the alias reaches. The import also admits the `[]T` type, which php has no spelling for.

An import is the more specific name, and `min`, `max` and `copy` shadow php's functions of those names in the importing file. `\min([3, 1, 2])` is php's, the way `\strlen($s)` is php's whatever the file imported, and `MAKE()` reaches php, which folds a function's case where this binds one spelling.

php's own `use Runtime\Go;` imports a name and no function with it: php aliases `Go\make()` onto `Runtime\Go\make()` and leaves a bare `min()` at php's. Binding the bare names is this runtime's addition, on the precedent of php's `use function Acme\min;`, which makes the short name the imported one and leaves `\min` for php's. phpscript reads the plain `use` alone; `use function Runtime\Go\min;` imports nothing here.

The names arrive with the import and no other way. A file that imported none of them resolves `make` and `len` the way php does, `function_exists("make")` reports false, and the [implemented API inventory](implemented-apis.md) covers the surface php defines. An embedding host installs the area with `rt.UseNamespace("Runtime\\Go")`.

php rejects a file that uses the syntax: `make([]string, 0, 10)` is a parse error under php and `len($s)` resolves to no php function. Keep the import out of source that must also run on php.

## Builtins

Each builtin behaves as go's. Where php has a value of its own in the same place, this is the behaviour:

| Call                        | Over a php value                                              |
|-----------------------------|---------------------------------------------------------------|
| `append($dst, ...$values)`  | A php array grows in place, and the handle comes back.        |
| `len($value)`               | An array's entries, a string's bytes, zero for null.          |
| `cap($value)`               | An array reports its length.                                  |
| `delete($collection, $key)` | Takes a php array as well as a go map.                        |
| `clear($collection)`        | Empties a php array; zeroes a go slice of its length.         |
| `copy($dst, $src)`          | Reads a php array as the source.                              |
| `min`, `max`                | Values, never an array. `\min([3, 1, 2])` is php's.           |
| `panic($value)`             | Throws a `RuntimeException` carrying `panic: ` and the value. |
| `print`, `println`          | Write to the script's output.                                 |

`new`, `recover`, `close` and the complex family stay unbound: php writes `new` and `catch` itself, and the rest need [channels](#no-channels) or a value php has none of.

## Conversions

A type name converts one value. php reads it through its own semantics first, where `"12"` becomes 12, and go's width truncates what comes out: `int8(300)` is 44 and `uint8(-1)` is 255. A call with no argument answers the type's zero value.

`int`, `int64` and `float64` name widths php already carries, and `int64(5)` is php's 5. The sized names answer a typed value instead. Each name is a class too, and `new Go\int64("7")` converts through it.

A literal folds while the file parses and reaches no function table at run time. The parser and the binding read one table, and `int8(300)` and `int8($n)` over a `$n` of 300 answer the same `int8(44)`.

## Typed values

A typed value carries one of go's sized widths. The type travels with it, through a variable, an array, a slice, a binding and a return, and `var_dump` names the width:

```php
use Runtime\Go;

var_dump(int8(300));            // int8(44)
var_dump(uint8(-1));            // uint8(255)
var_dump(float32(0.5));         // float32(0.5)
var_dump(int64(5));             // int(5)
```

Arithmetic keeps the width and wraps in it, as go's does. Three rules decide the width of a result:

- two values of one width answer that width;
- a typed value and one of php's numbers answer the typed width, the way go reads an untyped constant;
- two different widths answer a `TypeError` reading `Unsupported operand types: int8 + int16`, as go refuses the pair. Write the conversion; php has no promotion rule between widths to borrow.

```php
use Runtime\Go;

var_dump(int8(100) + int8(100));    // int8(-56), where php's ints widen to a float
var_dump(int8(100) + 1);            // int8(101)
var_dump(uint8(0) - 1);             // uint8(255)
var_dump(int8(7) / int8(2));        // int8(3), go's truncating division
var_dump(~uint8(0));                // uint8(255), where php's int answers -1

$i = int8(127);
$i++;
var_dump($i);                       // int8(-128)
```

php's own operators read the number the value holds: `==` and `<` compare numerically, `gettype` answers `integer` or `double`, and the casts convert. `===` reads the type, and `float32(0.1) === 0.1` is false. A php function answers php's types over a typed argument, and `abs(int8(-5))` is php's 5.

An unsigned value past `PHP_INT_MAX` prints what go prints. `echo`, `%d` and `%u` write `18446744073709551615`, and the integer domain clips: `(int)` and `intval()` answer `PHP_INT_MAX`, where `(float)` answers the float holding the value.

`var_export` prints the conversion back, `int8(44)`, which reproduces the value in a file that imported the runtime. `echo`, `print_r` and `json_encode` print the number alone.

## Slices

`make`'s first argument is a type, the one argument in the language written as one. The element type is any builtin type name, a class name, or a `[]` nesting of those; a class name allocates `any`, the element php carries an instance in.

```php
use Runtime\Go;

$s = make([]string, 0, 10);
$s = append($s, "a", "b");
echo len($s), ",", cap($s);     // 2,10

$rows = make([][]int64, 2);     // two empty slices
$users = make([]User, 1);       // one null
$widths = make([]int8, 2);      // two int8 zeroes
```

A length over the capacity, and a spelling that names no slice, each throw a `RuntimeException`.

`[]uint8` and `[]byte` are one type in go, and phpscript reads that one as php's string: it renders as text, takes a string offset, and iterates zero times. `[]int16` is the narrowest slice whose elements read back as numbers.

A `[]T` type hint declares the same types on a parameter, a return type and a property. No engine reads it, as with every other hint here:

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

## No channels

[Channels](https://go.dev/ref/spec#Channel_types) stay out, and the decision stands. A channel is a rendezvous between goroutines and a phpscript program starts none: the `go` statement has no spelling here, `select` waits on no case, and a channel with one end is a deadlock behind an API.

Concurrency here is the server. Each request answers on a [runtime of its own](../../use-cases/http-server.md), and two of them share state through [`SharedMemory`](../../use-cases/shared-memory.md), a database, or a queue a binding owns. A binding whose Go side runs goroutines keeps the channels there and returns the result.
