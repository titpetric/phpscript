package gd

import (
	"fmt"
	"image/color"

	"github.com/titpetric/phpscript/runner"
)

// A colour identifier is the packed value libgd uses for a true colour image:
// 0xAARRGGBB, with alpha in the top byte on GD's own scale of 0 to 127 where 0
// is opaque and 127 is transparent.
//
// That scale is inverted from every other alpha in Go, and it is the one thing
// in this package a caller gets wrong by assuming. It is kept because ported
// code does arithmetic on the value: imagecolorallocatealpha($im,230,230,230,70)
// has to mean the same 55% here as it did under GD.
const (
	alphaOpaque      = 0
	alphaTransparent = 127
)

func registerColor(rt *runner.Runtime) {
	// imagecolorallocate returns an opaque colour identifier for $red, $green and $blue, each 0 to 255.
	rt.RegisterFunc("imagecolorallocate", func(im *Image, red, green, blue int64) (any, error) {
		if im == nil {
			return false, nil
		}
		if err := checkRGB("imagecolorallocate", red, green, blue); err != nil {
			return false, err
		}
		return packColor(red, green, blue, alphaOpaque), nil
	})

	// imagecolorat returns the packed colour identifier of the pixel at $x, $y.
	rt.RegisterFunc("imagecolorat", func(im *Image, x, y int64) any {
		if im == nil || im.m == nil {
			return false
		}
		if !image_ptIn(im, x, y) {
			return false
		}
		c := color.RGBAModel.Convert(im.m.At(int(x), int(y))).(color.RGBA)
		return packColor(int64(c.R), int64(c.G), int64(c.B), alphaFromByte(c.A))
	})

	// imagecolorsforindex splits the packed identifier $color into an array with red, green, blue and alpha keys.
	rt.RegisterFunc("imagecolorsforindex", func(_ *Image, packed int64) map[string]any {
		r, g, b, a := unpackColor(packed)
		return map[string]any{
			"red":   int64(r),
			"green": int64(g),
			"blue":  int64(b),
			"alpha": int64(a),
		}
	})
}

// checkRGB raises the ValueError PHP raises for a component outside 0 to 255.
// Clamping instead would answer a colour for an argument PHP refuses, which
// hides the mistake rather than reporting it.
func checkRGB(fn string, red, green, blue int64) error {
	for i, v := range []int64{red, green, blue} {
		if v < 0 || v > 255 {
			return fmt.Errorf("%s(): Argument #%d ($%s) must be between 0 and 255 (inclusive)",
				fn, i+2, [...]string{"red", "green", "blue"}[i])
		}
	}
	return nil
}

func packColor(r, g, b, a int64) int64 {
	return clampComponent(a, alphaTransparent)<<24 |
		clampComponent(r, 255)<<16 |
		clampComponent(g, 255)<<8 |
		clampComponent(b, 255)
}

func unpackColor(packed int64) (r, g, b, a uint8) {
	return uint8(packed >> 16 & 0xff),
		uint8(packed >> 8 & 0xff),
		uint8(packed & 0xff),
		uint8(packed >> 24 & 0x7f)
}

func clampComponent(v, max int64) int64 {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// rgba turns a packed identifier into a Go colour, converting GD's 0..127
// alpha into the 0..255 the standard library wants and flipping its sense.
func rgba(packed int64) color.RGBA {
	r, g, b, a := unpackColor(packed)
	return color.RGBA{R: r, G: g, B: b, A: byteFromAlpha(a)}
}

// byteFromAlpha maps GD's 0 (opaque) to 127 (transparent) onto 255 to 0.
func byteFromAlpha(a uint8) uint8 {
	if a >= alphaTransparent {
		return 0
	}
	return uint8(255 - (int(a) * 255 / alphaTransparent))
}

// alphaFromByte is the inverse, for reading a pixel back out.
func alphaFromByte(a uint8) int64 {
	return int64(alphaTransparent - (int(a) * alphaTransparent / 255))
}

func image_ptIn(im *Image, x, y int64) bool {
	b := im.m.Bounds()
	return int(x) >= b.Min.X && int(x) < b.Max.X && int(y) >= b.Min.Y && int(y) < b.Max.Y
}

// blendPixel writes a packed colour over what is already there, honouring its
// alpha. An opaque colour replaces the pixel; a partly transparent one mixes,
// which is what imagealphablending being on means in PHP.
func blendPixel(im *Image, x, y int, packed int64) {
	c := rgba(packed)
	if c.A == 255 {
		im.m.Set(x, y, c)
		return
	}
	if c.A == 0 {
		return
	}
	dst := color.RGBAModel.Convert(im.m.At(x, y)).(color.RGBA)
	a := float64(c.A) / 255
	im.m.Set(x, y, color.RGBA{
		R: mix(c.R, dst.R, a),
		G: mix(c.G, dst.G, a),
		B: mix(c.B, dst.B, a),
		A: 255,
	})
}

func mix(src, dst uint8, a float64) uint8 {
	return uint8(float64(src)*a + float64(dst)*(1-a) + 0.5)
}
