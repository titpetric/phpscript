package model

import (
	"reflect"
	"testing"
)

// TestDatabaseProvider covers the shape of the contract, which is the whole of
// what the two sides share. The interface lives here so a runtime names a
// provider in its options without importing stdlib/database, and
// stdlib/database imports the runtime to register its bindings. Neither names
// the other and a signature drifting here breaks the host at its next build
// with nothing to point at.
//
// Connect is Open plus a ping and the two read the same name list and answer
// the same client: a caller that needs the storage reachable before it holds a
// client swaps one call for the other.
func TestDatabaseProvider(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*DatabaseProvider)(nil)).Elem()

	want := map[string]string{
		"Open":    "func(context.Context, ...string) (*sqlx.DB, error)",
		"Connect": "func(context.Context, ...string) (*sqlx.DB, error)",
	}
	if got := typ.NumMethod(); got != len(want) {
		t.Errorf("DatabaseProvider declares %d methods, want %d", got, len(want))
	}
	for name, signature := range want {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Errorf("DatabaseProvider declares no %s", name)
			continue
		}
		if got := method.Type.String(); got != signature {
			t.Errorf("%s is %s, want %s", name, got, signature)
		}
		if !method.Type.IsVariadic() {
			t.Errorf("%s takes a fixed name list, so it cannot resolve the first name that is configured", name)
		}
	}
}

// TestExtendedDatabaseProvider covers the split between resolving a connection
// and managing the set of them. A host that only resolves names implements
// DatabaseProvider alone and the two interfaces have to stay independent: a
// caller reaching for List type-asserts for this one and carries on without it.
func TestExtendedDatabaseProvider(t *testing.T) {
	t.Parallel()
	base := reflect.TypeOf((*DatabaseProvider)(nil)).Elem()
	extended := reflect.TypeOf((*ExtendedDatabaseProvider)(nil)).Elem()

	if extended.Implements(base) {
		t.Error("ExtendedDatabaseProvider implies DatabaseProvider, so a host cannot declare one without the other")
	}
	if base.Implements(extended) {
		t.Error("DatabaseProvider implies ExtendedDatabaseProvider, so every host has to answer List")
	}

	want := map[string]string{
		"List":     "func() []string",
		"Register": "func(string, string)",
	}
	if got := extended.NumMethod(); got != len(want) {
		t.Errorf("ExtendedDatabaseProvider declares %d methods, want %d", got, len(want))
	}
	for name, signature := range want {
		method, ok := extended.MethodByName(name)
		if !ok {
			t.Errorf("ExtendedDatabaseProvider declares no %s", name)
			continue
		}
		if got := method.Type.String(); got != signature {
			t.Errorf("%s is %s, want %s", name, got, signature)
		}
	}
}
