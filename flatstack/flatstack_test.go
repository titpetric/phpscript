package flatstack_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
)

type contextKey string

type storage struct {
	values map[string]string
	tenant string
}

func (s *storage) Set(_ context.Context, key, value string) { s.values[key] = value }
func (s *storage) Get(_ context.Context, key string) (record, error) {
	value, ok := s.values[key]
	if !ok {
		return record{}, errors.New("missing key")
	}
	return record{Value: value}, nil
}
func (s *storage) Tenant() string { return s.tenant }

type record struct {
	Value string
}

func newStorage(ctx context.Context) (*storage, error) {
	tenant, _ := ctx.Value(contextKey("tenant")).(string)
	return &storage{values: make(map[string]string), tenant: tenant}, nil
}

func TestRunnerCompatibleFastPath(t *testing.T) {
	program, err := parser.Parse(`<?php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $storage->tenant() . ":" . $record->value;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("benchmark program should use flat bytecode: %v", err)
	}

	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	runtime.SetContext(context.WithValue(context.Background(), contextKey("tenant"), "acme"))
	runtime.RegisterConstructor("Storage", newStorage)
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "acme:blue"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestArithmeticUsesFlatBytecode(t *testing.T) {
	program, err := parser.Parse(`<?php echo 20 + 22; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("arithmetic should compile to flat bytecode: %v", err)
	}

	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "42"; got != want {
		t.Fatalf("flat bytecode output = %q, want %q", got, want)
	}
}

func TestUserFunctionDeclarationAndCallFlatBytecode(t *testing.T) {
	program, err := parser.Parse(`<?php
		function add($a, $b) {
			return $a + $b;
		}
		$res = add(15, 27);
		echo $res;
	?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		t.Fatalf("user function declaration should compile to flat bytecode: %v", err)
	}

	var output strings.Builder
	runtime := flatstack.New(&output, flatstack.Options{})
	if err := runtime.Run(program); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "42"; got != want {
		t.Fatalf("user func output = %q, want %q", got, want)
	}
}
