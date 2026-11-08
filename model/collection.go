package model

// Values flow through phpscript as native Go types (see value.go). A PHP array
// literal is an *Array, but a value handed back by a forwarded Go function is
// just as likely to be a []string, a []map[string]any or a map[string]any:
// the runtime invokes bindings by reflection and boxes whatever they return.
//
// These two interfaces are the contract a consumer reads a collection
// through. modelutil holds the functions that read every shape, an *Array
// included. A binding returns the cheapest representation of its data and pays
// nothing to build an *Array with no reader: an *Array costs a struct, a
// map[any]any and a key slice, plus an interface box per key and per value,
// where the same data as a []string costs one allocation.
//
// *Array remains the right choice for values PHP mutates: the only shape with
// insertion-ordered hybrid int/string keys and support for `$a[] = v`.

// Collection is a value that answers its own length and iteration and a type
// declared outside model reads as an array without being one.
//
// *Array satisfies it. So does any view a caller builds over data it would
// rather not copy into one. Everything that reads a collection accepts it,
// and such a view reaches count(), foreach and the array bindings.
type Collection interface {
	Len() int
	Range(fn func(key, val any) bool)
}

// Keyed is a Collection that reads, writes and removes its entries by name,
// the operations indexing, isset() and unset() need.
type Keyed interface {
	Collection

	// Read answers the value held under key, or nil.
	Read(key string) any

	// Has reports whether anything holds key.
	Has(key string) bool

	// Write records a value under key.
	Write(key string, val any)

	// Delete forgets key.
	Delete(key string)
}
