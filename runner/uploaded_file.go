package runner

// UploadedFile holds one file part of a multipart request body, in the shape a
// PHP $_FILES entry exposes. TmpName is the absolute path of the server-side
// copy, which lives until Cleanup runs or the script moves it away.
type UploadedFile struct {
	Name string // client-supplied file name, without any directory part
	// FullPath is the file name as the client sent it, with the directory part
	// included. A directory upload uses FullPath to say where in the tree a file sat. PHP
	// 8.1 added it. Like Name, it holds the client's word. No path on this host
	// appears.
	FullPath string
	Type     string // client-supplied content type
	TmpName  string // path of the temporary copy, empty on a non-zero Error
	Size     int64
	Error    int // an UPLOAD_ERR_* code, UploadErrOK for a stored part
}

// refuse marks an upload as not stored, keeping the error code and clearing the content fields.
func (u *UploadedFile) refuse(code int) *UploadedFile {
	u.Type = ""
	u.TmpName = ""
	u.Size = 0
	u.Error = code
	return u
}
