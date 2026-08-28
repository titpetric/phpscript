name: imagecopyresampled scales a source region into a destination region
description: >
  It takes the destination point first, then the source point, then the two
  sizes, and interpolates between them. A crop is the same call with a source
  rectangle smaller than the image.
---
<?php

$src = imagecreatefromjpeg("testdata/landscape.jpg");

$half = imagecreatetruecolor(320, 200);
var_dump(imagecopyresampled($half, $src, 0, 0, 0, 0, 320, 200, 640, 400));
echo imagesx($half), "x", imagesy($half), "\n";

// A crop: the source rectangle is a window, the destination fills.
$crop = imagecreatetruecolor(120, 80);
var_dump(imagecopyresampled($crop, $src, 0, 0, 20, 0, 120, 80, 600, 400));
echo imagesx($crop), "x", imagesy($crop), "\n";

// The top left of the fixtures is a solid red marker, so a crop that starts
// at the origin keeps it and one that starts past it does not.
$corner = imagecreatetruecolor(20, 20);
imagecopyresampled($corner, $src, 0, 0, 0, 0, 20, 20, 40, 40);
$parts = imagecolorsforindex($corner, imagecolorat($corner, 10, 10));
echo $parts['red'] > 200 && $parts['green'] < 80 ? "marker-kept\n" : "marker-lost\n";

$inside = imagecreatetruecolor(20, 20);
imagecopyresampled($inside, $src, 0, 0, 300, 200, 20, 20, 40, 40);
$parts = imagecolorsforindex($inside, imagecolorat($inside, 10, 10));
echo $parts['red'] > 200 && $parts['green'] < 80 ? "marker-kept\n" : "marker-lost\n";

// A degenerate rectangle copies nothing and answers true.
var_dump(imagecopyresampled($half, $src, 0, 0, 0, 0, 0, 200, 640, 400));
---
bool(true)
320x200
bool(true)
120x80
marker-kept
marker-lost
bool(true)
