package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/test"
	"github.com/titpetric/phpscript/tests"
)

func TestMain(m *testing.M) {
	tests.TestMain(m)
}

func TestRunCommandJSON(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	cpuPath := filepath.Join(tmpDir, "cpu.pprof")
	memPath := filepath.Join(tmpDir, "mem.pprof")

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	errRun := test.Run(ctx, []string{"../../../tests/fixtures/exceptions/die_exit.phpt"}, test.Options{
		JSON:       true,
		CPUProfile: cpuPath,
		MemProfile: memPath,
	})
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if errRun != nil {
		t.Fatalf("unexpected error running test command: %v", errRun)
	}

	var report struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Results []struct {
			Passed bool `json:"passed"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if report.Total < 1 || report.Passed < 1 {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(cpuPath); err != nil {
		t.Fatalf("cpuprofile: %v", err)
	}
	if _, err := os.Stat(memPath); err != nil {
		t.Fatalf("memprofile: %v", err)
	}
}
