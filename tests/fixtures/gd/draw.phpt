name: lines, rectangles and the flood fill paint pixels
description: >
  imageline is Bresenham rather than an axis-aligned shortcut, so a diagonal
  lands on the pixels between its endpoints. GD rectangles include their far
  corner where a Go one would not, and imagefilledrectangle keeps GD's reading.
  imagefill replaces the connected region sharing the starting pixel's colour,
  which is what makes it useful for a background and not only for a blank
  image.
---
<?php

$im = imagecreatetruecolor(20, 20);
$black = imagecolorallocate($im, 0, 0, 0);
$red = imagecolorallocate($im, 255, 0, 0);
$blue = imagecolorallocate($im, 0, 0, 255);

// A horizontal line covers both endpoints.
imageline($im, 2, 5, 17, 5, $red);
var_dump(imagecolorat($im, 2, 5) === $red, imagecolorat($im, 17, 5) === $red);
var_dump(imagecolorat($im, 1, 5) === $red);

// A diagonal lands on its midpoint.
imageline($im, 0, 0, 10, 10, $blue);
var_dump(imagecolorat($im, 5, 5) === $blue);

// The far corner is inside the rectangle, as it is in GD.
$box = imagecreatetruecolor(10, 10);
imagefilledrectangle($box, 2, 2, 6, 6, $red);
var_dump(imagecolorat($box, 6, 6) === $red);
var_dump(imagecolorat($box, 7, 6) === $red);
var_dump(imagecolorat($box, 2, 2) === $red);

// A flood fill stops at a colour boundary.
$fill = imagecreatetruecolor(10, 10);
imageline($fill, 5, 0, 5, 9, $red);
imagefill($fill, 0, 0, $blue);
var_dump(imagecolorat($fill, 0, 0) === $blue, imagecolorat($fill, 4, 9) === $blue);
var_dump(imagecolorat($fill, 5, 5) === $blue);
var_dump(imagecolorat($fill, 6, 5) === $blue);

// A start outside the image fills nothing and still answers true.
var_dump(imagefill($fill, 99, 99, $blue));
echo $black === 0 ? "black-is-zero\n" : "black-is-not-zero\n";
---
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(true)
bool(false)
bool(false)
bool(true)
black-is-zero
