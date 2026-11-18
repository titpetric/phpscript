package model

import (
	"reflect"
	"testing"
)

// TestDatabaseProvider covers the contract's shape, all the two sides share.
// The interface lives here so a runtime names a provider in its options without
// importing stdlib/ps/database; stdlib/ps/database imports the runtime to
// register its bindings. Neither package names the other. No check points at a
// signature that drifts here until the host breaks at its next build.
//
// Connect is Open plus a ping and the two read the same name list and answer
// the same client: a caller that needs the storage reachable before it holds a
// client swaps one call for the other.
func TestDatabaseProvider(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*DatabaseProvider)(nil)).Elem()

	want := map[string]string{
		"Open":    "func(context.Context, ...string) (*sql.DB, error)",
		"Connect": "func(context.Context, ...string) (*sql.DB, error)",
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
// and managing the set. A host that only resolves names implements
// DatabaseProvider alone; the two interfaces must stay independent. A caller
// reaching for List type-asserts for the second interface and continues
// without the first.
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
