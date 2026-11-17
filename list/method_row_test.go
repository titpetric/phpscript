package list_test

import (
	"encoding/json"
	"testing"

	"github.com/titpetric/phpscript/list"
)

// TestMethodRowJSON covers the field names the API listing writes.
func TestMethodRowJSON(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(list.MethodRow{Class: "Task", Member: "all", Signature: "all()", Doc: "Every task."})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"class":"Task","member":"all","signature":"all()","doc":"Every task."}`
	if string(body) != want {
		t.Fatalf("MethodRow encoded as %s, want %s", body, want)
	}
}
