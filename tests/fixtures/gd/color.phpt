name: colours pack the way libgd packs them
description: >
  A colour identifier is 0xAARRGGBB with alpha on GD's own 0 to 127 scale where
  0 is opaque, so an opaque colour is the plain 0xRRGGBB a script can read. The
  packing is what lets a colour live in a variable and reach any drawing call,
  so it is asserted as a number here rather than through the pixels it paints.
  imagecolorat reads a painted pixel back to the same identifier, and
  imagecolorsforindex splits one apart.
---
<?php

$im = imagecreatetruecolor(8, 8);

$red = imagecolorallocate($im, 255, 0, 0);
$white = imagecolorallocate($im, 255, 255, 255);
echo $red, " ", $white, "\n";

$grey = imagecolorallocate($im, 230, 230, 230);
echo $grey, "\n";

$parts = imagecolorsforindex($im, $grey);
echo $parts['red'], ",", $parts['green'], ",", $parts['blue'], ",", $parts['alpha'], "\n";

// A component outside 0 to 255 is refused, as it is in PHP 8.
try {
	imagecolorallocate($im, 300, 0, 0);
	echo "no-error\n";
} catch (\Error $e) {
	echo "red-refused\n";
}

imagefilledrectangle($im, 3, 3, 3, 3, $red);
var_dump(imagecolorat($im, 3, 3) === $red);
var_dump(imagecolorat($im, 99, 99));
---
16711680 16777215
15132390
230,230,230,0
red-refused
bool(true)
bool(false)
