package parser_test

import (
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// tokTriple extracts (id, text) from an array-form token, or (-1, str) from a
// single-char string token.
func tokTriple(t *testing.T, v any) (int, string) {
	t.Helper()
	if s, ok := v.(string); ok {
		return -1, s
	}
	arr, ok := v.(*model.Array)
	if !ok {
		t.Fatalf("token is neither string nor *Array: %T", v)
	}
	id, _ := arr.Get(int64(0))
	text, _ := arr.Get(int64(1))
	return int(id.(int64)), text.(string)
}

func TestTokenGetAllShape(t *testing.T) {
	// Mirrors how minitpl wraps an expression before tokenizing.
	toks := parser.TokenGetAll(`<?php if ($this->_vars) { ?>`)

	var got []struct {
		id   int
		text string
	}
	toks.Range(func(_, v any) bool {
		id, text := tokTriple(t, v)
		got = append(got, struct {
			id   int
			text string
		}{id, text})
		return true
	})

	// Verify key tokens are classified the way minitpl's _split_exp expects.
	var sawOpenTag, sawIf, sawVar, sawArrow bool
	for _, g := range got {
		switch {
		case g.id == parser.T_OPEN_TAG:
			sawOpenTag = true
		case g.id == parser.T_IF && g.text == "if":
			sawIf = true
		case g.id == parser.T_VARIABLE && g.text == "$this":
			sawVar = true
		case g.id == parser.T_OBJECT_OPERATOR && g.text == "->":
			sawArrow = true
		}
	}
	if !sawOpenTag || !sawIf || !sawVar || !sawArrow {
		t.Fatalf("missing expected tokens: open=%v if=%v var=%v arrow=%v\n%+v",
			sawOpenTag, sawIf, sawVar, sawArrow, got)
	}
}

func TestTokenSingleCharStrings(t *testing.T) {
	toks := parser.TokenGetAll(`<?php ($a);`)
	var parens int
	toks.Range(func(_, v any) bool {
		if s, ok := v.(string); ok && (s == "(" || s == ")" || s == ";") {
			parens++
		}
		return true
	})
	if parens != 3 {
		t.Fatalf("expected 3 single-char tokens, got %d", parens)
	}
}

func TestTokenName(t *testing.T) {
	cases := map[int]string{
		parser.T_VARIABLE:        "T_VARIABLE",
		parser.T_OBJECT_OPERATOR: "T_OBJECT_OPERATOR",
		parser.T_DOUBLE_COLON:    "T_PAAMAYIM_NEKUDOTAYIM",
		parser.T_IF:              "T_IF",
		-1:                       "UNKNOWN",
	}
	for id, want := range cases {
		if got := parser.TokenName(id); got != want {
			t.Fatalf("TokenName(%d) = %q, want %q", id, got, want)
		}
	}
}

func TestTokenVariableWithDotMarker(t *testing.T) {
	// minitpl replaces "." with "__1" before tokenizing so nested accessors stay
	// part of one T_VARIABLE; verify that holds.
	toks := parser.TokenGetAll(`<?php $foo__1bar`)
	var found string
	toks.Range(func(_, v any) bool {
		if arr, ok := v.(*model.Array); ok {
			id, _ := arr.Get(int64(0))
			if int(id.(int64)) == parser.T_VARIABLE {
				txt, _ := arr.Get(int64(1))
				found = txt.(string)
			}
		}
		return true
	})
	if found != "$foo__1bar" {
		t.Fatalf("got %q, want $foo__1bar", found)
	}
}
