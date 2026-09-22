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

// Mail configures the mail servers mail() and `new Mail($name)` deliver
// through, keyed by the name a script asks for; "default" is the one a script
// that names none gets. With none configured, mail() still exists and fails
// catchably naming the server it looked for.
//
// The credentials stay here. Nothing a script can call spells a host or a
// password, and nothing it can call reads one back or lists what is
// configured: the map is handed to a provider, which reads a server's settings
// at the moment of a delivery and returns only the outcome.
//
// A virtual host that declares a mail block of its own gets only the servers it
// named, not the operator's with its own written over them. A map replaces
// where the struct this used to be merged field by field, which had a site
// inheriting the operator's password whenever it set only a host.
type Mail map[string]mail.Config

// Validate checks that every configured server can be delivered through, so a
// credential the operator got wrong fails the command rather than the first
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
// reaches it.
//
// It reads the generic map rather than the decoded configuration because that
// is the only place the difference is still visible: by the time goccy has
// tried to read "mail.example.com" as a Config it has nothing to say but that
// it cannot unmarshal a string into a struct, which names neither the key nor
// what to write instead. A file that carries the old block is told how to move
// it.
//
// The keys are checked in slice order and the names in sorted order, so the
// same file always fails on the same line.
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
