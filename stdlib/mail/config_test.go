package mail

import (
	"fmt"
	"strings"
	"testing"
)

// TestConfigTLS covers the two things the TLS settings decide: the name the
// certificate is matched against, which is the configured host, and whether it
// is checked at all.
func TestConfigTLS(t *testing.T) {
	config := Config{Host: "mail.titpetric.com", Port: 587}
	if got := config.tlsConfig().ServerName; got != "mail.titpetric.com" {
		t.Fatalf("ServerName = %q, want %q", got, "mail.titpetric.com")
	}
	if config.tlsConfig().InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = true, want verification on by default")
	}

	config.Insecure = true
	if !config.tlsConfig().InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = false, want the configured opt-out")
	}
}

// TestConfigValidate covers the checks that fail a command at startup rather
// than a delivery hours later.
func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		config Config
		want   string
	}{
		"missing host": {
			config: Config{From: "noreply@example.com"},
			want:   `mail "default": host is required`,
		},
		"missing from": {
			config: Config{Host: "mail.example.com"},
			want:   `mail "default": from is required`,
		},
		"invalid from": {
			config: Config{Host: "mail.example.com", From: "Example <not an address>"},
			want:   "invalid from address",
		},
		"bare address": {
			config: Config{Host: "mail.example.com", From: "noreply@example.com"},
		},
		"display name": {
			config: Config{Host: "mail.example.com", From: "Example <noreply@example.com>"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.config.Validate("default")
			if test.want == "" {
				if err != nil {
					t.Fatalf("error = %v, want none", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

// TestConfigStringElidesTheCredential pins the redaction. Nothing formats a
// Config today; the point is that a %v somebody adds later cannot be the way a
// password leaves the process.
func TestConfigStringElidesTheCredential(t *testing.T) {
	config := Config{
		Host:     "mail.example.com",
		Port:     587,
		Username: "noreply@example.com",
		Password: "hunter2",
		From:     "Example <noreply@example.com>",
	}

	got := config.String()
	for _, secret := range []string{"hunter2", "noreply@example.com:"} {
		if strings.Contains(got, secret) {
			t.Fatalf("String() = %q, want it to elide %q", got, secret)
		}
	}
	if !strings.Contains(got, "mail.example.com") || !strings.Contains(got, "587") {
		t.Fatalf("String() = %q, want it to name the host and port", got)
	}

	// The same holds through the verb an accidental log line would use.
	if formatted := fmt.Sprintf("%v", config); formatted != got {
		t.Fatalf("%%v = %q, want the redacted form %q", formatted, got)
	}
}
