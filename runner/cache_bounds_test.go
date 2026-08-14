package runner_test

import (
	"fmt"
	"testing"

	"github.com/expr-lang/expr/vm"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

func TestIncludeCacheCapacityBounds(t *testing.T) {
	cache := runner.NewIncludeCacheWithCapacity(5)

	if cache.Len() != 0 {
		t.Fatalf("expected empty cache, got len %d", cache.Len())
	}

	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("/path/script%d.php", i)
		cache.Set(path, &model.Program{})
	}

	if cache.Len() > 5 {
		t.Fatalf("expected cache len <= 5, got %d", cache.Len())
	}

	cache.Clear()
	if cache.Len() != 0 {
		t.Fatalf("expected 0 after Clear(), got %d", cache.Len())
	}
}

func TestExprCacheCapacityBounds(t *testing.T) {
	cache := runner.NewExprCacheWithCapacity(5)

	if cache.Len() != 0 {
		t.Fatalf("expected empty cache, got len %d", cache.Len())
	}

	for i := 0; i < 10; i++ {
		src := fmt.Sprintf("src_expr_%d", i)
		cache.SetSource(src, &vm.Program{})
	}

	if cache.Len() > 5 {
		t.Fatalf("expected cache len <= 5, got %d", cache.Len())
	}

	cache.Clear()
	if cache.Len() != 0 {
		t.Fatalf("expected 0 after Clear(), got %d", cache.Len())
	}
}
