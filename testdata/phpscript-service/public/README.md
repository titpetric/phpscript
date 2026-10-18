# The web root `phpscript server` requires.

It is empty on purpose: every endpoint in this application is a route
annotation on a file outside it, and a route annotation inside `public/` is
ignored.
