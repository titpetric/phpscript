package config

import (
	"fmt"
	"sort"

	"github.com/titpetric/phpscript/stdlib/ps/mail"
)

// flatMailKeys are the settings that mark a legacy unnamed mail block.
var flatMailKeys = []string{"host", "port", "username", "password", "from", "insecure"}

// Mail configures the mail servers mail() and `new Mail($name)` send
// through, keyed by the name a script names; "default" serves a script naming
// none.
//
// The credentials stay here; no script can spell a host or a password or
// read one back. A site declaring a mail block gets the servers it named, and
// inherits none from the operator; docs/configuration.md records this.
type Mail map[string]mail.Config

// Check reports a configured server mail cannot leave through, walking the
// names in sorted order, never in map order.
func (m Mail) Check(filename string) error {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := m[name].Check(name); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
	}
	return nil
}

// CheckMailDeclaration rejects a flat mail block before the typed decode
// reaches it. It says where to move the block.
//
// It reads the generic map because that is the only place the difference is
// still visible: the decoder can only report that a string is not a struct,
// naming no key or fix. The walk takes keys and names in a fixed order, and
// the same file fails on the same line.
func CheckMailDeclaration(filename string, declared map[string]any) error {
	servers, ok := declared["mail"].(map[string]any)
	if !ok {
		return nil
	}

	for _, key := range flatMailKeys {
		if _, ok := servers[key]; ok {
			return fmt.Errorf("%s: mail.%s is not a server name: mail is a map of named servers, so a flat block moves under a name such as %q", filename, key, "default")
		}
	}

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if servers[name] == nil {
			continue
		}
		if _, ok := servers[name].(map[string]any); !ok {
			return fmt.Errorf("%s: mail.%s must name the settings of one server", filename, name)
		}
	}

	return nil
}
