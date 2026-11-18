package server

import (
	chi "github.com/go-chi/chi/v5"
)

// newRouter returns chi as a platform.Router, naming chi in one place.
func newRouter() chi.Router {
	return chi.NewRouter()
}
