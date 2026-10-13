package core_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestTokenizerConstants covers the T_* ids token_get_all() reports arriving as
// constants, all of them.
//
// A script branches on a token by name, and a name that is not a constant reads
// as null, so comparing a token against it is a branch that never runs. Every
// id the tokenizer can emit is registered for that reason, and not the handful
// one caller happened to need. They are constants and no globals, so they
// resolve inside a function or a method too.
func TestTokenizerConstants(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	ids := parser.TokenIDs()
	if len(ids) == 0 {
		t.Fatal("the tokenizer reports no ids at all")
	}

	for name, id := range ids {
		value, ok := rt.Const(name)
		if !ok {
			t.Errorf("%s is not a defined constant", name)
			continue
		}
		if value != int64(id) {
			t.Errorf("%s = %#v, want int64(%d)", name, value, id)
		}
	}
}

// TestTokenName covers reading an id back as the name it was registered under,
// so a token_get_all() result prints as something a reader recognises. An id
// the tokenizer never emits answers "UNKNOWN", php's own wording for one.
func TestTokenName(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	fn, ok := rt.Callable("token_name")
	if !ok {
		t.Fatal("token_name is not registered")
	}

	for name, id := range parser.TokenIDs() {
		got, err := fn(int64(id))
		if err != nil {
			t.Fatalf("token_name(%d): %v", id, err)
		}
		if got != name {
			t.Errorf("token_name(%d) = %#v, want %q", id, got, name)
		}
	}

	got, err := fn(int64(-1))
	if err != nil {
		t.Fatalf("token_name(-1): %v", err)
	}
	if got != "UNKNOWN" {
		t.Errorf("token_name(-1) = %#v, want \"UNKNOWN\"", got)
	}
}
