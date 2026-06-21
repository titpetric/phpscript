# Syntax decisions

The phpscript runtime implements a subset of PHP syntax. The syntax
allows for usage of `class`, `new`, `throw`, `catch` and generally
supports PHP expression syntax for conditions, loops, ternary operators
and more.

This is a list of unsupported syntax:

- Namespaces
- Inheritance
- Interfaces, traits, implements
- Public / private variables
- Public / private class methods

Supported syntax:

- Statements in file
- Class + statements in file
- Class in file
- Composition with `include`
- Field and method access

The syntax is in essence a non-OOP version of PHP4 with the extension of
a built in `Exception` type and `try` + `catch` statements for error
handling. This is in contrast with PHP4 `set_error_handler` and
`trigger_error` which are unimplemented APIs.
