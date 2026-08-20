package server

import (
	"net"
	"net/http"
	"strings"
)

// hostMux dispatches a request to the site that answers its Host. It is what
// makes one process serve several websites: each site is a router of its own,
// built from its own application root and its own configuration, and the Host
// header is the only thing that selects between them.
//
// Matching is exact. A Host nobody claims gets 404 rather than falling through
// to some default site, because in a shared execution environment the wrong
// answer is a request landing in another tenant's code.
type hostMux struct {
	hosts map[string]http.Handler
}

// newHostMux returns a mux serving the given sites. The keys are domains as
// configured; they are normalized here so a lookup never has to be.
func newHostMux(sites map[string]http.Handler) *hostMux {
	hosts := make(map[string]http.Handler, len(sites))
	for domain, handler := range sites {
		hosts[normalizeHost(domain)] = handler
	}
	return &hostMux{hosts: hosts}
}

func (m *hostMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, ok := m.hosts[normalizeHost(r.Host)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
}

// normalizeHost reduces a Host header or a configured domain to the form the
// two compare equal in: lower case, no port, no trailing dot. A client is free
// to send any of "Example.COM", "example.com:8080" or the fully qualified
// "example.com.", and all three name the same site.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	// A bracketed IPv6 literal without a port survives SplitHostPort as is.
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
