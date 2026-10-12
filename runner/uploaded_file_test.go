package runner

import (
	"testing"
)

// TestUploadedFile covers the shape a $_FILES entry exposes. Name is the base
// name and FullPath the path the client sent, which a directory upload uses to
// say where in its own tree a file sat; neither is a path on this host, and
// TmpName is the only one that is.
func TestUploadedFile(t *testing.T) {
	file := &UploadedFile{
		Name:     "report.pdf",
		FullPath: "quarter/report.pdf",
		Type:     "application/pdf",
		TmpName:  "/tmp/phpscript-upload-1",
		Size:     2048,
		Error:    UploadErrOK,
	}

	if file.Error != UploadErrOK {
		t.Errorf("a stored part carries error %d, want UploadErrOK", file.Error)
	}
	if file.TmpName == "" {
		t.Error("a stored part names no temporary copy")
	}
	if file.Name == file.FullPath {
		t.Error("Name and FullPath are the same, so the directory part was not taken off")
	}
}

// TestUploadedFile_refuse covers what PHP writes for a part it refused: the
// error code, and nothing describing content there is none of. A script reading
// $_FILES sees a zero size and an empty tmp_name, and those say the file is not
// there to move.
func TestUploadedFile_refuse(t *testing.T) {
	file := &UploadedFile{
		Name:    "big.iso",
		Type:    "application/octet-stream",
		TmpName: "/tmp/phpscript-upload-2",
		Size:    1 << 30,
	}

	refused := file.refuse(UploadErrIniSize)
	if refused != file {
		t.Error("refuse answered a different file from the one it was given")
	}
	if file.Error != UploadErrIniSize {
		t.Errorf("Error = %d after refuse, want UploadErrIniSize (%d)", file.Error, UploadErrIniSize)
	}
	if file.TmpName != "" || file.Size != 0 || file.Type != "" {
		t.Errorf("a refused part still describes content: %+v", file)
	}
	if file.Name != "big.iso" {
		t.Errorf("Name = %q after refuse, and the client's name is what names the failure", file.Name)
	}
}
