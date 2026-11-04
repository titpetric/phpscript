package toolchain

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommandArgs(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want []string }{
		{[]string{"/usr/local/bin/phpx-test", "--matrix", "x"}, []string{"/usr/local/bin/phpx-test", "test", "--matrix", "x"}},
		{[]string{"phpscript-test", "test", "--matrix"}, []string{"phpscript-test", "test", "--matrix"}},
		{[]string{"phpscript-test"}, []string{"phpscript-test", "test"}},
		{[]string{"phpx-gen.exe", "a"}, []string{"phpx-gen.exe", "gen", "a"}},
		{[]string{"phpx", "run", "x.php"}, []string{"phpx", "run", "x.php"}},
		{[]string{"phpscript-linux-amd64", "run"}, []string{"phpscript-linux-amd64", "run"}},
		{nil, nil},
	}
	for _, tc := range cases {
		if got := CommandArgs(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("CommandArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCoverDir(t *testing.T) {
	t.Parallel()
	if got, want := CoverDir("/bin/phpx-test"), filepath.Join("cover", "phpx-test"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuiltWithCover(t *testing.T) {
	t.Parallel()
	// go test builds the test binary without -cover unless -cover is given;
	// either answer is valid, and the call must not panic.
	_ = BuiltWithCover()
}

func TestCoverRelaunchWithoutCoverBuild(t *testing.T) {
	t.Setenv("GOCOVERDIR", "")
	CoverRelaunch() // a plain test binary returns; a cover build would exec
}
