# Deferred callbacks

`defer()` is a phpscript standard-library function, not language syntax like
Go's `defer`. It registers a callback to run when the current function or file
returns. Multiple callbacks run in last-in, first-out order.

Use a closure:

```php
function work() {
	defer(function() { echo "done\n"; });

	echo "working\n";
}
```

Or defer a bound method:

```php
$db = new \PS\Database("app");
defer($db->close);
```
