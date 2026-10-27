package runner_test

import (
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestFromRequestMultipart covers the body shape ParseForm does not decode: a
// multipart POST has to reach $_POST as well as an urlencoded one does, and its
// file parts have to arrive in $_FILES.
func TestFromRequestMultipart(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "avatar", filename: "photo.png", content: "binary-ish"},
	)

	ctx := runner.FromRequest(r)
	if got := ctx.PostMap()["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q, want %q", got, "bob")
	}

	files := ctx.FileMap()["avatar"]
	if len(files) != 1 {
		t.Fatalf("uploads for avatar = %d, want 1", len(files))
	}
	upload := files[0]
	if upload.Error != runner.UploadErrOK {
		t.Fatalf("upload error = %d, want %d", upload.Error, runner.UploadErrOK)
	}
	if upload.Name != "photo.png" {
		t.Fatalf("upload name = %q, want %q", upload.Name, "photo.png")
	}
	if upload.Size != int64(len("binary-ish")) {
		t.Fatalf("upload size = %d, want %d", upload.Size, len("binary-ish"))
	}

	stored, err := os.ReadFile(upload.TmpName)
	if err != nil {
		t.Fatalf("read tmp_name: %v", err)
	}
	if string(stored) != "binary-ish" {
		t.Fatalf("tmp_name content = %q", stored)
	}
	if !ctx.IsUpload(upload.TmpName) {
		t.Fatalf("IsUpload(%q) = false", upload.TmpName)
	}

	// The temporary copy belongs to the request, and the host handler ends it.
	ctx.Cleanup()
	if _, err := os.Stat(upload.TmpName); !os.IsNotExist(err) {
		t.Fatalf("tmp_name after Cleanup: err = %v, want not-exist", err)
	}
}

// TestFilesSuperglobal checks the array shape a script indexes into, which is
// PHP's keys and not the Go struct behind them. full_path holds the name the
// client sent whole, name holds its base alone and a client cannot choose a
// directory for the server.
func TestFilesSuperglobal(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t, nil,
		multipartFile{field: "avatar", filename: `C:\Users\bob\photo.png`, content: "abc"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	src := `<?php
$f = $_FILES["avatar"];
echo $f["name"] . "|" . $f["full_path"] . "|" . $f["type"] . "|" . $f["size"] . "|" . $f["error"] . "|" . $f["tmp_name"];
`
	out := runCtx(t, ctx, src)
	want := `photo.png|C:\Users\bob\photo.png|application/octet-stream|3|0|` + ctx.FileMap()["avatar"][0].TmpName
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFilesSuperglobalRepeatedField covers a plain field sent twice. PHP has
// nowhere to put the first file in a scalar entry and the last one wins, the
// same way a repeated $_POST value does.
func TestFilesSuperglobalRepeatedField(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t, nil,
		multipartFile{field: "avatar", filename: "first.png", content: "aa"},
		multipartFile{field: "avatar", filename: "second.png", content: "bbbb"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	out := runCtx(t, ctx, `<?php echo $_FILES["avatar"]["name"] . "|" . $_FILES["avatar"]["size"];`)
	if want := "second.png|4"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// Both parts are still stored and Cleanup removes both.
	if got := len(ctx.FileMap()["avatar"]); got != 2 {
		t.Fatalf("uploads for avatar = %d, want 2", got)
	}
}

// TestFilesSuperglobalList covers a repeated field: PHP spells it "name[]" and
// writes the entry as parallel arrays in place of scalars.
func TestFilesSuperglobalList(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t, nil,
		multipartFile{field: "docs[]", filename: "a.txt", content: "aa"},
		multipartFile{field: "docs[]", filename: "b.txt", content: "bbbb"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	src := `<?php
$f = $_FILES["docs"];
echo $f["name"][0] . "|" . $f["name"][1] . "|" . $f["size"][0] . "|" . $f["size"][1];
`
	out := runCtx(t, ctx, src)
	want := "a.txt|b.txt|2|4"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestUploadMaxFilesize covers a single file part over the limit. PHP keeps the
// rest of the form and marks that one entry UPLOAD_ERR_INI_SIZE and a script
// can tell the user which file it refuses.
func TestUploadMaxFilesize(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "small", filename: "small.txt", content: "tiny"},
		multipartFile{field: "big", filename: "big.txt", content: strings.Repeat("x", 4096)},
	)

	ctx := runner.FromRequestOptions(r, runner.Options{UploadMaxFilesize: 1024})
	defer ctx.Cleanup()

	if got := ctx.PostMap()["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q, want the form to survive", got)
	}
	small := ctx.FileMap()["small"][0]
	if small.Error != runner.UploadErrOK || small.TmpName == "" {
		t.Fatalf("small upload = %+v, want it stored", small)
	}
	// PHP describes a refused part by the names the client sent and nothing
	// else: no type, no size, no temporary file.
	big := ctx.FileMap()["big"][0]
	if big.Error != runner.UploadErrIniSize {
		t.Fatalf("big upload error = %d, want %d", big.Error, runner.UploadErrIniSize)
	}
	if big.Name != "big.txt" || big.TmpName != "" || big.Type != "" || big.Size != 0 {
		t.Fatalf("big upload = %+v, want the names only", big)
	}
	if errs := ctx.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "upload_max_filesize") {
		t.Fatalf("errors = %v, want the upload_max_filesize error", errs)
	}

	out := runCtx(t, ctx, `<?php
echo $_FILES["small"]["error"] === UPLOAD_ERR_OK ? "small-ok" : "small-refused";
echo $_FILES["big"]["error"] === UPLOAD_ERR_INI_SIZE ? "|big-too-large" : "|big-ok";`)
	if want := "small-ok|big-too-large"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
