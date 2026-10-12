package gd

import (
	"image"
	"image/color"
	"testing"
)

// TestFillRect covers the uniform fill imagefilledrectangle and
// imagecreatetruecolor both go through: the named rectangle takes the colour
// and the pixel outside it is left alone.
func TestFillRect(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 3, 3))
	red := color.RGBA{R: 255, A: 255}
	fillRect(m, image.Rect(0, 0, 2, 2), red)

	if got := m.RGBAAt(1, 1); got != red {
		t.Errorf("inside the rectangle, pixel 1,1 = %v, want %v", got, red)
	}
	if got := m.RGBAAt(2, 2); got != (color.RGBA{}) {
		t.Errorf("outside the rectangle, pixel 2,2 = %v, want the zero colour", got)
	}
}

// TestCopyPixels covers the copy a decoded image takes when it cannot be drawn
// on, including the offset: src is read from at, so a source whose bounds do
// not start at the origin lands at the destination's origin.
func TestCopyPixels(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 4, 6, 6))
	blue := color.RGBA{B: 255, A: 255}
	src.SetRGBA(4, 4, blue)

	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	copyPixels(dst, src, src.Bounds().Min)

	if got := dst.RGBAAt(0, 0); got != blue {
		t.Errorf("pixel 0,0 = %v, want %v", got, blue)
	}
}

// TestResample covers the scale imagecopyresampled performs. A single colour
// scaled up stays that colour whatever the interpolation does between
// neighbours, so the assertion holds without pinning CatmullRom's weights.
func TestResample(t *testing.T) {
	green := color.RGBA{G: 255, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fillRect(src, src.Bounds(), green)

	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	resample(dst, dst.Bounds(), src, src.Bounds())

	for _, p := range []image.Point{{0, 0}, {3, 3}, {2, 1}} {
		if got := dst.RGBAAt(p.X, p.Y); got != green {
			t.Errorf("pixel %d,%d = %v, want %v", p.X, p.Y, got, green)
		}
	}
}

// TestDrawable covers the alias the handle is declared through, so the type a
// host reads off Underlying is the one this package writes into.
func TestDrawable(t *testing.T) {
	var d Drawable = image.NewRGBA(image.Rect(0, 0, 1, 1))
	im := &Image{m: d}

	if got := im.Underlying(); got != d {
		t.Errorf("Underlying() = %v, want the drawable the handle was built on", got)
	}
	if got := im.Bounds(); got != d.Bounds() {
		t.Errorf("Bounds() = %v, want %v", got, d.Bounds())
	}
}
