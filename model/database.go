package model

// ExtendedDatabaseProvider extends database providers with listing and registration.
type ExtendedDatabaseProvider interface {
	List() []string
	Register(string, string)
}
