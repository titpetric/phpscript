package server

import (
	chi "github.com/go-chi/chi/v5"
)

// newRouter returns the mux every handler tree this command builds starts
// from. It is a platform.Router, which is the same interface.
//
// The implementation is named here and nowhere else. A site's router, the
// process router and the ones the tests drive are then one mux, so a request
// that routes one way under test routes that way when it is served.
func newRouter() chi.Router {
	return chi.NewRouter()
}
