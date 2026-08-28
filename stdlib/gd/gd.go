// Package gd provides PHP's image functions over the standard library's image
// packages and golang.org/x/image.
//
// The surface is the one a ported image class actually calls: decode, create,
// resample, draw, write. It is not all of ext/gd, and it is not everything
// ext/gd would let you register: a binding is here because something needs it,
// and the text family is absent because nothing did.
//
// imagetypes is answered rather than omitted because ported code branches on
// it, but there is one decoder set behind it and it always reports the same
// three formats.
//
// A GD image reaches a script as a *Image, the way fopen hands back a stream.
// A colour reaches it as an int packed the way libgd packs one,
// 0xAARRGGBB with alpha in the top byte, so a script can hold a colour in a
// variable and pass it to any drawing call.
package gd

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"

	_ "image/gif"

	"golang.org/x/image/draw"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
)

// Image is a GD image handle. A script never looks inside it; it holds one and
// passes it back to the drawing and writing functions.
type Image struct {
	m draw.Image
	// truecolor records which constructor made it. imagecreate produced a
	// palette image in PHP and imagecreatetruecolor did not, and code that
	// branches on the difference reads it back through imageistruecolor.
	truecolor bool
}

// Bounds is the image rectangle. It is exported so a host embedding this
// package can inspect what a script produced.
func (im *Image) Bounds() image.Rectangle { return im.m.Bounds() }

// Underlying returns the drawable the handle wraps.
func (im *Image) Underlying() draw.Image { return im.m }

// PHP's IMG_* format bitmask, as libgd numbers them.
const (
	imgBMP       = 64
	imgGIF       = 1
	imgJPEG      = 2
	imgPNG       = 4
	imgWBMP      = 8
	imgWEBP      = 32
	imgXPM       = 16
	imgAVIF      = 128
	imgTIFF      = 256
	imgSupported = imgGIF | imgJPEG | imgPNG
)

// PHP's IMAGETYPE_* constants, the third element of getimagesize.
const (
	typeGIF  = 1
	typeJPEG = 2
	typePNG  = 3
	typeBMP  = 6
	typeWEBP = 18
)

// init contributes the image bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the image functions and the IMG_* and IMAGETYPE_*
// constants, rooted at the process working directory the way the filesystem
// shims are.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the image functions rooted at dir. Reads resolve
// against it and cannot climb out; writes are additionally held to the
// runtime's writable_paths, matching stdlib/files.
func RegisterRoot(rt *runner.Runtime, dir string) {
	r := root{dir: dir, writable: files.WritableRoots(dir, rt.WritablePaths())}

	registerConstants(rt)
	registerCreate(rt, r)
	registerInfo(rt, r)
	registerColor(rt)
	registerDraw(rt)
	registerWrite(rt, r)
}

func registerConstants(rt *runner.Runtime) {
	for name, v := range map[string]int64{
		"IMG_GIF":  imgGIF,
		"IMG_JPG":  imgJPEG,
		"IMG_JPEG": imgJPEG,
		"IMG_PNG":  imgPNG,
		"IMG_WBMP": imgWBMP,
		"IMG_XPM":  imgXPM,
		"IMG_WEBP": imgWEBP,
		"IMG_BMP":  imgBMP,
		"IMG_AVIF": imgAVIF,
		"IMG_TIFF": imgTIFF,

		"IMAGETYPE_GIF":  typeGIF,
		"IMAGETYPE_JPEG": typeJPEG,
		"IMAGETYPE_PNG":  typePNG,
		"IMAGETYPE_BMP":  typeBMP,
		"IMAGETYPE_WEBP": typeWEBP,
	} {
		rt.SetConst(name, v)
	}
}

// root maps a path a script supplied onto the host filesystem, the same way
// stdlib/files does, so an image read and a file read agree on what a relative
// path means.
type root struct {
	dir      string
	writable []string
}

func (r root) resolve(p string) string {
	if filepath.IsAbs(p) {
		return path.Clean(p)
	}
	clean := path.Clean("/" + filepath.ToSlash(p))
	return filepath.Join(r.dir, filepath.FromSlash(clean))
}

