package server

import (
	chi "github.com/go-chi/chi/v5"
)

// newRouter returns a chi router as a platform.Router and names chi in this one place.
func newRouter() chi.Router {
	return chi.NewRouter()
}
