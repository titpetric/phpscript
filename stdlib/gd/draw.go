package gd

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"

	"github.com/titpetric/phpscript/runner"
)

func registerDraw(rt *runner.Runtime) {
	// imagecopyresampled copies a $src_w by $src_h region of $src at $src_x, $src_y into $dst at $dst_x, $dst_y, scaled to $dst_w by $dst_h, interpolating as it goes.
	rt.RegisterFunc("imagecopyresampled", func(dst, src *Image, dstX, dstY, srcX, srcY, dstW, dstH, srcW, srcH int64) bool {
		return copyResampled(dst, src, dstX, dstY, srcX, srcY, dstW, dstH, srcW, srcH, draw.CatmullRom)
	})

	// imageline draws a line from $x1, $y1 to $x2, $y2 in $color.
	rt.RegisterFunc("imageline", func(im *Image, x1, y1, x2, y2, packed int64) bool {
		if im == nil || im.m == nil {
			return false
		}
		line(im, int(x1), int(y1), int(x2), int(y2), packed)
		return true
	})

	// imagefilledrectangle fills the rectangle with corners $x1, $y1 and $x2, $y2 in $color.
	rt.RegisterFunc("imagefilledrectangle", func(im *Image, x1, y1, x2, y2, packed int64) bool {
		if im == nil || im.m == nil {
			return false
		}
		r := normalise(int(x1), int(y1), int(x2), int(y2)).Intersect(im.m.Bounds())
		c := rgba(packed)
		if c.A == 255 {
			draw.Draw(im.m, r, &image.Uniform{c}, image.Point{}, draw.Src)
			return true
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				blendPixel(im, x, y, packed)
			}
		}
		return true
	})

	// imagefill flood fills from $x, $y in $color, replacing the connected region that shares the starting pixel's colour.
	rt.RegisterFunc("imagefill", func(im *Image, x, y, packed int64) bool {
		if im == nil || im.m == nil {
			return false
		}
		// A starting point outside the image fills nothing and still answers
		// true, which is what GD does: there is no region to walk, and the
		// call is not an error.
		if !image_ptIn(im, x, y) {
			return true
		}
		floodFill(im, int(x), int(y), packed)
		return true
	})
}

func copyResampled(dst, src *Image, dstX, dstY, srcX, srcY, dstW, dstH, srcW, srcH int64, scaler draw.Scaler) bool {
	if dst == nil || src == nil || dst.m == nil || src.m == nil {
		return false
	}
	// A degenerate rectangle copies nothing and answers true, as GD does. The
	// handles were valid and the call is not an error; there is no area.
	if dstW <= 0 || dstH <= 0 || srcW <= 0 || srcH <= 0 {
		return true
	}
	sb := src.m.Bounds()
	sr := image.Rect(
		sb.Min.X+int(srcX), sb.Min.Y+int(srcY),
		sb.Min.X+int(srcX+srcW), sb.Min.Y+int(srcY+srcH),
	).Intersect(sb)
	if sr.Empty() {
		return true
	}
	dr := image.Rect(int(dstX), int(dstY), int(dstX+dstW), int(dstY+dstH))
	scaler.Scale(dst.m, dr, src.m, sr, draw.Src, nil)
	return true
}

func normalise(x1, y1, x2, y2 int) image.Rectangle {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	// GD rectangles include the far corner; Go's exclude it.
	return image.Rect(x1, y1, x2+1, y2+1)
}

// line is Bresenham. The standard library draws no lines, and the axis aligned
// shortcut is not enough: imagerectangle and any ported border code reach for
// arbitrary endpoints.
func line(im *Image, x1, y1, x2, y2 int, packed int64) {
	dx := abs(x2 - x1)
	dy := -abs(y2 - y1)
	sx, sy := 1, 1
	if x1 >= x2 {
		sx = -1
	}
	if y1 >= y2 {
		sy = -1
	}
	err := dx + dy
	for {
		if image_ptIn(im, int64(x1), int64(y1)) {
			blendPixel(im, x1, y1, packed)
		}
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// floodFill is the scanline flood fill imagefill needs. Anything simpler only
// covers filling a blank image from its corner, which is one of the two ways
// ported code uses it.
func floodFill(im *Image, x, y int, packed int64) {
	b := im.m.Bounds()
	target := color.RGBAModel.Convert(im.m.At(x, y)).(color.RGBA)
	fill := rgba(packed)
	if target == fill {
		return
	}

	match := func(px, py int) bool {
		return color.RGBAModel.Convert(im.m.At(px, py)).(color.RGBA) == target
	}

	stack := []image.Point{{x, y}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !match(p.X, p.Y) {
			continue
		}

		// Walk left and right to the ends of this run.
		left := p.X
		for left-1 >= b.Min.X && match(left-1, p.Y) {
			left--
		}
		right := p.X
		for right+1 < b.Max.X && match(right+1, p.Y) {
			right++
		}

		for px := left; px <= right; px++ {
			im.m.Set(px, p.Y, fill)
		}

		// Seed the rows above and below, one point per contiguous run.
		for _, ny := range []int{p.Y - 1, p.Y + 1} {
			if ny < b.Min.Y || ny >= b.Max.Y {
				continue
			}
			inRun := false
			for px := left; px <= right; px++ {
				if match(px, ny) {
					if !inRun {
						stack = append(stack, image.Point{px, ny})
						inRun = true
					}
				} else {
					inRun = false
				}
			}
		}
	}
}
