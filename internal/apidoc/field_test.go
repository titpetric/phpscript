package apidoc

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

// fieldHolder is the shape a binding with readable properties takes: exported
// fields a script reads, machinery it does not, and a field holding a
// registered class.
type fieldHolder struct {
	Bridge     *fieldBridge
	ID         string
	IsReadonly bool
	deadline   time.Time
	pool       int
}

type fieldBridge struct{}

// TestField covers the two spellings a property carries: the Go field name
// keying the doc index, and the snake_case name a script writes.
func TestField(t *testing.T) {
	t.Parallel()
	fields := reflectFields(reflect.TypeOf(&fieldHolder{}), nil)
	if len(fields) == 0 {
		t.Fatal("reflectFields found no property")
	}
	if got, want := fields[1].GoName, "ID"; got != want {
		t.Errorf("GoName = %q, want %q", got, want)
	}
	if got, want := fields[1].Name, "id"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
}

// TestReflectFieldsSkipsUnexported pins the one rule on which properties reach
// a listing. Runtime.structField resolves a property read to an exported field
// and reports an unexported one as absent, and a listing naming one would name
// a property no script can read.
func TestReflectFieldsSkipsUnexported(t *testing.T) {
	t.Parallel()
	for _, f := range reflectFields(reflect.TypeOf(&fieldHolder{}), nil) {
		if f.GoName == "deadline" || f.GoName == "pool" {
			t.Errorf("unexported field %s is published as $%s", f.GoName, f.Name)
		}
	}
}

// TestReflectFieldsKeepsDeclarationOrder pins the order properties print in:
// the order the struct declares them, the order `go doc` prints a struct in. A
// reader comparing the two reads the same list.
func TestReflectFieldsKeepsDeclarationOrder(t *testing.T) {
	t.Parallel()
	want := []string{"bridge", "id", "is_readonly"}
	fields := reflectFields(reflect.TypeOf(&fieldHolder{}), nil)
	if len(fields) != len(want) {
		t.Fatalf("reflectFields returned %d properties, want %d: %+v", len(fields), len(want), fields)
	}
	for i, name := range want {
		if fields[i].Name != name {
			t.Errorf("property %d = $%s, want $%s", i, fields[i].Name, name)
		}
	}
}

// TestReflectFieldsNoStruct covers the types that carry no property: a named
// scalar a constructor returns, which Time\Duration is, and a struct declaring
// every field unexported, which HTTP\Client's bound type does.
func TestReflectFieldsNoStruct(t *testing.T) {
	t.Parallel()
	if got := reflectFields(reflect.TypeOf(time.Duration(0)), nil); got != nil {
		t.Errorf("a named scalar reports properties %+v, want none", got)
	}
	if got := reflectFields(reflect.TypeOf(&fieldOpaque{}), nil); got != nil {
		t.Errorf("an all-unexported struct reports properties %+v, want none", got)
	}
}

// fieldOpaque is the shape HTTP\Client's bound type takes: it holds a standard
// library client and a setting of its own, and publishes neither.
type fieldOpaque struct {
	client  *http.Client
	timeout time.Duration
}

// TestFieldType covers the type a property read answers with. A field holding
// a registered class reports that class, the way a return value does, and every
// other field maps through the PHP spelling of its Go kind.
func TestFieldType(t *testing.T) {
	t.Parallel()
	classTypes := map[reflect.Type]string{reflect.TypeOf(&fieldBridge{}): "Database\\Bridge"}
	fields := reflectFields(reflect.TypeOf(&fieldHolder{}), classTypes)
	want := map[string]string{"bridge": "Database\\Bridge", "id": "string", "is_readonly": "bool"}
	for _, f := range fields {
		if f.Type != want[f.Name] {
			t.Errorf("$%s is %s, want %s", f.Name, f.Type, want[f.Name])
		}
	}
}

// TestField_Signature covers the property declaration a definition prints.
func TestField_Signature(t *testing.T) {
	t.Parallel()
	f := Field{Name: "is_readonly", GoName: "IsReadonly", Type: "bool"}
	if got, want := f.Signature(), "public bool $is_readonly"; got != want {
		t.Errorf("Signature() = %q, want %q", got, want)
	}
}
