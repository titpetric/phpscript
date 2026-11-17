package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestHelperGoLit covers the go runtime's composite literals: []any and
// map[string]any by inference, and the typed slice and map by spelling.
func TestHelperGoLit(t *testing.T) {
	t.Parallel()
	pairs := []model.ArrayItemValue{{Val: int64(1)}, {Val: int64(2)}, {Val: int64(3)}}
	list, err := helperGoLit(false, "", "", pairs...)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got, ok := list.([]any); !ok || len(got) != 3 || got[2] != int64(3) {
		t.Errorf("[] literal = %#v, want []any{1, 2, 3}", list)
	}

	keyed := []model.ArrayItemValue{{Key: "a", Val: int64(1)}, {Key: "b", Val: int64(2)}}
	byName, err := helperGoLit(true, "", "", keyed...)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if got, ok := byName.(map[string]any); !ok || got["b"] != int64(2) {
		t.Errorf("{} literal = %#v, want map[string]any", byName)
	}

	names, err := helperGoLit(false, "", "[]string", model.ArrayItemValue{Val: "ada"}, model.ArrayItemValue{Val: "grace"})
	if err != nil {
		t.Fatalf("[]string: %v", err)
	}
	if got, ok := names.([]string); !ok || got[1] != "grace" {
		t.Errorf("[]string literal = %#v, want []string", names)
	}

	ages, err := helperGoLit(true, "string", "int", model.ArrayItemValue{Key: "ada", Val: int64(36)})
	if err != nil {
		t.Fatalf("map[string]int: %v", err)
	}
	if got, ok := ages.(map[string]int64); !ok || got["ada"] != 36 {
		t.Errorf("map[string]int literal = %#v, want map[string]int64", ages)
	}

	if _, err := helperGoLit(false, "", "[]int8", model.ArrayItemValue{Val: int64(300)}); err != nil {
		t.Fatalf("[]int8: %v", err)
	}
}

// TestGoLitNested covers the typed forms that nest: a slice of slices, a map
// of slices and a slice of maps, each converted entry by entry.
func TestGoLitNested(t *testing.T) {
	t.Parallel()
	rows, err := helperGoLit(false, "", "[][]string",
		model.ArrayItemValue{Val: []any{"a", "b"}},
		model.ArrayItemValue{Val: []any{"c"}},
	)
	if err != nil {
		t.Fatalf("[][]string: %v", err)
	}
	got, ok := rows.([][]string)
	if !ok || len(got) != 2 || got[0][1] != "b" || got[1][0] != "c" {
		t.Errorf("[][]string literal = %#v, want [[a b] [c]]", rows)
	}

	byName, err := helperGoLit(true, "string", "[]int",
		model.ArrayItemValue{Key: "evens", Val: []any{int64(2), int64(4)}},
	)
	if err != nil {
		t.Fatalf("map[string][]int: %v", err)
	}
	evens, ok := byName.(map[string][]int64)
	if !ok || len(evens["evens"]) != 2 || evens["evens"][1] != 4 {
		t.Errorf("map[string][]int literal = %#v, want evens [2 4]", byName)
	}

	users, err := helperGoLit(false, "", "[]map[string]int",
		model.ArrayItemValue{Val: map[string]any{"ada": int64(36)}},
	)
	if err != nil {
		t.Fatalf("[]map[string]int: %v", err)
	}
	list, ok := users.([]map[string]int64)
	if !ok || len(list) != 1 || list[0]["ada"] != 36 {
		t.Errorf("[]map[string]int literal = %#v, want one map", users)
	}
}
