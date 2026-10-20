package mail

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
	"testing"
)

// TestBuildMessage_Plain pins the message mail() has always sent, byte for
// byte: a message with no HTML part takes none of the multipart rendering, so a
// script that never passes $html sees no change on the wire.
func TestBuildMessage_Plain(t *testing.T) {
	t.Parallel()
	got, err := buildMessage("Example <noreply@example.com>", Message{
		Recipient: "a@example.com",
		Subject:   "Hello",
		Body:      "Body line",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "From: Example <noreply@example.com>\r\n" +
		"To: a@example.com\r\n" +
		"Subject: Hello\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"Body line"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

// TestBuildMessage_Alternative covers the structure a mail client reads: one
// multipart/alternative body holding the text first and the HTML last, because
// RFC 2046 puts the part a client prefers last, each declared UTF-8 and
// quoted-printable, and nothing after them.
func TestBuildMessage_Alternative(t *testing.T) {
	t.Parallel()
	raw, err := buildMessage("noreply@example.com", Message{
		Recipient: "a@example.com",
		Subject:   "Week sent back",
		Body:      "Your week was sent back.\nOpen it to see why.",
		HTML:      "<p>Your week was sent back.</p>\n<p><a href=\"https://example.com/week\">Open it</a></p>",
		ReplyTo:   "lead@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := parse(t, raw)
	if got := message.Header.Get("Reply-To"); got != "lead@example.com" {
		t.Errorf("Reply-To = %q, want %q", got, "lead@example.com")
	}
	if got := message.Header.Get("MIME-Version"); got != "1.0" {
		t.Errorf("MIME-Version = %q, want 1.0", got)
	}

	parts := readParts(t, message, "multipart/alternative")
	want := []struct{ contentType, content string }{
		{"text/plain; charset=UTF-8", "Your week was sent back.\r\nOpen it to see why."},
		{"text/html; charset=UTF-8", "<p>Your week was sent back.</p>\r\n<p><a href=\"https://example.com/week\">Open it</a></p>"},
	}
	if len(parts) != len(want) {
		t.Fatalf("parts = %d, want %d", len(parts), len(want))
	}
	for i, part := range parts {
		if part.contentType != want[i].contentType {
			t.Errorf("part %d Content-Type = %q, want %q", i, part.contentType, want[i].contentType)
		}
		if part.encoding != "quoted-printable" {
			t.Errorf("part %d Content-Transfer-Encoding = %q, want quoted-printable", i, part.encoding)
		}
		if part.decoded != want[i].content {
			t.Errorf("part %d decodes to %q, want %q", i, part.decoded, want[i].content)
		}
	}
}

// TestBuildMessage_HTMLOnly covers HTML with no text: one text/html part, with
// no alternative wrapped around a single choice.
func TestBuildMessage_HTMLOnly(t *testing.T) {
	t.Parallel()
	raw, err := buildMessage("noreply@example.com", Message{
		Recipient: "a@example.com",
		Subject:   "Hello",
		HTML:      "<p>Héllo</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := parse(t, raw)
	if got := message.Header.Get("Content-Type"); got != "text/html; charset=UTF-8" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if got := message.Header.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Fatalf("Content-Transfer-Encoding = %q, want quoted-printable", got)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "<p>Héllo</p>" {
		t.Fatalf("body decodes to %q", decoded)
	}
}

// TestBuildMessage_Encoding covers what buildMessage owes a relay: 7-bit lines
// of at most 76 characters ending in CRLF, whatever the script wrote. A relay
// rejects or breaks a single line past the 998 octets RFC 5321 allows; a raw
// UTF-8 byte in a part is only safe when the relay speaks 8BITMIME. Both parts
// still decode back to what the script sent.
func TestBuildMessage_Encoding(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Žižek naïve café € 😀 ", 120)
	text := long + "\n.\nFrom here on   \ntrailing tab\t"
	html := "<p>" + long + "</p>"
	raw, err := buildMessage("noreply@example.com", Message{
		Recipient: "a@example.com",
		Subject:   "Encoding",
		Body:      text,
		HTML:      html,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, body, _ := strings.Cut(raw, "\r\n\r\n")
	for i := 0; i < len(body); i++ {
		if c := body[i]; c >= 0x80 || c == 0 {
			t.Fatalf("body byte %d is %#x, want 7-bit", i, c)
		}
		if body[i] == '\n' && (i == 0 || body[i-1] != '\r') {
			t.Fatalf("body byte %d is a bare LF", i)
		}
	}
	for i, line := range strings.Split(body, "\r\n") {
		if len(line) > 76 {
			t.Fatalf("line %d is %d characters, want 76 at most: %q", i, len(line), line)
		}
	}

	parts := readParts(t, parse(t, raw), "multipart/alternative")
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if want := strings.ReplaceAll(text, "\n", "\r\n"); parts[0].decoded != want {
		t.Errorf("text decodes to %q, want %q", parts[0].decoded, want)
	}
	if parts[1].decoded != html {
		t.Errorf("html decodes to %q, want %q", parts[1].decoded, html)
	}
}

// TestBuildMessage_HeaderInjection covers every header a script or a host
// supplies. A CR or LF in any of them would start a header of the script's
// choosing, a Bcc being the one that turns a contact form into a relay, so
// header strips both and the value stays on its own line.
func TestBuildMessage_HeaderInjection(t *testing.T) {
	t.Parallel()
	const smuggled = "\r\nBcc: everyone@example.net\nX-Injected: 1"
	for _, test := range []struct {
		name    string
		from    string
		message Message
	}{
		{name: "From", from: "noreply@example.com" + smuggled, message: Message{Recipient: "a@example.com"}},
		{name: "To", from: "noreply@example.com", message: Message{Recipient: "a@example.com" + smuggled}},
		{name: "Subject", from: "noreply@example.com", message: Message{Recipient: "a@example.com", Subject: "Hello" + smuggled}},
		{name: "Reply-To", from: "noreply@example.com", message: Message{Recipient: "a@example.com", ReplyTo: "lead@example.com" + smuggled}},
		{name: "Reply-To, multipart", from: "noreply@example.com", message: Message{Recipient: "a@example.com", Body: "t", HTML: "<p>h</p>", ReplyTo: "lead@example.com" + smuggled}},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := buildMessage(test.from, test.message)
			if err != nil {
				t.Fatal(err)
			}
			message := parse(t, raw)
			for _, injected := range []string{"Bcc", "X-Injected"} {
				if values, ok := message.Header[injected]; ok {
					t.Errorf("message carries an injected %s header: %q", injected, values)
				}
			}
		})
	}
}

// part is one decoded body part of a multipart message.
type part struct {
	contentType string
	encoding    string
	decoded     string
}

// parse reads a rendered message back the way a receiving client would.
func parse(t *testing.T, raw string) *netmail.Message {
	t.Helper()
	message, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("the rendered message does not parse: %v\n%s", err, raw)
	}
	return message
}

// readParts checks the message is of mediaType and returns each part with its
// declared headers and its quoted-printable body decoded.
func readParts(t *testing.T, message *netmail.Message, mediaType string) []part {
	t.Helper()
	gotType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if gotType != mediaType || params["boundary"] == "" {
		t.Fatalf("Content-Type = %q %v, want %s with a boundary", gotType, params, mediaType)
	}

	var parts []part
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		raw, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			return parts
		}
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := io.ReadAll(quotedprintable.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, part{
			contentType: raw.Header.Get("Content-Type"),
			encoding:    raw.Header.Get("Content-Transfer-Encoding"),
			decoded:     string(decoded),
		})
	}
}
