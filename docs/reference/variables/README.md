# Variables

| PHP language-reference feature  | Status                | Notes                                                                                                                                                           |
|---------------------------------|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Basics                          | Compatibility         | Variables use the `$name` form; an assignment creates one.                                                                                                      |
| Predefined variables            | Partial compatibility | The runtime seeds only the request variables documented in [Predefined variables](../predefined-variables/README.md).                                           |
| Variable scope                  | Partial compatibility | Calls have local scope; `static $x` persists per function (per closure value); `global` is a won't-build no-op.                                                 |
| Variable variables              | Partial compatibility | `new $className` and `$obj->$method(...)` resolve the name at run time; `$$name` is unavailable.                                                                |
| Variables from external sources | Partial compatibility | The request context carries HTTP query, form, and route values.                                                                                                 |
| References                      | Partial compatibility | `foreach ($a as &$v)` works; `$a = &$b` and `function &f()` parse, survive the formatter and bind by value. See [Value semantics](../types/value-semantics.md). |

## Basics

```php
$name = "Ada";
$items = array();
$items[] = $name;
```

An assignment can target a variable, an array index, an object property, a static property, or a `list(...)` target. Every PHP compound assignment applies: `+=`, `-=`, `*=`, `/=`, `%=`, `**=`, `.=`, `&=`, `|=`, `^=`, `<<=` and `>>=`.

Writing through an index that does not exist yet creates the arrays on the way down, as PHP does:

```php
$tree = array();
$tree["one"]["two"] = "deep";      // both levels are created
```

`unset()` removes a variable, an array entry, an object property, or a static property. Removing an array entry does not renumber the entries around it.

```php
$row = array("a" => 1, "b" => 2, "c" => 3);
unset($row["b"]);
echo implode(",", array_keys($row));      // a,c
```

A value a binding returned as a native slice is the exception, because a slice is dense and cannot hold the hole PHP leaves. Removing an element renumbers the ones after it, as `array_values()` does; see the [divergences](../../README.md#known-divergences-from-php).

```php
$e = explode(",", "x,y,z");
unset($e[1]);
echo implode(",", array_keys($e));        // 0,1   php answers 0,2
```

## Variable scope

Each function call receives a local scope containing its arguments. A function does not implicitly see variables from its caller. Blocks do not create a scope of their own.

phpscript will not build PHP's `global` statement: it parses and binds nothing, and the variable it names stays unset inside the function. Pass the collaborator as a parameter instead; [Design decisions](../../design.md) records why.

Function-level `static` variables work as PHP defines them: the initializer runs once per function lifetime, later writes persist across calls, and every closure value carries its own static storage.

```php
function counter() {
	static $n = 0;
	$n++;
	return $n;                        // 1, 2, 3 across calls
}
```

## Destructuring

`list(...)` assignment works:

```php
list($id, $name) = $row;
```

The parser accepts PHP's `foreach ($rows as list($id, $name))` destructuring form, and the runtime then throws `assign: unsupported list() element` on the first iteration, where php destructures each row.
