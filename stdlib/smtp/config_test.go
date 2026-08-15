package smtp

import (
	"context"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestNewSMTPBindingInsecure covers the option that turns off certificate
// verification, including the scripted spellings of a truthy value.
func TestNewSMTPBindingInsecure(t *testing.T) {
	tests := map[string]struct {
		value any
		want  bool
	}{
		"unset":       {value: nil, want: false},
		"true":        {value: true, want: true},
		"false":       {value: false, want: false},
		"string true": {value: "true", want: true},
		"string one":  {value: "1", want: true},
		"string off":  {value: "off", want: false},
		"string zero": {value: "0", want: false},
		"empty":       {value: "", want: false},
		"int one":     {value: 1, want: true},
		"int zero":    {value: 0, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			options := model.NewArray()
			options.Set("host", "mail.example.com")
			options.Set("from", "noreply@example.com")
			if test.value != nil {
				options.Set("insecure", test.value)
			}

			client, err := NewSMTPBinding(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if client.config.Insecure != test.want {
				t.Fatalf("insecure = %v, want %v", client.config.Insecure, test.want)
			}
			if got := client.tlsConfig().InsecureSkipVerify; got != test.want {
				t.Fatalf("InsecureSkipVerify = %v, want %v", got, test.want)
			}
		})
	}
}

// TestTLSConfigServerName pins the name the certificate is matched against to
// the configured host, which is what a script overrides when the host answers
// under a name its certificate does not carry.
func TestTLSConfigServerName(t *testing.T) {
	client := NewSMTP(Config{Host: "mail.titpetric.com", Port: 587})
	if got := client.tlsConfig().ServerName; got != "mail.titpetric.com" {
		t.Fatalf("ServerName = %q, want %q", got, "mail.titpetric.com")
	}
}
