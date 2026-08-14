package status

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func TestDiskStoragePutGetRoundtrip(t *testing.T) {
	d, err := NewDiskStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := &Request{ID: testULID, Request: "GET /x", Duration: 3 * time.Millisecond}
	if err := d.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	rc, err := d.Get(context.Background(), testULID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var got Request
	if err := json.NewDecoder(rc).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != rec.ID || got.Request != rec.Request {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestDiskStorageRejectsInvalidIDs(t *testing.T) {
	dir := t.TempDir()
	d, err := NewDiskStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "../../etc/passwd", "short", strings.Repeat("A", 27), "01arz3ndektsv4rrffq69g5fav"} {
		if err := d.Put(context.Background(), &Request{ID: id}); err == nil {
			t.Errorf("Put(%q) accepted", id)
		}
		if _, err := d.Get(context.Background(), id); err == nil {
			t.Errorf("Get(%q) accepted", id)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("directory not empty after rejected ids: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "etc")); !os.IsNotExist(err) {
		t.Fatalf("traversal artifact: %v", err)
	}
}

func TestSampledBounds(t *testing.T) {
	if !sampled(100) || !sampled(150) {
		t.Fatal("sampling >= 100 must always write")
	}
	if sampled(0) || sampled(-1) {
		t.Fatal("sampling <= 0 must never write")
	}
	hits := 0
	for i := 0; i < 10000; i++ {
		if sampled(0.5) {
			hits++
		}
	}
	// 0.5% of 10000 = ~50; accept a generous band.
	if hits == 0 || hits > 300 {
		t.Fatalf("sampled(0.5) hit %d/10000, outside plausible band", hits)
	}
}

func TestFinishWritesSampledTrace(t *testing.T) {
	dir := t.TempDir()
	s := NewModule(Options{RingBufferSize: 4, TopRequests: 5, Driver: "disk", Path: dir, Sampling: 100})
	handler := s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello", nil))
	id := rec.Header().Get("Request-Id")
	if id == "" {
		t.Fatal("no Request-Id issued")
	}
	if _, err := os.Stat(filepath.Join(dir, id+".json")); err != nil {
		t.Fatalf("trace not written: %v", err)
	}
}

func TestFinishSamplingZeroWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s := NewModule(Options{RingBufferSize: 4, TopRequests: 5, Driver: "disk", Path: dir, Sampling: 0})
	handler := s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("sampling=0 wrote %d files", len(entries))
	}
}

// TestDetailPermalinkFromStorage proves the permalink case of the issue:
// the record is gone from the in-memory ring but survives on disk; JSON
// requests get the stored file streamed straight into the body.
func TestDetailPermalinkFromStorage(t *testing.T) {
	dir := t.TempDir()
	s := NewModule(Options{RingBufferSize: 4, TopRequests: 5, Driver: "disk", Path: dir, Sampling: 100})
	rec := &Request{ID: testULID, Request: "GET /old", ResponseStatus: 200}
	if err := s.storage.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	// Not in history (fresh module) — must come from storage.
	req := httptest.NewRequest(http.MethodGet, ServerStatusDetailPath+testULID, nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	var got Request
	if err := json.Unmarshal(body, &got); err != nil || got.ID != testULID {
		t.Fatalf("streamed permalink wrong: %v %s", err, body)
	}
	// Unknown id still 404s.
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, ServerStatusDetailPath+"01ARZ3NDEKTSV4RRFFQ69G5FAX", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing id code = %d", w.Code)
	}
}
