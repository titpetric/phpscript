package config

import (
	"fmt"
	"sort"

	"github.com/titpetric/phpscript/stdlib/mail"
)

// flatMailKeys are the settings of one server. One of them directly under mail
// means the file wrote the single unnamed block phpscript carried before the
// key became a map of named servers.
var flatMailKeys = []string{"host", "port", "username", "password", "from", "insecure"}

// Mail configures the mail servers mail() and `new Mail($name)` send
// through, keyed by the name a script names; "default" is what a script
// naming none gets.
//
// The credentials stay here: nothing a script can call spells a host or a
// password or reads one back. A site declaring a mail block gets the servers it
// named and none of the operator's, which docs/configuration.md records.
type Mail map[string]mail.Config

// Validate checks that every configured server can be delivered through, so a
// credential the operator got wrong fails the command and not the first
// delivery. A @schedule job discovering it at three in the morning is the
// failure mode worth avoiding.
//
// Servers are checked in name order so the reported one does not depend on map
// iteration.
func (m Mail) Validate(filename string) error {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := m[name].Validate(name); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
	}
	return nil
}

// ValidateMailDeclaration rejects a flat mail block before the typed decode
// reaches it, and says where to move it.
//
// It reads the generic map because that is the only place the difference is
// still visible: the decoder can only report that a string is not a struct,
// which names neither the key nor the fix. Keys and names are checked in a
// fixed order, so the same file fails on the same line.
func ValidateMailDeclaration(filename string, declared map[string]any) error {
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
