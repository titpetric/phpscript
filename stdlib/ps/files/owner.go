package files

import (
	"fmt"
	"os/user"
	"strconv"
)

// lookupUser resolves the second argument of chown(), which PHP accepts as
// either a user name or a numeric uid, to a uid.
func lookupUser(owner any) (int, error) {
	switch v := owner.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		// A numeric string is an id, not a name: PHP takes "0" as root.
		if id, err := strconv.Atoi(v); err == nil {
			return id, nil
		}
		u, err := user.Lookup(v)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(u.Uid)
	}
	return 0, fmt.Errorf("chown: unsupported user %T", owner)
}

// lookupGroup is lookupUser for chgrp(): a group name or a numeric gid.
func lookupGroup(group any) (int, error) {
	switch v := group.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		if id, err := strconv.Atoi(v); err == nil {
			return id, nil
		}
		g, err := user.LookupGroup(v)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(g.Gid)
	}
	return 0, fmt.Errorf("chgrp: unsupported group %T", group)
}
