# Variables

| PHP language-reference feature  | Status                | Notes                                                                                                                     |
|---------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------------|
| Basics                          | Compatibility         | Variables use the `$name` form and are created by assignment.                                                             |
| Predefined variables            | Partial compatibility | Only the request variables documented in [Predefined variables](../predefined-variables/README.md) are seeded.            |
| Variable scope                  | Partial compatibility | Calls have local scope; PHP `global` and static locals are unavailable.                                                   |
| Variable variables              | Not implemented       | Forms such as `$$name` are unavailable.                                                                                   |
| Variables from external sources | Partial compatibility | HTTP query, form, and route values are provided by the request context.                                                   |
| References                      | Partial compatibility | `foreach ($a as &$v)` works; `&` elsewhere parses but binds by value. See [Value semantics](../types/value-semantics.md). |

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

PHP's `global` statement and static local variables are not implemented.

## Destructuring

`list(...)` assignment is supported:

```php
list($id, $name) = $row;
```

PHP's `foreach ($rows as list($id, $name))` destructuring form is parsed but is
not implemented by the runtime.
