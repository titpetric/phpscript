package runner

// UploadedFile is one file part of a multipart request body, in the shape a
// PHP $_FILES entry exposes. TmpName is the absolute path of the server-side
// copy, which lives until Cleanup runs or the script moves it away.
type UploadedFile struct {
	Name string // client-supplied file name, without any directory part
	// FullPath is the file name as the client sent it, directory part and all,
	// which a directory upload uses to say where in the tree a file sat. PHP
	// 8.1 added it; like Name, it carries the client's word and no path on
	// this host.
	FullPath string
	Type     string // client-supplied content type
	TmpName  string // path of the temporary copy, empty on a non-zero Error
	Size     int64
	Error    int // an UPLOAD_ERR_* code, UploadErrOK for a stored part
}

// refuse marks an upload as not stored, in the shape PHP writes for an entry it
// refused: the error code, and nothing that describes content there is none of.
func (u *UploadedFile) refuse(code int) *UploadedFile {
	u.Type = ""
	u.TmpName = ""
	u.Size = 0
	u.Error = code
	return u
}
