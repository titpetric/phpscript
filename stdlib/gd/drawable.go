package gd

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// Drawable is an image this package can write a pixel into. A handle wraps one,
// and a decoder that produced something else is copied into one by wrap.
type Drawable = draw.Image

// fillRect paints r in one colour, replacing what was under it.
func fillRect(dst Drawable, r image.Rectangle, c color.Color) {
	draw.Draw(dst, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// copyPixels copies src into dst, reading src from at.
func copyPixels(dst Drawable, src image.Image, at image.Point) {
	draw.Draw(dst, dst.Bounds(), src, at, draw.Src)
}

// resample scales the sr region of src into the dr region of dst.
//
// CatmullRom is the interpolation imagecopyresampled is read as: GD's own
// resample is bicubic, and nearest neighbour would answer a different image for
// every scaled fixture.
func resample(dst Drawable, dr image.Rectangle, src image.Image, sr image.Rectangle) {
	draw.CatmullRom.Scale(dst, dr, src, sr, draw.Src, nil)
}
