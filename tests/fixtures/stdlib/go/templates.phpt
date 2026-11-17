name: open and close tags mix html with evaluated code
description: >
  A closing tag ends the php block and the text after it is echoed as it is
  written, line breaks and indentation included. A foreach between two blocks
  repeats a row and a short echo tag writes the value. ob_start() around the
  markup captures it and the script echoes the fragment once. php runs the
  body unchanged, so both columns define the output.
runtime: go
runner:
  php: true
---
<?php

ob_start();
?>
<ul>
<?php foreach (["a", "b"] as $item) { ?>
    <li><?php echo $item; ?></li>
<?php } ?>
</ul>
<?php
$page = ob_get_clean();
echo $page;
---
<ul>
    <li>a</li>
    <li>b</li>
</ul>
