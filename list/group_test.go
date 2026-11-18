package list_test

import (
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/list"
)

// TestGroupByDirKeepsDiscoveryOrder holds the bucketing contract: a folder
// groups by the first path that named it. Later paths of the same folder
// join that group even when another folder came between. A bare filename lands
// under ".".
func TestGroupByDirKeepsDiscoveryOrder(t *testing.T) {
	t.Parallel()
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
