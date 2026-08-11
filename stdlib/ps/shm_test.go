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

func TestSHMBinding(t *testing.T) {
	shm := ps.NewSHM()
	ctx := ps.SHMContext(context.Background(), shm)

	src := `<?php
		$shm = new PS\SHM;
		$shm->set("key", "hello");
		$shm->incr("count");
		$shm->incr("count");
		echo $shm->get("key") . ":" . $shm->count("count");
	?>`

	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	buf := new(bytes.Buffer)
	rt := flatstack.New(buf, flatstack.Options{})
	rt.SetContext(ctx)
	stdlib.Register(rt)

	if err := rt.Run(prog); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := buf.String()
	want := "hello:2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
