# The web root `phpscript server` requires.

It carries no endpoint. Every endpoint in this application is a route
annotation on a file outside it, and the scan passes over a route annotation
inside `public/`.