// resolveWrite is resolve for a path about to be written. A path outside
// writable_paths is an error, not a false return, matching stdlib/files: a
// refused write is a configuration the script was never allowed to escape.
func (r root) resolveWrite(fn, p string) (string, error) {
	name := r.resolve(p)
	if len(r.writable) == 0 {
		return name, nil
	}
	for _, w := range r.writable {
		if files.Within(name, w) {
			return name, nil
		}
	}
	return "", fmt.Errorf("%s(): %s is outside writable_paths", fn, p)
}

// ---------------------------------------------------------------------------
// Creating and decoding
// ---------------------------------------------------------------------------

func registerCreate(rt *runner.Runtime, r root) {
	// imagecreatetruecolor returns a new true colour image of $width by $height, filled with opaque black.
	rt.RegisterFunc("imagecreatetruecolor", func(width, height int64) (any, error) {
		if err := checkDimensions("imagecreatetruecolor", width, height); err != nil {
			return false, err
		}
		m := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
		draw.Draw(m, m.Bounds(), &image.Uniform{color.RGBA{A: 255}}, image.Point{}, draw.Src)
		return &Image{m: m, truecolor: true}, nil
	})

	// imagecreatefromjpeg decodes $filename as JPEG and returns an image, or false when it cannot be read.
	rt.RegisterFunc("imagecreatefromjpeg", decoder(r, "jpeg"))
	// imagecreatefrompng decodes $filename as PNG and returns an image, or false when it cannot be read.
	rt.RegisterFunc("imagecreatefrompng", decoder(r, "png"))
	// imagecreatefromgif decodes $filename as GIF and returns an image, or false when it cannot be read.
	rt.RegisterFunc("imagecreatefromgif", decoder(r, "gif"))
	// imagedestroy frees $image. Memory is reclaimed automatically here, so it only drops the pixels and returns true.
	rt.RegisterFunc("imagedestroy", func(im *Image) bool {
		if im == nil {
			return false
		}
		im.m = nil
		return true
	})
}

// decoder builds imagecreatefrom<format>. PHP's decoders are format specific
// and this keeps that contract: naming the wrong one is an error, not a
// silent success, so a script that branches on the return sees what PHP sees.
func decoder(r root, want string) func(string) any {
	return func(filename string) any {
		f, err := os.Open(r.resolve(filename))
		if err != nil {
			return false
		}
		defer f.Close()
		src, format, err := image.Decode(f)
		if err != nil || format != want {
			return false
		}
		return wrap(src)
	}
}

// wrap turns a decoded image into a handle, copying into RGBA when the decoder
// produced something that cannot be drawn on. Every paletted or YCbCr image
// takes this path, which is what makes a decoded JPEG or GIF writable.
func wrap(src image.Image) *Image {
	if d, ok := src.(draw.Image); ok {
		if _, paletted := src.(*image.Paletted); !paletted {
			return &Image{m: d, truecolor: true}
		}
	}
	b := src.Bounds()
	m := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), src, b.Min, draw.Src)
	_, paletted := src.(*image.Paletted)
	return &Image{m: m, truecolor: !paletted}
}

// ---------------------------------------------------------------------------
// Inspection
// ---------------------------------------------------------------------------

func registerInfo(rt *runner.Runtime, r root) {
	// imagetypes returns a bitmask of the formats this build reads and writes: IMG_GIF, IMG_JPG and IMG_PNG.
	rt.RegisterFunc("imagetypes", func() int64 { return imgSupported })

	// imagesx returns the width of $image in pixels.
	rt.RegisterFunc("imagesx", func(im *Image) any {
		if im == nil || im.m == nil {
			return false
		}
		return int64(im.m.Bounds().Dx())
	})

	// imagesy returns the height of $image in pixels.
	rt.RegisterFunc("imagesy", func(im *Image) any {
		if im == nil || im.m == nil {
			return false
		}
		return int64(im.m.Bounds().Dy())
	})

	// getimagesize returns array(width, height, IMAGETYPE_*, "width=.. height=..") for $filename, or false when it is not an image.
	rt.RegisterFunc("getimagesize", func(filename string) any {
		f, err := os.Open(r.resolve(filename))
		if err != nil {
			return false
		}
		defer f.Close()
		cfg, format, err := image.DecodeConfig(f)
		if err != nil {
			return false
		}
		// The four positional elements only. PHP adds "bits", "channels" and
		// "mime" alongside them; nothing reads those through this binding, and
		// a list keeps the indices a caller destructures by.
		return []any{
			int64(cfg.Width),
			int64(cfg.Height),
			imageTypeOf(format),
			fmt.Sprintf(`width="%d" height="%d"`, cfg.Width, cfg.Height),
		}
	})
}

