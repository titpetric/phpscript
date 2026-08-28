name: images are created, sized and freed
description: >
  imagecreatetruecolor answers a handle a script holds in a variable, and
  imagesx/imagesy read its size back. A zero or negative dimension raises the
  ValueError PHP 8 raises rather than answering false, and imagedestroy answers
  true once.
---
<?php

$true = imagecreatetruecolor(64, 32);
echo imagesx($true), "x", imagesy($true), "\n";

try {
	imagecreatetruecolor(0, 10);
	echo "no-error\n";
} catch (\Error $e) {
	echo "width-refused\n";
}
try {
	imagecreatetruecolor(10, -1);
	echo "no-error\n";
} catch (\Error $e) {
	echo "height-refused\n";
}

var_dump(imagedestroy($true));
---
64x32
width-refused
height-refused
bool(true)
