# Variables

| PHP language-reference feature  | Status                | Notes                                                                                                          |
|---------------------------------|-----------------------|----------------------------------------------------------------------------------------------------------------|
| Basics                          | Compatibility         | Variables use the `$name` form and are created by assignment.                                                  |
| Predefined variables            | Partial compatibility | Only the request variables documented in [Predefined variables](../predefined-variables/README.md) are seeded. |
| Variable scope                  | Partial compatibility | Calls have local scope; PHP `global` and static locals are unavailable.                                        |
| Variable variables              | Not implemented       | Forms such as `$$name` are unavailable.                                                                        |
| Variables from external sources | Partial compatibility | HTTP query, form, and route values are provided by the request context.                                        |
| References                      | Not implemented       | `&` is accepted in limited positions but does not create PHP reference semantics.                              |

## Basics

```php
$name = "Ada";
$items = array();
$items[] = $name;
```

Variables, array indexes, object properties, and `list(...)` targets can be
assigned. The supported compound assignments are `+=`, `-=`, and `.=`.

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
