package config

import (
	"fmt"
)

// Check reports whatever would stop a server from starting under this
// configuration, checking nothing that would start one.
//
// filename names the file in errors from blocks that carry no name of their
// own. root is the application root a single-root server would serve, and the
// check refuses it beside a virtual host list, which names its own.
func (c Config) Check(filename, root string) error {
	if err := c.Test.Check(filename); err != nil {
		return err
	}
	if err := c.Telemetry.Check(); err != nil {
		return err
	}

	if len(c.VirtualHost) > 0 {
		if root != "" {
			return ErrRootWithVirtualHosts(root)
		}
		return c.CheckVirtualHosts()
	}

	if root == "" {
		root = "."
	}
	return c.CheckRoot(root)
}

// ErrRootWithVirtualHosts is the refusal an application root gets when the
// configuration lists virtual hosts.
func ErrRootWithVirtualHosts(root string) error {
	return fmt.Errorf("server: the configuration lists virtual hosts, which name their own roots; %q on the command line has no virtual host to belong to", root)
}
