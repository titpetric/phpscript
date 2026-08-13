package testcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	testcmd "github.com/titpetric/phpscript/cmd/phpscript/test"
)

func TestRunCommand(t *testing.T) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "phpscript-testcmd-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jsonReport := filepath.Join(tmpDir, "report.json")
	htmlReport := filepath.Join(tmpDir, "report.html")

	opts := testcmd.Options{
		Report:     jsonReport,
		ReportHTML: htmlReport,
	}

	err = testcmd.Run(ctx, []string{"../../../tests/fixtures"}, opts)
	if err != nil {
		t.Fatalf("unexpected error running test command: %v", err)
	}

	if _, err := os.Stat(jsonReport); os.IsNotExist(err) {
		t.Fatalf("expected JSON report file at %s", jsonReport)
	}

	if _, err := os.Stat(htmlReport); os.IsNotExist(err) {
		t.Fatalf("expected HTML report file at %s", htmlReport)
	}
}
