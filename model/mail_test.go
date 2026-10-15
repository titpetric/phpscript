package model

import (
	"reflect"
	"testing"
)

// TestMailProvider covers the promise the interface's shape carries: nothing
// here answers a credential. Every method answers an error alone, so a host
// reads the host name, the username and the password inside Send, uses them for
// the one delivery and drops them; a getter would put them on the runtime,
// where a script could reach them.
//
// Configured is separate from Send so `new Mail` reports a server nobody
// configured at construction, ahead of the first send some hours later. A
// provider's servers are fixed when it is built, so the verdict and the
// resolution Send makes read one set.
func TestMailProvider(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*MailProvider)(nil)).Elem()
	errType := reflect.TypeOf((*error)(nil)).Elem()

	want := map[string]string{
		"Configured": "func(string) error",
		"Send":       "func(context.Context, string, string, string, string) error",
	}
	if got := typ.NumMethod(); got != len(want) {
		t.Errorf("MailProvider declares %d methods, want %d", got, len(want))
	}
	for name, signature := range want {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Errorf("MailProvider declares no %s", name)
			continue
		}
		if got := method.Type.String(); got != signature {
			t.Errorf("%s is %s, want %s", name, got, signature)
		}
	}

	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if got := method.Type.NumOut(); got != 1 {
			t.Errorf("%s answers %d values, want 1: a credential has no way out of a provider", method.Name, got)
			continue
		}
		if got := method.Type.Out(0); got != errType {
			t.Errorf("%s answers %s, want error", method.Name, got)
		}
	}
}
