name: the writers encode to a file or to the output
description: >
  imagejpeg and imagepng take a filename, or write to the script's output when
  it is null, which is how ported code streams an image without a temporary
  file. Each round trip is checked by reading the file back with getimagesize,
  so the assertion is on the bytes rather than on the handle that produced
  them.
---
<?php

$im = imagecreatetruecolor(40, 25);
$red = imagecolorallocate($im, 200, 30, 30);
imagefilledrectangle($im, 0, 0, 39, 24, $red);

var_dump(imagejpeg($im, "testdata/out.jpg", 88));
var_dump(imagepng($im, "testdata/out.png"));

foreach (array("out.jpg", "out.png") as $name) {
	$info = getimagesize("testdata/" . $name);
	echo $name, " ", $info[0], "x", $info[1], " type=", $info[2], "\n";
}

// A null filename writes to the output. Buffer it so the fixture can measure
// the payload instead of printing it. PHP 8 takes only null here; these
// bindings accept the empty string a PHP 5 port would have written too.
ob_start();
imagepng($im, null);
$png = ob_get_clean();
echo strlen($png) > 0 ? "streamed\n" : "empty\n";
echo substr($png, 1, 3), "\n";

unlink("testdata/out.jpg");
unlink("testdata/out.png");
---
bool(true)
bool(true)
out.jpg 40x25 type=2
out.png 40x25 type=3
streamed
PNG
