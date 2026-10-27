package tests

import (
	"context"
	"reflect"
	"testing"
)

// TestRecord covers the struct a Get answers with. Record exports both fields,
// and runner.helperGet reads `$rec->key` off them by reflection; an unexported
// field would be invisible to a script.
func TestRecord(t *testing.T) {
	record := Record{Key: "color", Value: "blue"}
	if record.Key != "color" || record.Value != "blue" {
		t.Errorf("Record = %+v, want {color blue}", record)
	}

	for _, name := range []string{"Key", "Value"} {
		field, ok := reflect.TypeOf(record).FieldByName(name)
		if !ok {
			t.Fatalf("Record declares no field %s", name)
		}
		if !field.IsExported() {
			t.Errorf("Record.%s is unexported, so a script cannot read it", name)
		}
	}
}

// TestStorage covers the capability PHP reaches through `new Storage`: the
// tenant the constructor reads off the context the runner fills in, and the
// three readers a fixture calls on it. All() is key-sorted, and a foreach over
// it reports the same order on every run.
func TestStorage(t *testing.T) {
	ctx := context.WithValue(context.Background(), tenantKey, "acme")
	storage, err := NewStorage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if got := storage.Tenant(); got != "acme" {
		t.Errorf("Tenant() = %q, want \"acme\"", got)
	}

	storage.Set(ctx, "size", "large")
	storage.Set(ctx, "color", "blue")
	if got := storage.Len(); got != 2 {
		t.Errorf("Len() = %d after two writes, want 2", got)
	}

	record, err := storage.Get(ctx, "color")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Record{Key: "color", Value: "blue"}); record != want {
		t.Errorf("Get(\"color\") = %+v, want %+v", record, want)
	}

	all, err := storage.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{{Key: "color", Value: "blue"}, {Key: "size", Value: "large"}}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("All() = %+v, want %+v", all, want)
	}
}

// TestStorage_Keep covers the predicate shape a host declares in Go's own
// terms. coerceArgOn wraps a PHP closure into func(key, value string) bool
// through reflect.MakeFunc, and the method reads two strings and a bool and
// knows nothing about the VM.
func TestStorage_Keep(t *testing.T) {
	storage := &memStorage{data: map[string]string{"a": "1", "b": "2", "c": "3"}}

	kept := storage.Keep(func(key, _ string) bool { return key != "b" })
	want := []Record{{Key: "a", Value: "1"}, {Key: "c", Value: "3"}}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("Keep() = %+v, want %+v", kept, want)
	}

	if none := storage.Keep(func(string, string) bool { return false }); len(none) != 0 {
		t.Errorf("Keep() = %+v on a predicate that answers false, want nothing", none)
	}
}

// TestStorage_Walk covers the uniform func(...any) (any, error) shape, whose
// error slot is where a callback reports a failure without a panic. A failing
// visit stops the walk, and a caller reads one error and not one per key.
func TestStorage_Walk(t *testing.T) {
	storage := &memStorage{data: map[string]string{"a": "1", "b": "2", "c": "3"}}

	var seen []string
	if err := storage.Walk(func(args ...any) (any, error) {
		seen = append(seen, args[0].(string))
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := seen; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("Walk visited %v, want [a b c]", got)
	}

	seen = nil
	err := storage.Walk(func(args ...any) (any, error) {
		seen = append(seen, args[0].(string))
		return nil, context.Canceled
	})
	if err != context.Canceled {
		t.Errorf("Walk() = %v, want the error the visit answered", err)
	}
	if got := seen; !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("Walk visited %v after the first failure, want [a]", got)
	}
}
