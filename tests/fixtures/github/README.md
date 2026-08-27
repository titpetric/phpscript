# Issue reproductions

One fixture per open GitHub issue, in the `.phpt` format under a `.yml`
extension: the fixture runner only discovers `.phpt`, so these stay out of the
passing suite on purpose. Each one is written against the *expected*
behaviour and fails phpscript today. Fixing the issue is done when its file
passes; rename it to `.phpt`, move it to the area it belongs to, and close
the issue with it.
