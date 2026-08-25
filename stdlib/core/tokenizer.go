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

	// The T_* ids token_get_all reports. A script branches on them by name, so
	// every id the tokenizer can emit is registered rather than the handful one
	// caller happened to need: a name that is not a constant reads as null, and
	// comparing a token against null is a branch that never runs. They are
	// constants, not globals, so they resolve inside methods and functions too.
	for name, id := range parser.TokenIDs() {
		rt.SetConst(name, int64(id))
	}
}
