name: the include option runs before the entrypoint
description: >
  runner.include names a file included once per session before the entrypoint,
  which is what --include sets on every command. Its declarations are in place
  before the first statement of the script runs. php has no equivalent option.
options:
  include: prelude.php
runner:
  php: false
---
<?php

echo defined("PRELUDE_LOADED") ? "prelude loaded\n" : "prelude missing\n";
echo prelude_greet("world"), "\n";
echo get_included_files()[0], "\n";
?>
---
prelude loaded
hello, world
/prelude.php
