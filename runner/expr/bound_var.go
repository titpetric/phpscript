package expr

// BoundVar is one variable the compiled expression reads: the PHP name, the
// bare-name flag Eval's resolution needs, and the slot the value lands in.
type BoundVar struct {
	Name  string
	Const bool
	Slot  int
}
