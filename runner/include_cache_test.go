package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestIncludeCache covers storing a parsed program under its cleaned path. A
// parsed program is treated as immutable: hoisting copies declarations into
// per-runtime maps and statement execution only reads the tree, so one program
// is shared by every caller.
func TestIncludeCache(t *testing.T) {
	cache := NewIncludeCache()
	if got := cache.Len(); got != 0 {
		t.Errorf("Len() = %d on a fresh cache, want 0", got)
	}
	if _, ok := cache.Get("/app/lib.php"); ok {
		t.Error("Get() = true on a fresh cache")
	}

	prog := &model.Program{}
	cache.Set("/app/lib.php", prog)
	got, ok := cache.Get("/app/lib.php")
	if !ok {
		t.Fatal("Get() = false after Set")
	}
	if got != prog {
		t.Error("Get() answered another program")
	}
	if n := cache.Len(); n != 1 {
		t.Errorf("Len() = %d after one Set, want 1", n)
	}

	cache.Clear()
	if n := cache.Len(); n != 0 {
		t.Errorf("Len() = %d after Clear, want 0", n)
	}
}

// TestNewIncludeCacheWithCapacity covers the bound that keeps a long-running
// process from growing one entry per path it ever included. A capacity of zero
// or less reads as the default, so a host passing an unset value gets the
// bound and never an unbounded map.
func TestNewIncludeCacheWithCapacity(t *testing.T) {
	cache := NewIncludeCacheWithCapacity(2)
	cache.Set("/a.php", &model.Program{})
	cache.Set("/b.php", &model.Program{})
	cache.Set("/c.php", &model.Program{})
	if got := cache.Len(); got != 2 {
		t.Errorf("Len() = %d at a capacity of two, want 2", got)
	}

	for _, capacity := range []int{0, -1} {
		if got := NewIncludeCacheWithCapacity(capacity).maxEntries; got != DefaultMaxCacheSize {
			t.Errorf("a capacity of %d reads as %d, want %d", capacity, got, DefaultMaxCacheSize)
		}
	}
	if got := NewIncludeCache().maxEntries; got != DefaultMaxCacheSize {
		t.Errorf("NewIncludeCache() is bounded at %d, want %d", got, DefaultMaxCacheSize)
	}
}

// TestIncludeCache_Account covers the heap a precompile pass reports adding.
// It is zero until a pass runs, as a lazily filled cache reports: the files in
// it were parsed for a request that named them. A negative figure is dropped,
// so a miscounted pass cannot credit the total.
func TestIncludeCache_Account(t *testing.T) {
	cache := NewIncludeCache()
	if got := cache.Precompiled(); got != 0 {
		t.Errorf("Precompiled() = %d before a pass, want 0", got)
	}

	cache.Account(2048)
	cache.Account(1024)
	if got, want := cache.Precompiled(), int64(3072); got != want {
		t.Errorf("Precompiled() = %d after two passes, want %d", got, want)
	}

	cache.Account(-512)
	if got, want := cache.Precompiled(), int64(3072); got != want {
		t.Errorf("Precompiled() = %d after a negative figure, want %d", got, want)
	}

	var absent *IncludeCache
	absent.Account(1024)
	if got := absent.Precompiled(); got != 0 {
		t.Errorf("a nil cache reports %d, want 0", got)
	}
}

// TestIncludeCacheVersion covers the counter a reader deriving something from
// the whole cache holds its copy against. Entry count would not serve: a Set
// replacing a re-parsed file leaves the count where it was, so the derived copy
// would read as fresh.
func TestIncludeCacheVersion(t *testing.T) {
	cache := NewIncludeCache()
	cache.Set("/a.php", &model.Program{})

	entries, version := cache.snapshot()
	if len(entries) != 1 {
		t.Fatalf("the snapshot holds %d entries, want 1", len(entries))
	}
	if cache.changedSince(version) {
		t.Error("changedSince() = true on the version the snapshot was taken at")
	}

	cache.Set("/a.php", &model.Program{})
	if n := cache.Len(); n != 1 {
		t.Fatalf("Len() = %d after replacing one entry, want 1", n)
	}
	if !cache.changedSince(version) {
		t.Error("changedSince() = false after a Set that left the count alone")
	}

	_, replaced := cache.snapshot()
	cache.Clear()
	if !cache.changedSince(replaced) {
		t.Error("changedSince() = false after Clear")
	}
}
