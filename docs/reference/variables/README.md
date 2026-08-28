# Variables

| PHP language-reference feature  | Status                | Notes                                                                                                                                                               |
|---------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Basics                          | Compatibility         | Variables use the `$name` form and are created by assignment.                                                                                                       |
| Predefined variables            | Partial compatibility | Only the request variables documented in [Predefined variables](../predefined-variables/README.md) are seeded.                                                      |
| Variable scope                  | Partial compatibility | Calls have local scope; `static $x` persists per function (per closure value); `global` is a won't-implement no-op.                                                 |
| Variable variables              | Not implemented       | Forms such as `$$name` are unavailable.                                                                                                                             |
| Variables from external sources | Partial compatibility | HTTP query, form, and route values are provided by the request context.                                                                                             |
| References                      | Partial compatibility | `foreach ($a as &$v)` works; `$a = &$b` and `function &f()` parse, are kept by the formatter and bind by value. See [Value semantics](../types/value-semantics.md). |

## Basics

```php
$name = "Ada";
$items = array();
$items[] = $name;
```

Variables, array indexes, object properties, static properties, and `list(...)`
targets can be assigned. Every PHP compound assignment applies: `+=`, `-=`,
`*=`, `/=`, `%=`, `**=`, `.=`, `&=`, `|=`, `^=`, `<<=` and `>>=`.

Writing through an index that does not exist yet creates the arrays on the way
down, as PHP does:

```php
$tree = array();
$tree["one"]["two"] = "deep";      // both levels are created
```

`unset()` removes a variable, an array entry, an object property, or a static
property. Removing an array entry does not renumber the entries around it.

```php
$row = array("a" => 1, "b" => 2, "c" => 3);
unset($row["b"]);
echo implode(",", array_keys($row));      // a,c
```

## Variable scope

Each function call receives a local scope containing its arguments. A function
does not implicitly see variables from its caller. Blocks do not create an
additional scope.

PHP's `global` statement will not be implemented: it parses and binds nothing,
so the variable it names stays unset inside the function. Pass the collaborator
as a parameter instead; [Design decisions](../../design.md) records why.

Function-level `static` variables work as PHP defines them: the initializer
runs once per function lifetime, later writes persist across calls, and every
closure value carries its own static storage.

```php
function counter() {
	static $n = 0;
	$n++;
	return $n;                        // 1, 2, 3 across calls
}
```

## Destructuring

`list(...)` assignment is supported:

```php
list($id, $name) = $row;
```

PHP's `foreach ($rows as list($id, $name))` destructuring form is parsed but is
not implemented by the runtime.
