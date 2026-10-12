package model

// Func is a callable value: either a user-defined PHP function (Decl set) or a
// host Go function (Go set). Registered host functions use Go.
type Func struct {
	Decl *FuncDecl
	Go   any // an arbitrary Go func, invoked via reflection by the runtime
}
