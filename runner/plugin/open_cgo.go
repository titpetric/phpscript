//go:build cgo

package plugin

import (
	goplugin "plugin"
)

// symbols is the part of an opened plugin the loader uses. It exists so the
// non-cgo build has something to return nothing of.
type symbols interface {
	Lookup(name string) (any, error)
}

// openedPlugin adapts *plugin.Plugin, whose Lookup returns a plugin.Symbol
// rather than an any, though the two are the same type.
type openedPlugin struct {
	p *goplugin.Plugin
}

func (o openedPlugin) Lookup(name string) (any, error) { return o.p.Lookup(name) }

// openPlugin dlopens path. The Go runtime refuses a plugin built against a
// different version of any package the two share, so the error is worth
// passing through unwrapped: it names the package that differs.
func openPlugin(path string) (symbols, error) {
	p, err := goplugin.Open(path)
	if err != nil {
		return nil, err
	}
	return openedPlugin{p: p}, nil
}
