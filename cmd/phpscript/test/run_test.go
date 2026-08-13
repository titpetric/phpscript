package test_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/test"
)

func TestRunCommand(t *testing.T) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "phpscript-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jsonReport := filepath.Join(tmpDir, "report.json")
	htmlReport := filepath.Join(tmpDir, "report.html")

	opts := test.Options{
		Report:     jsonReport,
		ReportHTML: htmlReport,
	}

	err = test.Run(ctx, []string{"../../../tests/fixtures"}, opts)
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
