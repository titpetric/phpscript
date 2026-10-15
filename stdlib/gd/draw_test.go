package gd_test

import (
	"testing"
)

// gridHelper is a PHP function the drawing tests echo their result through. A
// grid of packed colours reads as one string, so a test names the whole image
// it expected and the failure shows which pixel moved.
const gridHelper = `<?php
function grid($im, $w, $h) {
	$out = "";
	for ($y = 0; $y < $h; $y++) {
		for ($x = 0; $x < $w; $x++) {
			$out .= ($x ? "," : "") . imagecolorat($im, $x, $y);
		}
		$out .= ";";
	}
	return $out;
}
`

// TestDrawFilledRectangle covers the corner convention. GD rectangles include
// the far corner where Go's exclude it, so a fill from 0,0 to 1,1 paints four
// pixels and a ported script keeps the area it drew under GD.
func TestDrawFilledRectangle(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(3, 3);
$red = imagecolorallocate($im, 255, 0, 0);
var_export(imagefilledrectangle($im, 0, 0, 1, 1, $red));
echo grid($im, 3, 3);
`)
	want := "true" +
		"16711680,16711680,0;" +
		"16711680,16711680,0;" +
		"0,0,0;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestDrawFilledRectangleNormalisesCorners covers a rectangle named from its
// far corner back. GD accepts either order, so a script computing coordinates
// does not have to sort them.
func TestDrawFilledRectangleNormalisesCorners(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(2, 2);
$red = imagecolorallocate($im, 255, 0, 0);
imagefilledrectangle($im, 1, 1, 0, 0, $red);
echo grid($im, 2, 2);
`)
	want := "16711680,16711680;16711680,16711680;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestDrawLine covers the Bresenham walk. The standard library draws no lines
// and an axis-aligned shortcut is not enough, because ported border code names
// arbitrary endpoints, so both an axis-aligned and a diagonal line are read
// back.
func TestDrawLine(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		call string
		want string
	}{
		{
			name: "horizontal",
			call: "imageline($im, 0, 0, 2, 0, $red)",
			want: "16711680,16711680,16711680;0,0,0;0,0,0;",
		},
		{
			name: "vertical",
			call: "imageline($im, 1, 0, 1, 2, $red)",
			want: "0,16711680,0;0,16711680,0;0,16711680,0;",
		},
		{
			name: "diagonal",
			call: "imageline($im, 0, 0, 2, 2, $red)",
			want: "16711680,0,0;0,16711680,0;0,0,16711680;",
		},
		{
			name: "reversed diagonal",
			call: "imageline($im, 2, 2, 0, 0, $red)",
			want: "16711680,0,0;0,16711680,0;0,0,16711680;",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(3, 3);
$red = imagecolorallocate($im, 255, 0, 0);
var_export(`+test.call+`);
echo grid($im, 3, 3);
`)
			if out != "true"+test.want {
				t.Errorf("echoed %q, want %q", out, "true"+test.want)
			}
		})
	}
}

// TestDrawLineClipsToTheImage covers endpoints past an edge. GD draws the part
// that lands and reports success, so a script computing coordinates from input
// does not have to clip them itself.
func TestDrawLineClipsToTheImage(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(2, 2);
$red = imagecolorallocate($im, 255, 0, 0);
var_export(imageline($im, -5, 0, 5, 0, $red));
echo grid($im, 2, 2);
`)
	want := "true16711680,16711680;0,0;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestDrawFill covers the scanline flood fill. It replaces the connected region
// sharing the starting pixel's colour, so filling a blank image from a corner
// paints all of it and a region walled off by another colour is left.
func TestDrawFill(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(3, 3);
$red = imagecolorallocate($im, 255, 0, 0);
$blue = imagecolorallocate($im, 0, 0, 255);

// A wall down the middle column leaves two regions.
imageline($im, 1, 0, 1, 2, $blue);
var_export(imagefill($im, 0, 0, $red));
echo grid($im, 3, 3);
`)
	want := "true" +
		"16711680,255,0;" +
		"16711680,255,0;" +
		"16711680,255,0;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestDrawFillOutsideTheImage covers a starting point the image does not hold.
// There is no region to walk, and GD treats the call as done and never
// failed, so the return stays true and nothing is painted.
func TestDrawFillOutsideTheImage(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$im = imagecreatetruecolor(2, 2);
$red = imagecolorallocate($im, 255, 0, 0);
var_export(imagefill($im, 9, 9, $red));
echo grid($im, 2, 2);
`)
	want := "true0,0;0,0;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestDrawCopyResampled covers the resize every ported thumbnail path runs. A
// uniform source scales to a uniform destination, and a degenerate rectangle
// copies nothing and still reports success: the handles are valid and there is
// no area.
func TestDrawCopyResampled(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, gridHelper+`
$src = imagecreatetruecolor(2, 2);
$red = imagecolorallocate($src, 255, 0, 0);
imagefilledrectangle($src, 0, 0, 1, 1, $red);

$dst = imagecreatetruecolor(4, 4);
var_export(imagecopyresampled($dst, $src, 0, 0, 0, 0, 4, 4, 2, 2));
echo grid($dst, 4, 4);

$empty = imagecreatetruecolor(2, 2);
var_export(imagecopyresampled($empty, $src, 0, 0, 0, 0, 0, 0, 2, 2));
echo grid($empty, 2, 2);
`)
	want := "true" +
		"16711680,16711680,16711680,16711680;" +
		"16711680,16711680,16711680,16711680;" +
		"16711680,16711680,16711680,16711680;" +
		"16711680,16711680,16711680,16711680;" +
		"true0,0;0,0;"
	if out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}
