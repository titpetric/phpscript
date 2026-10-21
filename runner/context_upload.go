package runner

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file is the upload half of a multipart request: copying each file part
// to its own temporary file, the error codes PHP reports on a refused part,
// the $_FILES shapes a script indexes, and the cleanup that ends the request.

// PHP's UPLOAD_ERR_* codes, as they appear in a $_FILES entry. The list names
// only the ones this runtime can produce; 2, 3 and 8 come from a per-form
// limit and from extensions phpscript does not have.
const (
	UploadErrOK        = 0
	UploadErrIniSize   = 1
	UploadErrNoFile    = 4
	UploadErrNoTmpDir  = 6
	UploadErrCantWrite = 7
)

// collectUploads copies every file part of a parsed multipart body to a
// temporary file and records it on the context. PHP exposes a path on disk and
// no stream, so the copy happens up front; Cleanup removes
// whatever the script did not move away.
func (c Context) collectUploads(r *http.Request, maxFileSize Size) {
	if r.MultipartForm == nil {
		return
	}
	for field, headers := range r.MultipartForm.File {
		for _, header := range headers {
			upload := saveUpload(header, maxFileSize)
			if upload.Error == UploadErrIniSize {
				c.recordError(fmt.Errorf("uploaded file %q of %d bytes exceeds the upload_max_filesize limit of %d bytes",
					upload.Name, header.Size, maxFileSize.Bytes()))
			}
			files := c.filesInto()
			files[field] = append(files[field], upload)
		}
	}
	// net/http spills the parts it could not hold in memory into temporary
	// files of its own. saveUpload has copied every one of them by now, so the
	// originals go before the handler has a chance to forget about them.
	_ = r.MultipartForm.RemoveAll()
}

// saveUpload writes one file part to its own temporary file. It reports a
// failure the way PHP does, as an error code on the entry, because a script
// reads $_FILES[...]["error"] and catches nothing.
func saveUpload(header *multipart.FileHeader, maxFileSize Size) *UploadedFile {
	upload := &UploadedFile{
		Name:     uploadBaseName(header.Filename),
		FullPath: header.Filename,
		Type:     header.Header.Get("Content-Type"),
		Size:     header.Size,
		Error:    UploadErrOK,
	}
	// A part PHP refuses before it reads it keeps only the two names the
	// client sent: no type, no size, no temporary file.
	if upload.Name == "" {
		return upload.refuse(UploadErrNoFile)
	}
	if maxFileSize.Exceeds(header.Size) {
		return upload.refuse(UploadErrIniSize)
	}

	src, err := header.Open()
	if err != nil {
		return upload.refuse(UploadErrCantWrite)
	}
	defer src.Close()

	dst, err := os.CreateTemp("", "phpscript-upload-*")
	if err != nil {
		return upload.refuse(UploadErrNoTmpDir)
	}
	size, err := io.Copy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst.Name())
		return upload.refuse(UploadErrCantWrite)
	}

	upload.TmpName = dst.Name()
	upload.Size = size
	return upload
}

// uploadBaseName strips the directory part a client may have sent along with
// the file name. The separator is the client's, not this host's, so it cuts on
// both; PHP keeps the base name only.
func uploadBaseName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// Cleanup removes the temporary files created for the uploaded parts of a
// request. A host handler defers it for the lifetime of one request; a file the
// script moved with move_uploaded_file is already gone and Cleanup skips it.
func (c Context) Cleanup() {
	for _, files := range c.files() {
		for _, file := range files {
			if file.TmpName != "" {
				_ = os.Remove(file.TmpName)
			}
		}
	}
}

// IsUpload reports whether path is the temporary file of a part of this
// request. It backs is_uploaded_file() and move_uploaded_file(), which in PHP
// refuse any path the request did not produce. A path the script has already
// moved away drops out of that set, so the copy has to still be there.
func (c Context) IsUpload(path string) bool {
	c.decodeForm()
	for _, files := range c.files() {
		for _, file := range files {
			if file.TmpName != "" && file.TmpName == path {
				_, err := os.Stat(path)
				return err == nil
			}
		}
	}
	return false
}

// uploadKeys is the key set of a $_FILES entry, in the order PHP writes it.
var uploadKeys = [...]string{"name", "full_path", "type", "tmp_name", "error", "size"}

// uploadValue reads one key of a $_FILES entry off an upload, so the scalar and
// the parallel-array shapes cannot disagree about what a key holds.
func uploadValue(file *UploadedFile, key string) any {
	switch key {
	case "name":
		return file.Name
	case "full_path":
		return file.FullPath
	case "type":
		return file.Type
	case "tmp_name":
		return file.TmpName
	case "error":
		return int64(file.Error)
	case "size":
		return file.Size
	}
	return nil
}

// filesArrayInto renders $_FILES. A field named "files[]" takes PHP's parallel
// array shape, $_FILES["files"]["name"][0]; any other field takes the scalar
// shape, $_FILES["file"]["name"], and keeps the last file sent under it, the
// way a repeated form value assigns over the one before it.
func (c Context) filesArrayInto(arr *model.Array) *model.Array {
	if len(c.files()) == 0 {
		return arr
	}
	fields := make([]string, 0, len(c.files()))
	for field := range c.files() {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		files := c.files()[field]
		if len(files) == 0 {
			continue
		}
		if name, isList := strings.CutSuffix(field, "[]"); isList {
			arr.Set(name, uploadListArray(files))
			continue
		}
		arr.Set(field, uploadArray(files[len(files)-1]))
	}
	return arr
}

// uploadArray renders one upload as PHP's keyed array.
func uploadArray(file *UploadedFile) *model.Array {
	arr := model.NewArraySize(len(uploadKeys))
	for _, key := range uploadKeys {
		arr.Set(key, uploadValue(file, key))
	}
	return arr
}

// uploadListArray renders a list field: the same keys, each holding one value
// per file in place of a single value.
func uploadListArray(files []*UploadedFile) *model.Array {
	arr := model.NewArraySize(len(uploadKeys))
	for _, key := range uploadKeys {
		column := model.NewArraySize(len(files))
		for i, file := range files {
			column.Set(int64(i), uploadValue(file, key))
		}
		arr.Set(key, column)
	}
	return arr
}
