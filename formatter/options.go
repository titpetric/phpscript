package formatter

// Options controls AST pretty-printing.
type Options struct {
	// Comments is every comment of the source file, in source order. The AST
	// holds none of them and the printer places them by line: a comment goes
	// out before the first statement that starts below it.
	Comments []Comment

	// GoRuntime spells the file in the go runtime style: no trailing
	// semicolons and member names in lowerCamelCase.
	GoRuntime bool
}
