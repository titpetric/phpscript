package config_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/stdlib/mail"
)

// TestMail_Validate covers the check every configured server is held to before
// a command runs. A credential the operator got wrong fails the command and
// never the first delivery: on a site whose only sender is a @schedule job,
// that delivery is the middle of the night.
//
// Names are walked in sorted order, so a file with two bad servers reports the
// same one on every run.
func TestMail_Validate(t *testing.T) {
	t.Parallel()
	good := mail.Config{Host: "mail.example.com", From: "noreply@example.com"}

	t.Run("every server is deliverable", func(t *testing.T) {
		servers := config.Mail{"default": good, "marketing": good}
		if err := servers.Validate("phpscript.yml"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no servers configured", func(t *testing.T) {
		if err := (config.Mail{}).Validate("phpscript.yml"); err != nil {
			t.Fatal(err)
		}
		if err := config.Mail(nil).Validate("phpscript.yml"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the error names the file and the server", func(t *testing.T) {
		servers := config.Mail{"marketing": {From: "noreply@example.com"}}
		err := servers.Validate("sites/one/phpscript.yml")
		if err == nil {
			t.Fatal("a server with no host validated")
		}
		if !strings.Contains(err.Error(), "sites/one/phpscript.yml") {
			t.Errorf("err = %q, want it to name the file", err.Error())
		}
		if !strings.Contains(err.Error(), "marketing") {
			t.Errorf("err = %q, want it to name the server", err.Error())
		}
	})

	t.Run("the first name in sorted order is reported", func(t *testing.T) {
		servers := config.Mail{
			"zulu":  {From: "noreply@example.com"},
			"alpha": {From: "noreply@example.com"},
			"mid":   good,
		}
		err := servers.Validate("phpscript.yml")
		if err == nil {
			t.Fatal("two servers with no host validated")
		}
		if !strings.Contains(err.Error(), "alpha") {
			t.Errorf("err = %q, want it to name alpha", err.Error())
		}
	})
}

// TestValidateMailDeclaration covers the refusal a flat mail block gets ahead
// of the typed decode. The generic map is the one place the difference is still
// visible: a typed decode can report that a string is not a struct, which names
// neither the key nor the fix.
//
// Keys are checked in the order flatMailKeys lists them and names in sorted
// order, so one file fails on one line every time.
func TestValidateMailDeclaration(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		declared map[string]any
		want     string
	}{
		{
			name:     "no mail key",
			declared: map[string]any{"autoindex": true},
		},
		{
			name:     "mail is not a map",
			declared: map[string]any{"mail": "default"},
		},
		{
			name:     "named servers",
			declared: map[string]any{"mail": map[string]any{"default": map[string]any{"host": "mail.example.com"}}},
		},
		{
			name:     "a name with nothing under it",
			declared: map[string]any{"mail": map[string]any{"default": nil}},
		},
		{
			name:     "a flat block",
			declared: map[string]any{"mail": map[string]any{"host": "mail.example.com"}},
			want:     "mail.host is not a server name",
		},
		{
			name:     "a flat block naming a password",
			declared: map[string]any{"mail": map[string]any{"password": "secret"}},
			want:     "mail.password is not a server name",
		},
		{
			name:     "a server name holding a scalar",
			declared: map[string]any{"mail": map[string]any{"default": "mail.example.com"}},
			want:     "mail.default must name the settings of one server",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := config.ValidateMailDeclaration("phpscript.yml", test.declared)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%+v validated", test.declared)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %q, want it to carry %q", err.Error(), test.want)
			}
			if !strings.Contains(err.Error(), "phpscript.yml") {
				t.Errorf("err = %q, want it to name the file", err.Error())
			}
		})
	}
}

// TestValidateMailDeclaration_flatKeyOrder covers the key the refusal names
// when a flat block sets several. flatMailKeys fixes the order, so host is
// reported ahead of password whatever order the decoder walked the map in.
func TestValidateMailDeclaration_flatKeyOrder(t *testing.T) {
	t.Parallel()
	declared := map[string]any{"mail": map[string]any{
		"password": "secret",
		"host":     "mail.example.com",
		"from":     "noreply@example.com",
	}}

	err := config.ValidateMailDeclaration("phpscript.yml", declared)
	if err == nil {
		t.Fatal("a flat block validated")
	}
	if !strings.Contains(err.Error(), "mail.host") {
		t.Errorf("err = %q, want it to name mail.host", err.Error())
	}
}
