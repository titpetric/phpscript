package files

import (
	"os"
	"os/user"
	"strconv"
	"testing"
)

// TestLookupUser covers the argument chown() takes: PHP accepts a user name or
// a numeric uid, and a numeric string is an id. "0" is root in PHP and never a
// user called zero, so lookupUser reads the digits before the name table.
func TestLookupUser(t *testing.T) {
	t.Parallel()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(current.Uid)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		owner any
		want  int
	}{
		{name: "an int64 is the uid", owner: int64(uid), want: uid},
		{name: "an int is the uid", owner: uid, want: uid},
		{name: "a numeric string is an id", owner: strconv.Itoa(uid), want: uid},
		{name: "a name resolves through the user table", owner: current.Username, want: uid},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := lookupUser(test.owner)
			if err != nil {
				t.Fatalf("lookupUser(%#v): %v", test.owner, err)
			}
			if got != test.want {
				t.Errorf("lookupUser(%#v) = %d, want %d", test.owner, got, test.want)
			}
		})
	}
}

// TestLookupGroup covers the same two spellings for chgrp(), against the
// group the running process belongs to.
func TestLookupGroup(t *testing.T) {
	t.Parallel()
	gid := os.Getgid()
	group, err := user.LookupGroupId(strconv.Itoa(gid))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		group any
		want  int
	}{
		{name: "an int64 is the gid", group: int64(gid), want: gid},
		{name: "an int is the gid", group: gid, want: gid},
		{name: "a numeric string is an id", group: strconv.Itoa(gid), want: gid},
		{name: "a name resolves through the group table", group: group.Name, want: gid},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := lookupGroup(test.group)
			if err != nil {
				t.Fatalf("lookupGroup(%#v): %v", test.group, err)
			}
			if got != test.want {
				t.Errorf("lookupGroup(%#v) = %d, want %d", test.group, got, test.want)
			}
		})
	}
}

// TestLookupRefusesOtherTypes covers an argument neither function can read.
// PHP's chown() takes a string or an int and nothing else, so an array or a
// bool answers the error naming the call and leaves the uid at zero: zero is
// root, and a silent zero would change the owner of the file.
func TestLookupRefusesOtherTypes(t *testing.T) {
	t.Parallel()
	for _, value := range []any{nil, true, 1.5, []string{"root"}} {
		if id, err := lookupUser(value); err == nil || id != 0 {
			t.Errorf("lookupUser(%#v) = %d, %v, want 0 and an error", value, id, err)
		}
		if id, err := lookupGroup(value); err == nil || id != 0 {
			t.Errorf("lookupGroup(%#v) = %d, %v, want 0 and an error", value, id, err)
		}
	}
}
