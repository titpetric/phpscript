package list_test

import (
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/list"
)

// TestGroupByDirKeepsDiscoveryOrder holds the bucketing contract: a folder
// groups under the first path that named it, later paths of the same folder
// join it even when another folder came between, and a bare filename lands
// under ".".
func TestGroupByDirKeepsDiscoveryOrder(t *testing.T) {
	groups := list.GroupByDir([]string{
		"a/b.php",
		"a/m/x.php",
		"a/z.php",
		"top.php",
	})

	want := []list.PathGroup{
		{Dir: "a", Indexes: []int{0, 2}},
		{Dir: "a/m", Indexes: []int{1}},
		{Dir: ".", Indexes: []int{3}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("GroupByDir = %#v, want %#v", groups, want)
	}
}
