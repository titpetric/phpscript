package core

import (
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// init contributes token_get_all and token_name to stdlib.Register.
func init() {
	runner.RegisterBinding(registerTokenizer)
}

func registerTokenizer(rt *runner.Runtime) {
	rt.RegisterFunc("token_get_all", parser.TokenGetAll)
	// token_name returns the name of token $id, such as "T_VARIABLE"; an unknown id returns "UNKNOWN".
	rt.RegisterFunc("token_name", func(id int64) string { return parser.TokenName(int(id)) })

	// Bare T_* constants used by Compiler::_split_exp. They are constants, not
	// globals, so they resolve inside methods/functions too (PHP semantics).
	rt.SetConst("T_VARIABLE", int64(parser.T_VARIABLE))
	rt.SetConst("T_OBJECT_OPERATOR", int64(parser.T_OBJECT_OPERATOR))
	rt.SetConst("T_STRING", int64(parser.T_STRING))
}
