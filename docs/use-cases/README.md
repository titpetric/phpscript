# Use cases walkthrough

The `phpscript` project has similar use cases to PHP. With implicit error checking and source code evaluation that matches the experience of PHP, it can be pretty much used in the same ways as you would use PHP.

- [Building an application](application.md)
- [Usage of Go bindings](bindings.md)
- [Custom entrypoints](entrypoints.md)
- [Error handling](error-handling.md)
- [Serving static files](static-files.md)
- [Routing](routing.md)
- [Virtual hosting](virtual-hosting.md)
- [Database bindings](database.md)
- [HTTP client bindings](http-client.md)
- [HTTP server bindings](http-server.md)
- [Mail bindings](mail.md)
- [Shared memory bindings](shared-memory.md)
- [Templating](templating.md)

Combining these principles you can create lightweight web applications that follow simple PHP conventions without OOP. This repository carries two, and tests cover both:

- [demos/example](../../demos/example) - the application the walkthrough builds
- [demos/dbadmin](../../demos/dbadmin) - a phpMyAdmin-style SQLite console
