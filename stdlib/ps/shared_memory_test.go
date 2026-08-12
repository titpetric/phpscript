package ps_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/ps"
)

func TestSharedMemoryBindingAcrossRequests(t *testing.T) {
	shm := ps.NewSharedMemory()
	runRequest := func(src string) string {
		t.Helper()

		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		buf := new(bytes.Buffer)
		rt := flatstack.New(buf, flatstack.Options{})
		rt.SetContext(ps.SharedMemoryContext(context.Background(), shm))
		stdlib.Register(rt)

		if err := rt.Run(prog); err != nil {
			t.Fatalf("Run error: %v", err)
		}
		return buf.String()
	}

	first := runRequest(`<?php
		$shm = new PS\SharedMemory;
		$shm->incr("requests");
		echo $shm->count("requests");
	?>`)
	if first != "1" {
		t.Fatalf("first request got %q, want %q", first, "1")
	}

	second := runRequest(`<?php
		$shm = new PS\SharedMemory;
		$shm->incr("requests");
		echo $shm->count("requests");
	?>`)
	if second != "2" {
		t.Errorf("second request got %q, want %q", second, "2")
	}
}
