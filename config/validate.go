package config

import (
	"fmt"
)

// Validate reports whatever would stop a server from starting under this
// configuration, checking nothing that would start one.
//
// filename names the file in errors from blocks that carry no name of their
// own. root is the application root a single-root server would serve, and is
// refused beside a virtual host list, which names its own.
func (c Config) Validate(filename, root string) error {
	if err := c.Test.Validate(filename); err != nil {
		return err
	}
	if err := c.Telemetry.Validate(); err != nil {
		return err
	}

	if len(c.VirtualHost) > 0 {
		if root != "" {
			return ErrRootWithVirtualHosts(root)
		}
		return c.ValidateVirtualHosts()
	}

	if root == "" {
		root = "."
	}
	return c.ValidateRoot(root)
}

// ErrRootWithVirtualHosts is the refusal an application root gets when the
// configuration lists virtual hosts.
func ErrRootWithVirtualHosts(root string) error {
	return fmt.Errorf("server: the configuration lists virtual hosts, which name their own roots; %q on the command line has no virtual host to belong to", root)
}
