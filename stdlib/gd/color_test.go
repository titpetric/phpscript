package gd_test

import (
	"strings"
	"testing"
)

// TestColorAllocateRoundTrip covers the packed identifier a script carries in a
// variable. A colour is 0xAARRGGBB, so allocating one, painting with it and
// reading the pixel back answers the same integer, and a script can compare two
// colours with ==.
func TestColorAllocateRoundTrip(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, `<?php
$im = imagecreatetruecolor(2, 2);
$red = imagecolorallocate($im, 255, 0, 0);
$grey = imagecolorallocate($im, 10, 20, 30);
imagefilledrectangle($im, 0, 0, 0, 0, $red);
imagefilledrectangle($im, 1, 1, 1, 1, $grey);
echo $red . " " . $grey . " " . imagecolorat($im, 0, 0) . " " . imagecolorat($im, 1, 1);
`)
	// 16711680 is 0xFF0000 and 660510 is 0x0A141E.
	if want := "16711680 660510 16711680 660510"; out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestColorsForIndexSplitsThePackedValue covers the four components a script
// reads back out of an identifier. Alpha is on GD's own 0 to 127 scale, where 0
// is opaque and 127 is transparent, so ported code that computes on the number
// keeps the same meaning it had under GD.
func TestColorsForIndexSplitsThePackedValue(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		packed string
		want   string
	}{
		{name: "opaque black", packed: "0", want: "0 0 0 0"},
		{name: "opaque red", packed: "0xFF0000", want: "255 0 0 0"},
		{name: "half transparent", packed: "0x40FF8000", want: "255 128 0 64"},
		{name: "fully transparent", packed: "0x7F000000", want: "0 0 0 127"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, out := runScript(t, `<?php
$im = imagecreatetruecolor(1, 1);
$c = imagecolorsforindex($im, `+test.packed+`);
echo $c["red"] . " " . $c["green"] . " " . $c["blue"] . " " . $c["alpha"];
`)
			if out != test.want {
				t.Errorf("echoed %q, want %q", out, test.want)
			}
		})
	}
}

// TestColorAlphaBlends covers what alpha does when blendPixel writes a colour.
// An opaque colour replaces the pixel and a fully transparent one leaves it
// alone, the behaviour a PHP script sees with imagealphablending on.
func TestColorAlphaBlends(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, `<?php
$im = imagecreatetruecolor(1, 1);
$red = imagecolorallocate($im, 255, 0, 0);
imagefilledrectangle($im, 0, 0, 0, 0, $red);
echo imagecolorat($im, 0, 0);

// 0x7F0000FF is blue at alpha 127, GD's fully transparent.
imagefilledrectangle($im, 0, 0, 0, 0, 0x7F0000FF);
echo " " . imagecolorat($im, 0, 0);
`)
	if want := "16711680 16711680"; out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestColorAtOutsideTheImage covers a coordinate the image does not hold.
// GD answers false, so a script that walks past an edge sees a value it can
// test and never a colour the image does not hold.
func TestColorAtOutsideTheImage(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, `<?php
$im = imagecreatetruecolor(2, 2);
var_export(imagecolorat($im, 2, 0));
var_export(imagecolorat($im, 0, -1));
var_export(imagecolorat($im, 1, 1));
`)
	if want := "falsefalse0"; out != want {
		t.Errorf("echoed %q, want %q", out, want)
	}
}

// TestColorAllocateRejectsAComponentOutOfRange covers the ValueError PHP 8
// raises for a component outside 0 to 255. Clamping would answer a colour for
// an argument the engine refuses, and the script would carry on past the
// mistake.
func TestColorAllocateRejectsAComponentOutOfRange(t *testing.T) {
	t.Parallel()
	_, out := runScript(t, `<?php
$im = imagecreatetruecolor(1, 1);
try {
	imagecolorallocate($im, 300, 0, 0);
	echo "no error";
} catch (Throwable $e) {
	echo $e->getMessage();
}
`)
	if !strings.Contains(out, "must be between 0 and 255") {
		t.Errorf("echoed %q, want a message naming the range", out)
	}
	if !strings.Contains(out, "$red") {
		t.Errorf("echoed %q, want a message naming the argument", out)
	}
}
