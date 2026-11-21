# Go runtime

A binding is a Go function. The values it takes and returns are Go's: a slice, a map, an integer of a stated width. `use Runtime\Go;` writes those values in php source. `make` allocates the slice a binding reads. `uint8($n)` stays a uint8 across an assignment or a return, and `len` counts either side's collection. The configuration key `runner.runtime: go` selects the namespace for the whole runtime. No file writes the statement.

The import binds go's builtin functions and type names, which [godoc](https://pkg.go.dev/builtin) documents: `make`, `append`, `len`, `cap`, `copy`, `delete`, `clear`, `min`, `max`, `panic`, `print`, `println`, and go's type names for booleans, strings and numbers. They answer bare for the rest of the file. Qualified names `Go\make` and `Runtime\Go\make` work wherever the alias reaches. Two further names belong to this runtime: `getenv` answers the variable's value, null for a name the process does not hold, and `numcpu` answers `runtime.NumCPU` without an import. The import also admits the `[]T` and `*T` types, the `pkg.Type` selector its imports spell, the `[...]` and `{...}` composite literals, and the `&x` and `*p` operators. php has no spelling for those.

An import is the more specific name. `min`, `max`, `copy` and `getenv` shadow php's functions of those names in the importing file. `\min([3, 1, 2])` is php's, the way `\strlen($s)` is php's whatever the file imported. `MAKE()` reaches php, which folds a function's case where this binds one spelling.

php's own `use Runtime\Go;` imports a name, not a function: php aliases `Go\make()` onto `Runtime\Go\make()` and leaves a bare `min()` at php's. Binding the bare names is this runtime's addition, on the precedent of php's `use function Acme\min;`, which makes the short name the import and leaves `\min` to php. phpscript reads the plain `use` alone, and `use function Runtime\Go\min;` imports nothing here.

The names arrive with the import, or with `runner.runtime: go` for every file. No other route exists. A file that imported none of them, with a runtime configured for php, resolves `make` and `len` the way php does. `function_exists("make")` reports false, and the [implemented API inventory](implemented-apis.md) covers the surface php defines. An embedding host installs the area with `rt.UseNamespace("Runtime\\Go")`.

## The configuration key

`runner.runtime` takes `phpscript` or `go`, with `phpscript` the default. `go` installs `Runtime\Go`. It installs no area under `Runtime\PS` or `Runtime\PX`, and parses every file as though the file wrote `use Runtime\Go;`: the builtins, the `[]T` type and the [import statement](go-imports.md). In that mode, `strlen`, `phpinfo` and the rest of php's surface answer as undefined names. The configuration refuses a non-empty `runner.areas` beside `go`.

```yaml
runner:
  runtime: go
```

The worked example is [`testdata/phpscript-go-runtime-server`](../../../testdata/phpscript-go-runtime-server/README.md).

The `use` binds one statement as well: `import "net/http";` reaches go's own packages, and `http.NewRequest(...)` is a name of the area that binds them. [Go imports](go-imports.md) is that half.

php rejects a file that uses the syntax: `make([]string, 0, 10)` is a parse error under php and `len($s)` resolves to no php function. Keep the import out of source that must also run on php.

## Builtins

Each builtin behaves as go's. Where php holds a value in the same place, this is the behaviour:

| Call                        | Over a php value                                             |
|-----------------------------|--------------------------------------------------------------|
| `append($dst, ...$values)`  | A php array grows in place, and the handle comes back.       |
| `len($value)`               | An array's entries, a string's bytes, zero for null.         |
| `cap($value)`               | An array reports its length.                                 |
| `delete($collection, $key)` | Takes a php array as well as a go map.                       |
| `clear($collection)`        | Empties a php array; zeroes a go slice of its length.        |
| `copy($dst, $src)`          | Reads a php array as the source.                             |
| `min`, `max`                | Values, never an array. `\min([3, 1, 2])` is php's.          |
| `panic($value)`             | Throws a `RuntimeException` reading `panic: ` and the value. |
| `print`, `println`          | Write to the script's output.                                |
| `getenv($name)`             | The process environment, null for a name it does not hold.   |
| `numcpu()`                  | The logical CPUs the process can use.                        |

