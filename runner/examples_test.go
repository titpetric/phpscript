package runner_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

func ExampleContext() {
	req := httptest.NewRequest(http.MethodGet, "/users/42?tab=profile", nil)
	req.Header.Set("X-Request-Id", "abc123")
	req.Pattern = "GET /users/{id}"
	req.SetPathValue("id", "42")

	ctx := runner.FromRequest(req)
	ctx.Header("X-Powered-By: phpscript")

	fmt.Println(ctx.Get["tab"])
	fmt.Println(ctx.Path["id"])
	fmt.Println(ctx.Headers["X-Request-Id"])
	fmt.Println(ctx.ResponseHeaders().Get("X-Powered-By"))

	// Output:
	// profile
	// 42
	// abc123
	// phpscript
}

func ExampleTranspiler() {
	t := runner.NewTranspiler()

	src, vars, err := t.Transpile(&model.Binary{
		Op: ".",
		Left: &model.Var{
			Name: "greeting",
		},
		Right: &model.Lit{
			Value: " world",
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(src)
	fmt.Println(vars)

	// Output:
	// __concat(v_greeting, " world")
	// [greeting]
}
