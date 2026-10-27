package tests_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// benchHash is one bcrypt line at cost 4, the cheapest the format allows.
// The parse checks the format of a stored hash and derives nothing from it,
// and one hash repeated per row prices the read the way distinct rows would.
const benchHash = "$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW"

// BenchmarkSessionManagerDefault measures `new Session\Manager` with no
// arguments: disk storage and the application's own .htpasswd. The subtests
// scale that file, pricing what the default user store costs a request that
// constructs a manager and never calls login(). users=0 is the floor: the
// same spelling against a root with no password file.
func BenchmarkSessionManagerDefault(b *testing.B) {
	for _, users := range []int{0, 10, 1000} {
		b.Run(fmt.Sprintf("users=%d", users), func(b *testing.B) {
			benchmarkSessionManager(b, users)
		})
	}
}

func benchmarkSessionManager(b *testing.B, users int) {
	b.Setenv("TMPDIR", b.TempDir())

	root := b.TempDir()
	if users > 0 {
		lines := make([]string, 0, users)
		for i := range users {
			lines = append(lines, fmt.Sprintf("user%04d:%s", i, benchHash))
		}
		body := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(root, ".htpasswd"), []byte(body), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	prog, err := parser.Parse(`<?php $session = new Session\Manager;`)
	if err != nil {
		b.Fatal(err)
	}

	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	ctx := runner.NewContext()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.ResetSession(io.Discard, nil)
		ctx.Register(rt)
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}
