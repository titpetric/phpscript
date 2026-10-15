package gd_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/gd"
)

// runScript runs src with the standard library installed and the image
// bindings rooted at the working directory, and returns the handle the script
// left in the IM constant together with what it echoed.
//
// A script is the only way to build an *Image: the constructors are registered
// functions and the type has no exported one.
func runScript(t *testing.T, src string) (*gd.Image, string) {
	t.Helper()
	return runScriptAt(t, ".", src)
}

// runScriptAt is runScript with the image bindings rooted at dir, for a test
// that reads a file through them.
func runScriptAt(t *testing.T, dir, src string) (*gd.Image, string) {
	t.Helper()

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	// The second registration replaces the one stdlib.Register installed at
	// the working directory, the way stdlib.RegisterFS reroots a virtual host.
	gd.RegisterRoot(rt, dir)
	rt.SetContext(context.Background())

	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}

	value, known := rt.Const("IM")
	if !known {
		return nil, out.String()
	}
	handle, ok := value.(*gd.Image)
	if !ok {
		t.Fatalf("IM is %T, want a *gd.Image", value)
	}
	return handle, out.String()
}

// writePNG writes a solid PNG of the given size at path.
func writePNG(t *testing.T, path string, width, height int) {
	t.Helper()

	m := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			m.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(file, m); err != nil {
		_ = file.Close()
		t.Fatalf("encode: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// TestRegister covers what the stdlib wiring installs. The IMG_* and
// IMAGETYPE_* constants are set unconditionally because ported code branches on
// them, and imagetypes reports one fixed set: the decoders are linked into the
// program and never discovered at runtime.
func TestRegister(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	gd.Register(rt)
	rt.SetContext(context.Background())

	for name, want := range map[string]int64{
		"IMG_GIF":        1,
		"IMG_JPEG":       2,
		"IMG_PNG":        4,
		"IMAGETYPE_GIF":  1,
		"IMAGETYPE_JPEG": 2,
		"IMAGETYPE_PNG":  3,
	} {
		value, known := rt.Const(name)
		if !known {
			t.Errorf("%s is not defined", name)
			continue
		}
		if value != want {
			t.Errorf("%s = %#v, want %d", name, value, want)
		}
	}

	program, err := rt.Load(`<?php $im = imagecreatetruecolor(4, 3); echo imagetypes() . " " . imagesx($im) . "x" . imagesy($im);`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	// 7 is IMG_GIF|IMG_JPEG|IMG_PNG, the three formats this build handles.
	if got := out.String(); got != "7 4x3" {
		t.Errorf("echoed %q, want %q", got, "7 4x3")
	}
}

// TestRegisterRoot covers the path rule the image functions share with
// stdlib/files: every path a script names is anchored inside the root,
// absolute and climbing spellings included. A path reaching the host
// filesystem would let a virtual host read a file file_get_contents refuses.
func TestRegisterRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "shot.png"), 4, 3)

	for _, spelling := range []string{"shot.png", "./shot.png", "/shot.png", "../shot.png", "../../shot.png"} {
		t.Run(spelling, func(t *testing.T) {
			_, out := runScriptAt(t, root, `<?php
$info = getimagesize(`+phpString(spelling)+`);
echo $info[0] . "x" . $info[1] . " " . $info[2];
`)
			// 3 is IMAGETYPE_PNG.
			if out != "4x3 3" {
				t.Errorf("getimagesize(%s) echoed %q, want %q", spelling, out, "4x3 3")
			}
		})
	}

	// A name nothing resolves to reports false, as PHP's own does.
	_, out := runScriptAt(t, root, `<?php var_export(getimagesize("missing.png"));`)
	if out != "false" {
		t.Errorf("getimagesize of a missing file echoed %q, want false", out)
	}
}

// TestImage covers the handle a script holds between calls. imagedestroy drops
// the pixels and keeps the handle itself valid, so the size functions report
// false afterwards where they reported a number.
func TestImage(t *testing.T) {
	t.Parallel()
	handle, out := runScript(t, `<?php
$im = imagecreatetruecolor(4, 3);
var_export(imagesx($im));
define("IM", $im);
var_export(imagedestroy($im));
var_export(imagesx($im));
`)
	if out != "4truefalse" {
		t.Errorf("echoed %q, want %q", out, "4truefalse")
	}
	if handle == nil {
		t.Fatal("the script defined no IM constant")
	}
}

// TestImage_Bounds covers the rectangle a Go host embedding this package reads
// off a handle a script produced. A created image starts at the origin, so the
// bounds are the width and height the script named.
func TestImage_Bounds(t *testing.T) {
	t.Parallel()
	handle, _ := runScript(t, `<?php define("IM", imagecreatetruecolor(7, 5));`)
	if handle == nil {
		t.Fatal("the script defined no IM constant")
	}

	if got, want := handle.Bounds(), image.Rect(0, 0, 7, 5); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

// TestImage_Underlying covers the drawable the handle wraps. A host writes
// through it, so the value is the same drawable the script's own calls paint
// on, and a pixel set here is the pixel the next call reads.
func TestImage_Underlying(t *testing.T) {
	t.Parallel()
	handle, _ := runScript(t, `<?php define("IM", imagecreatetruecolor(2, 2));`)
	if handle == nil {
		t.Fatal("the script defined no IM constant")
	}

	drawable := handle.Underlying()
	if drawable == nil {
		t.Fatal("Underlying() = nil, want the drawable the handle wraps")
	}
	if got, want := drawable.Bounds(), handle.Bounds(); got != want {
		t.Errorf("Underlying().Bounds() = %v, want the handle's %v", got, want)
	}

	// A fresh true colour image is filled with opaque black.
	if got := color.RGBAModel.Convert(drawable.At(0, 0)); got != (color.RGBA{A: 255}) {
		t.Errorf("pixel 0,0 = %#v, want opaque black", got)
	}

	drawable.Set(0, 0, color.RGBA{R: 255, A: 255})
	if got := color.RGBAModel.Convert(handle.Underlying().At(0, 0)); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("after Set, pixel 0,0 = %#v, want opaque red", got)
	}
}

// phpString renders a Go string as a PHP single-quoted literal, for a path
// spliced into a test script.
func phpString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `\'`) + "'"
}