func imageTypeOf(format string) int64 {
	switch format {
	case "gif":
		return typeGIF
	case "jpeg":
		return typeJPEG
	case "png":
		return typePNG
	}
	return 0
}

// ---------------------------------------------------------------------------
// Writing
// ---------------------------------------------------------------------------

func registerWrite(rt *runner.Runtime, r root) {
	// imagejpeg writes $image to $filename as JPEG at $quality (default 75), or to the output when $filename is null or empty.
	rt.RegisterFunc("imagejpeg", func(im *Image, args ...any) (any, error) {
		return encodeTo(rt, r, "imagejpeg", im, args, func(w io.Writer, m image.Image, opts []any) error {
			quality := 75
			if len(opts) > 0 {
				if q, ok := toInt(opts[0]); ok && q >= 1 && q <= 100 {
					quality = int(q)
				}
			}
			return jpeg.Encode(w, m, &jpeg.Options{Quality: quality})
		})
	})

	// imagepng writes $image to $filename as PNG, or to the output when $filename is null or empty. $quality selects the compression level.
	rt.RegisterFunc("imagepng", func(im *Image, args ...any) (any, error) {
		return encodeTo(rt, r, "imagepng", im, args, func(w io.Writer, m image.Image, opts []any) error {
			enc := png.Encoder{CompressionLevel: png.DefaultCompression}
			if len(opts) > 0 {
				if lvl, ok := toInt(opts[0]); ok && lvl >= 0 && lvl <= 9 {
					// PHP takes 0..9 where 0 is none, which is Go's
					// NoCompression, and 9 is Go's BestCompression.
					switch {
					case lvl == 0:
						enc.CompressionLevel = png.NoCompression
					case lvl >= 7:
						enc.CompressionLevel = png.BestCompression
					default:
						enc.CompressionLevel = png.BestSpeed
					}
				}
			}
			return enc.Encode(w, m)
		})
	})
}

// encodeTo is the shared body of the three writers: pick the destination the
// way PHP does, then hand the writer to the format.
func encodeTo(rt *runner.Runtime, r root, fn string, im *Image, args []any, enc func(io.Writer, image.Image, []any) error) (any, error) {
	if im == nil || im.m == nil {
		return false, nil
	}
	var (
		name    string
		toFile  bool
		options []any
	)
	if len(args) > 0 {
		if s, ok := args[0].(string); ok && s != "" {
			name, toFile = s, true
		}
		options = args[1:]
	}
	// An empty filename streams to the output. PHP 8 raises "Path must not be
	// empty" and takes only null for that, but the empty string is how PHP 4
	// and 5 code spelled it, and a port carries the older spelling; there is no
	// second reading of imagejpeg($im, "") to lose by accepting it.

	if !toFile {
		if err := enc(rt.Output(), im.m, options); err != nil {
			return false, nil
		}
		return true, nil
	}

	target, err := r.resolveWrite(fn, name)
	if err != nil {
		return false, err
	}
	f, err := os.Create(target)
	if err != nil {
		return false, nil
	}
	err = enc(f, im.m, options)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, nil
	}
	return true, nil
}

// checkDimensions raises the ValueError PHP raises for a non-positive width or
// height. PHP 8 turned what used to be a false return into a thrown error, and
// a binding that answers false instead would let a script carry on past a
// mistake the engine it is being ported from stops at.
func checkDimensions(fn string, width, height int64) error {
	if width <= 0 {
		return fmt.Errorf("%s(): Argument #1 ($width) must be greater than 0", fn)
	}
	if height <= 0 {
		return fmt.Errorf("%s(): Argument #2 ($height) must be greater than 0", fn)
	}
	return nil
}

// toInt accepts the numeric shapes a PHP argument arrives as.
func toInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
