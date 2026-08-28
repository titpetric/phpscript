name: Common\Image drives the bindings end to end
description: >
  A real image class from a ported application, running on these bindings. It is
  the integration check the individual fixtures cannot be: load dispatches to
  loadjpg through a variable method name, resize picks its own dimensions and
  calls imagecopyresampled through resizeCopy, mkcolor goes through hexdec, and
  the writers are reached as save/savepng/get/getpng.

  Two things are trimmed from the original: the GD version probe, which asked
  function_exists what this build supports, and the $version field it set,
  which chose between imagecreate and imagecreatetruecolor and between the two
  copies. There is one of each here, so the branch had nothing to pick.

  It is not compared against the reference runtime. get() and getpng() call
  imagejpeg($im, "") to stream, which is the PHP 5 spelling; PHP 8 raises
  "Path must not be empty" and takes only null. These bindings accept both, so
  the class runs here and fatals there, and the difference is the class's age
  rather than a divergence in the bindings.
runner:
  php: false
---
<?php

include "Image.php";

use Common\Image;

$im = new Image();
var_dump($im->load("testdata/landscape.jpg"));

// resize() without $aspect fits the width and keeps the ratio.
$im->resize(320);
echo $im->new_width, "x", $im->new_height, "\n";

$im->save("testdata/o.jpg", 88);
$saved = getimagesize("testdata/o.jpg");
echo "saved ", $saved[0], "x", $saved[1], " type=", $saved[2], "\n";

// A width the image already fits inside is left alone.
$im->resize(1000);
echo $im->new_width, "x", $im->new_height, "\n";

// resizeH scales by height, and does grow the image.
$im->resizeH(800);
echo $im->new_width, "x", $im->new_height, "\n";

$im->auto_crop(120, 80);
echo $im->new_width, "x", $im->new_height, "\n";

$im->auto_crop_top(120, 80);
echo $im->new_width, "x", $im->new_height, "\n";

// thumbnail() lays the image on a square plate: size plus twice the margin.
// It never writes new_width/new_height, so those still read what auto_crop
// left; the plate size is checked through the file it writes below.
$im->thumbnail("#101010", "#FFFFFF", 120, 5, true, true);
echo $im->new_width, "x", $im->new_height, "\n";

$im->savepng("testdata/o.png");
$png = getimagesize("testdata/o.png");
echo "thumb ", $png[0], "x", $png[1], " type=", $png[2], "\n";

// mkcolor and blend read a hex string through hexdec.
$plate = $im->create(10, 10);
echo $im->mkcolor($plate, "#FF0000"), "\n";
echo $im->mkcolor($plate, "#FF0000", true), "\n";
echo $im->blend($plate, "#000000", "#FFFFFF", 0.5), "\n";

// border() paints the working image and leaves its size alone.
$im->border("#FF00FF", 3);
echo $im->new_width, "x", $im->new_height, "\n";

// get() and getpng() buffer the writers instead of naming a file.
echo strlen($im->get(70)) > 0 ? "jpeg-bytes\n" : "no-jpeg-bytes\n";
echo substr($im->getpng(), 1, 3), "\n";

// A file that is not an image is refused before any handle is made.
$bad = new Image();
var_dump($bad->load("testdata/nope.jpg"));

$im->destroy();
unlink("testdata/o.jpg");
unlink("testdata/o.png");
---
bool(true)
320x200
saved 320x200 type=2
640x400
1280x800
120x80
120x80
120x80
thumb 130x130 type=3
16711680
65535
8355711
120x80
jpeg-bytes
PNG
bool(false)