`new`, `recover`, `close` and the complex family stay unbound: php writes `new` and `catch`. The rest need [channels](#no-channels) or a value php has no spelling for.

## Errors

Go holds a single failure value, `error`. It has no class hierarchy. In the go runtime, `try`, `catch` and `throw` speak it:

- `catch (error $e)` and `catch ($e)` are the two clauses, and an error matches both. `$e` holds the thrown error, a `%w` wrapper included, as `if err != nil` reads the value.
- `throw` takes an error: `throw errors.New("...")`, `throw fmt.Errorf("...")`, or a value a binding returned. Any other value raises the `TypeError` that reports `throw(): Argument #1 must be of type error, string given`.
- A go binding that returns a non-nil error raises it, and `catch (error $e)` catches the call as go does.
- The runtime installs no PHP throwable class. `Exception`, `RuntimeException`, `TypeError` and the rest name nothing. `catch (Exception $e)` fails the parse, and `new Exception` reports an undefined class.
- `finally` runs as in the php runtime. `exit` and the time limit stay uncatchable in either mode.

```php
import "errors";
import "fmt";

try {
    throw fmt.Errorf("open %s: %w", "db", errors.New("refused"));
} catch (error $e) {
    echo $e, "\n";   // open db: refused
}
```

`import "errors"` installs `Runtime\Go\Errors`, holding `New`, and `fmt` holds `Errorf` together with the formatting calls. The php runtime installs its exception classes and reads `catch (Exception $e)`.

## Output

`echo` writes to the runtime's output in either namespace, and the open and close tags still mix HTML with evaluated code. The parser echoes the text between `?>` and the next `<?php` as it stands, line breaks and indentation included. A `foreach` between two blocks repeats a row.

```php
ob_start();
?>
<ul>
<?php foreach ($rows as $row) { ?>
    <li><?php echo $row; ?></li>
<?php } ?>
</ul>
<?php
$list = ob_get_clean();
echo $list;
```

The go runtime keeps php's output buffers for templates: `ob_start`, `ob_get_level`, `ob_get_contents`, `ob_get_clean`, `ob_end_clean`, `ob_end_flush` and `ob_get_flush` scope what `echo` writes, and the call that closes a buffer returns or writes its contents. `ob_start` accepts php's callback and chunk size and ignores both, because nothing transforms the buffered text.

In a server the output is the request's response writer. A handler echoes its body to the writer. `header("Name: value")` stages a response header ahead of the first write. A second echo writes a fragment rendered into a string into the page. An echo before a throw has already begun the response. Outside a server the output is the process's standard output, where `print` and `println` write as well. `echo` reaches the response writer's `WriteString`, which takes the string without a copy.

A go-runtime file writes member and method names in lowerCamelCase, as `$r->url->query()->get($name)` and `$r->pathValue($name)` show. The names resolve whatever their case. `phpscript fmt` rewrites them to this spelling.

## Conversions

A type name converts one value. php applies php's semantics first: `"12"` becomes 12. Go's width then truncates the result: `int8(300)` is 44 and `uint8(-1)` is 255. A call without an argument answers the type's zero value.

`int`, `int64` and `float64` name widths php already holds, and `int64(5)` is php's 5. The sized names answer a typed value instead. Each name is also a class, and `new Go\int64("7")` converts through it.

A literal folds while the file parses and reaches no function table at run time. The parser and the binding read one table. `int8(300)` and `int8($n)` answer the same `int8(44)` for a `$n` of 300.

## Typed values

A typed value holds one of go's sized widths. The type travels with it across a variable, an array, a slice, a binding, a return, and `var_dump` names the width:

```php
use Runtime\Go;

var_dump(int8(300));            // int8(44)
var_dump(uint8(-1));            // uint8(255)
var_dump(float32(0.5));         // float32(0.5)
var_dump(int64(5));             // int(5)
```

Arithmetic keeps the width and wraps in it, as go's does. Three rules decide the width of a result:

- two values of one width answer that width.
- a typed value with one of php's numbers answers the typed width, the way go reads an untyped constant.
- two different widths answer a `TypeError` reading `Unsupported operand types: int8 + int16`, as go refuses the pair. Write the conversion, as php has no promotion rule between widths to borrow.

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

php's own operators read the number the value holds: `==` and `<` compare numerically, `gettype` answers `integer` or `double`, and the casts convert. The go runtime compares with go's `==` and `!=`. The parse rejects both `===` and `!==`. php functions answer php's types for a typed argument, and `abs(int8(-5))` is php's 5.

An unsigned value past `PHP_INT_MAX` prints what go prints. `echo`, `%d` and `%u` write `18446744073709551615`, and the integer domain clips: `(int)` and `intval()` answer `PHP_INT_MAX`, where `(float)` answers the float holding the value.

`var_export` prints the conversion back, `int8(44)`, which reproduces the value in a file that imported the runtime. `echo`, `print_r` and `json_encode` print the number alone.

## Composite literals

`[...]` builds a slice, `{key: value}` a map. A literal naming its element type builds the go value that type names. The inferred forms answer `[]any` and `map[string]any`:

```php
use Runtime\Go;

$list = [1, 2, 3];                      // []any
$user = {"name": "ada", "age": 36};     // map[string]any
$names = []string{"ada", "grace"};      // []string
$ages = map[string]int{"ada": 36};      // map[string]int64
```

The element type is a builtin type name. A typed literal narrows each entry as the conversion does. `[]int8{300}` holds `int8(44)`. A class name holds `any`. A type nests. `[][]string`, `map[string][]int` and `[]map[string]int` build every level, converting each entry on the way in. A braced entry elides the element type it repeats: `[][]string{{"a"}}` is `[][]string{[]string{"a"}}`. The formatter writes the type back. A dims-only literal takes its base type from the values: `[][]{{"a"}}` is `[][]string`.

`array()` is a parse error here. A php `[key => value]` entry is also a parse error: the braced form is the map. Indexing, assignment and `delete` read and write the container the literal built, and `delete` removes a map key as go's does.

A literal is a full slice, and `append` answers the slice it grew. `$s = [1, 2]; append($s, 3);` leaves `$s` with two entries, while `$s = append($s, 3)` keeps the third.

## Pointers

`&x` answers a pointer to the storage x names, and `*$p` reads that storage. An assignment through the pointer writes the storage, while an assignment to the value copies it:

```php
use Runtime\Go;

$a = 1;
$p = &$a;
*$p = 2;
echo $a;            // 2

$b = $a;            // a copy
$b = 3;
echo $a;            // still 2
```

An element takes an address, `&$list[1]`. A property takes an address: `&$obj->field`. A compound assignment writes through the pointer, `*$p += 1`. A binding's native `*T` dereferences to the value it holds.

The operators arrive with the import, as the literals do. A php file keeps `&` as the by-reference marker and `*` as multiplication.

## Slices

`make`'s first argument is a type, the only argument in the language written that way. The element type is any builtin type name, a class name, or a `[]` nesting of any of them. A class name allocates `any`, the element php stores an instance in.

```php
use Runtime\Go;

$s = make([]string, 0, 10);
$s = append($s, "a", "b");
echo len($s), ",", cap($s);     // 2,10

$rows = make([][]int64, 2);     // two empty slices
$users = make([]User, 1);       // one null
$widths = make([]int8, 2);      // two int8 zeroes
```

A length over the capacity throws a `RuntimeException`. A spelling that names no slice throws the same.

`[]uint8` and `[]byte` are a single type in go. phpscript reads it as php's string: it renders as text, takes a string offset, and iterates zero times. `[]int16` is the narrowest slice whose elements read back as numbers.

A `[]T` type hint declares the same types on a parameter, a return type, a property. No engine reads it, as with all hints here:

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

[Channels](https://go.dev/ref/spec#Channel_types) stay out. The decision stands. A channel is a rendezvous between goroutines, and a phpscript program starts none. The `go` statement has no spelling here, `select` waits on no case. A channel with one end is a deadlock behind an API.

Concurrency here is the server. Each request answers on a [separate runtime](../../use-cases/http-server.md). Two requests share state through [`SharedMemory`](../../use-cases/shared-memory.md), a database, or a queue a binding owns. A binding whose Go side runs goroutines keeps the channels on that side and returns the result.
