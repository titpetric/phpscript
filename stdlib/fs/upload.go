package fs

import (
	"os"

	"github.com/titpetric/phpscript/runner"
)

// registerUploads installs the pair of functions a script needs after $_FILES
// arrives. An uploaded part is written outside the root by the request runtime,
// so a script reaches it by the absolute tmp_name it was handed; both functions
// refuse a path this request did not produce, as PHP does.
func registerUploads(rt *runner.Runtime, r root) {
	isUpload := func(p string) bool {
		request, ok := runner.RequestContext(rt.Context())
		return ok && request.IsUpload(p)
	}

	rt.RegisterFunc("is_uploaded_file", isUpload)
	rt.RegisterFunc("move_uploaded_file", func(from, to string) bool {
		if !isUpload(from) {
			return false
		}
		dst := r.resolve(to)
		if err := moveFile(from, dst); err != nil {
			return false
		}
		// The temporary copy is private to the process that wrote it, so the
		// stored file gets a mode of its own: the configured upload_file_mode,
		// or 0644, the mode PHP's own umask usually lands on.
		_ = os.Chmod(dst, rt.UploadFileMode().Mode())
		return true
	})
}
