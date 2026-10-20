package server

import (
	chi "github.com/go-chi/chi/v5"
)

// newRouter returns the mux every handler tree this command builds starts
// from. A chi.Router is a platform.Router: the same interface.
//
// Only this function names the implementation. A site's router, the process
// router and the ones the tests drive are then one mux, so a request that
// routes one way under test routes that way in the server.
func newRouter() chi.Router {
	return chi.NewRouter()
}
