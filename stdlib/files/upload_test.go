package files_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// uploadRequest builds a POST carrying one file part, the body a browser sends
// for a form with a file input.
func uploadRequest(t *testing.T, field, filename, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("POST", "/upload", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

// TestUploadedFileShims covers the pair of functions a script needs after
// $_FILES arrives: the upload is on disk outside the root, and moving it into
// the root is the only supported way to keep it past the request.
func TestUploadedFileShims(t *testing.T) {
	ctx := runner.FromRequest(uploadRequest(t, "avatar", "photo.png", "image-bytes"))
	defer ctx.Cleanup()

	root := t.TempDir()
	src := `<?php
$f = $_FILES["avatar"];
if (is_uploaded_file($f["tmp_name"])) { echo "upload"; } else { echo "not-upload"; }
if (is_uploaded_file("/etc/passwd")) { echo "|unguarded"; } else { echo "|guarded"; }
if (move_uploaded_file($f["tmp_name"], "photo.png")) { echo "|moved"; } else { echo "|failed"; }
if (move_uploaded_file($f["tmp_name"], "again.png")) { echo "|moved-twice"; } else { echo "|gone"; }
if ($f["error"] === UPLOAD_ERR_OK) { echo "|ok"; } else { echo "|error"; }
`
	out := runFS(t, root, &ctx, src)

	want := "upload|guarded|moved|gone|ok"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}

	stored, err := os.ReadFile(filepath.Join(root, "photo.png"))
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if string(stored) != "image-bytes" {
		t.Fatalf("moved file content = %q", stored)
	}
}

// TestMoveUploadedFileMode pins the mode of a stored upload: the temporary copy
// is created private to this process, and a file a web server is expected to
// serve cannot stay that way. A host that serves uploads to nobody but itself
// configures something tighter.
func TestMoveUploadedFileMode(t *testing.T) {
	for _, test := range []struct {
		name string
		opts runner.Options
		want os.FileMode
	}{
		{name: "default", want: 0o644},
		{name: "configured", opts: runner.Options{UploadFileMode: 0o600}, want: 0o600},
		{name: "group readable", opts: runner.Options{UploadFileMode: 0o640}, want: 0o640},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := runner.FromRequest(uploadRequest(t, "avatar", "photo.png", "image-bytes"))
			defer ctx.Cleanup()

			root := t.TempDir()
			runFSOptions(t, root, &ctx, test.opts,
				`<?php move_uploaded_file($_FILES["avatar"]["tmp_name"], "photo.png");`)

			st, err := os.Stat(filepath.Join(root, "photo.png"))
			if err != nil {
				t.Fatal(err)
			}
			if got := st.Mode().Perm(); got != test.want {
				t.Fatalf("mode = %o, want %o", got, test.want)
			}
		})
	}
}

// TestMoveUploadedFileUnwritable covers a move that cannot finish. PHP reports
// it as a false return and nothing else, and the upload has to survive it: the
// request still lists it, and the handler still has to be able to remove it.
func TestMoveUploadedFileUnwritable(t *testing.T) {
	ctx := runner.FromRequest(uploadRequest(t, "avatar", "photo.png", "image-bytes"))
	defer ctx.Cleanup()

	// A directory in the way fails both the rename and the copy that backs it
	// up, which is the portable way to make a move fail from a test.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "photo.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmpName := ctx.Files["avatar"][0].TmpName

	out := runFS(t, root, &ctx, `<?php
if (move_uploaded_file($_FILES["avatar"]["tmp_name"], "photo.png")) { echo "moved"; } else { echo "failed"; }`)
	if out != "failed" {
		t.Fatalf("got %q, want %q", out, "failed")
	}
	// A move that could not finish leaves the upload where it was, so the
	// request can still report it and Cleanup can still remove it.
	if _, err := os.Stat(tmpName); err != nil {
		t.Fatalf("tmp_name after a failed move: %v", err)
	}
}

// runFS parses and runs src with the filesystem shims rooted at root. request
// is optional; when set, its superglobals and uploads are installed too.
func runFS(t *testing.T, root string, request *runner.Context, src string) string {
	t.Helper()
	return runFSOptions(t, root, request, runner.Options{}, src)
}

// runFSOptions is runFS for the cases that configure the runtime, the upload
// file mode in particular.
func runFSOptions(t *testing.T, root string, request *runner.Context, opts runner.Options, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, opts)
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	if request != nil {
		request.Register(rt)
	}
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}
