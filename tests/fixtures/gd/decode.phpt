name: getimagesize and the format decoders read the fixtures
description: >
  getimagesize reports width, height, an IMAGETYPE_ constant and the ready made
  img tag without decoding the pixels. The imagecreatefrom family is format
  specific the way PHP's is: naming the wrong decoder for a file answers false
  rather than quietly succeeding, which is what lets ported code branch on the
  return. imagecreatefromstring sniffs instead, and a file that is not an image
  is false everywhere.
---
<?php

foreach (array("landscape.jpg", "portrait.png", "square.gif") as $name) {
	$info = getimagesize("testdata/" . $name);
	echo $name, " ", $info[0], "x", $info[1], " type=", $info[2], " ", $info[3], "\n";
}

echo IMAGETYPE_GIF, IMAGETYPE_JPEG, IMAGETYPE_PNG, "\n";

$jpg = imagecreatefromjpeg("testdata/landscape.jpg");
$png = imagecreatefrompng("testdata/portrait.png");
$gif = imagecreatefromgif("testdata/square.gif");
echo imagesx($jpg), " ", imagesx($png), " ", imagesx($gif), "\n";

// A decoder is not a sniffer: the wrong one refuses.
var_dump(imagecreatefrompng("testdata/landscape.jpg"));
var_dump(imagecreatefromjpeg("testdata/square.gif"));

var_dump(getimagesize("testdata/nope.jpg"));
var_dump(imagecreatefromjpeg("testdata/nope.jpg"));
---
landscape.jpg 640x400 type=2 width="640" height="400"
portrait.png 300x480 type=3 width="300" height="480"
square.gif 256x256 type=1 width="256" height="256"
123
640 300 256
bool(false)
bool(false)
bool(false)
bool(false)
