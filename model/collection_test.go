package model

import (
	"reflect"
	"testing"
)

// stringList is the smallest Collection that is not an *Array: a view over a
// slice a binding already holds.
type stringList []string

func (l stringList) Len() int { return len(l) }

func (l stringList) Range(fn func(key, val any) bool) {
	for i, item := range l {
		if !fn(int64(i), item) {
			return
		}
	}
}

// propBag is a Keyed view over a map, with insertion order kept so its Range is
// comparable against a wanted sequence.
type propBag struct {
	order []string
	vals  map[string]any
}

func newPropBag() *propBag {
	return &propBag{vals: map[string]any{}}
}

func (b *propBag) Len() int { return len(b.order) }

func (b *propBag) Range(fn func(key, val any) bool) {
	for _, key := range b.order {
		if !fn(key, b.vals[key]) {
			return
		}
	}
}

func (b *propBag) Read(key string) any { return b.vals[key] }

func (b *propBag) Has(key string) bool {
	_, ok := b.vals[key]
	return ok
}

func (b *propBag) Write(key string, val any) {
	if !b.Has(key) {
		b.order = append(b.order, key)
	}
	b.vals[key] = val
}

func (b *propBag) Delete(key string) {
	delete(b.vals, key)
	for i, name := range b.order {
		if name == key {
			b.order = append(b.order[:i], b.order[i+1:]...)
			return
		}
	}
}

// TestCollection covers the contract a value satisfies to read as an array
// without being one: its own length and its own iteration. count(), foreach and
// the array bindings read a collection through LenValues, RangeValues and
// IsCollection and a view over data a binding already holds reaches all three
// and pays nothing to build an *Array.
//
// A nil *Array reports no collection and count() applies PHP's scalar rule to
// it the way it does to a string.
func TestCollection(t *testing.T) {
	t.Parallel()
	var _ Collection = NewArray()

	var view Collection = stringList{"a", "b", "c"}
	if got, ok := LenValues(view); !ok || got != 3 {
		t.Errorf("LenValues() = (%d, %v), want (3, true)", got, ok)
	}
	if !IsCollection(view) {
		t.Error("IsCollection() = false on a Collection")
	}

	var keys, vals []any
	RangeValues(view, func(key, val any) bool {
		keys = append(keys, key)
		vals = append(vals, val)
		return true
	})
	if want := []any{int64(0), int64(1), int64(2)}; !reflect.DeepEqual(keys, want) {
		t.Errorf("RangeValues visited keys %v, want %v", keys, want)
	}
	if want := []any{"a", "b", "c"}; !reflect.DeepEqual(vals, want) {
		t.Errorf("RangeValues visited values %v, want %v", vals, want)
	}

	var missing *Array
	if got, ok := LenValues(missing); ok || got != 0 {
		t.Errorf("LenValues() = (%d, %v) on a nil *Array, want (0, false)", got, ok)
	}
}

// TestKeyed covers the four operations indexing, isset() and unset() need from a
// collection read by name: Read answers nil for a name nothing holds, and Has
// tells that apart from a name holding nil.
//
// Keyed embeds Collection and one value answers count() and foreach as well,
// and a runtime reaching a bag declared in another package needs no *Array.
func TestKeyed(t *testing.T) {
	t.Parallel()
	var bag Keyed = newPropBag()

	bag.Write("host", "localhost")
	bag.Write("port", int64(8080))
	bag.Write("tls", nil)

	if got := bag.Read("host"); got != "localhost" {
		t.Errorf(`Read("host") = %v, want "localhost"`, got)
	}
	if got := bag.Read("missing"); got != nil {
		t.Errorf(`Read("missing") = %v, want nil`, got)
	}
	if !bag.Has("tls") {
		t.Error(`Has("tls") = false on a name holding nil`)
	}
	if bag.Has("missing") {
		t.Error(`Has("missing") = true`)
	}
	if got := bag.Len(); got != 3 {
		t.Errorf("Len() = %d after three writes, want 3", got)
	}

	bag.Write("port", int64(9090))
	if got := bag.Read("port"); got != int64(9090) {
		t.Errorf(`Read("port") = %v after a second write, want 9090`, got)
	}
	if got := bag.Len(); got != 3 {
		t.Errorf("Len() = %d after rewriting a name, want 3", got)
	}

	bag.Delete("host")
	if bag.Has("host") {
		t.Error(`Has("host") = true after Delete`)
	}
	if got, ok := LenValues(bag); !ok || got != 2 {
		t.Errorf("LenValues() = (%d, %v), want (2, true)", got, ok)
	}

	var keys []any
	RangeValues(bag, func(key, _ any) bool {
		keys = append(keys, key)
		return true
	})
	if want := []any{"port", "tls"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("RangeValues visited keys %v, want %v", keys, want)
	}
}
