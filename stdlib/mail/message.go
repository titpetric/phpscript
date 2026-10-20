package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
)

// Message is a queued email, with the name of its target server.
//
// Body is the plain text. HTML, when set, travels beside it as the alternative
// a mail client prefers, and ReplyTo, when set, is where a reply goes in place
// of From.
type Message struct {
	Server    string
	Recipient string
	Subject   string
	Body      string
	HTML      string
	ReplyTo   string
}

// buildMessage renders the RFC 5322 message. From carries the configured
// display name ("Name <user@host>"), the envelope sender is the bare address.
//
// A message with no HTML is the text/plain message mail() has always sent,
// byte for byte. One with HTML and text is multipart/alternative, text first
// and HTML last, because RFC 2046 puts the preferred part last; HTML alone is
// a single text/html part. Either way buildMessage encodes the parts
// quoted-printable, so a long line or a non-ASCII byte reaches the server as
// 7-bit lines of 76 characters.
func buildMessage(from string, message Message) (string, error) {
	headers := []string{
		"From: " + header(from),
		"To: " + header(message.Recipient),
		"Subject: " + header(message.Subject),
	}
	if message.ReplyTo != "" {
		headers = append(headers, "Reply-To: "+header(message.ReplyTo))
	}
	headers = append(headers, "MIME-Version: 1.0")

	var body bytes.Buffer
	switch {
	case message.HTML == "":
		headers = append(headers, "Content-Type: text/plain; charset=UTF-8")
		body.WriteString(message.Body)
	case message.Body == "":
		headers = append(headers, "Content-Type: text/html; charset=UTF-8", "Content-Transfer-Encoding: quoted-printable")
		if err := writeQuotedPrintable(&body, message.HTML); err != nil {
			return "", err
		}
	default:
		contentType, err := writeAlternative(&body, message.Body, message.HTML)
		if err != nil {
			return "", err
		}
		headers = append(headers, "Content-Type: "+contentType)
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body.String(), nil
}

// writeAlternative writes text and html as the two parts of a
// multipart/alternative body, returning the Content-Type naming its boundary.
func writeAlternative(w io.Writer, text, html string) (string, error) {
	writer := multipart.NewWriter(w)
	for _, part := range []struct{ mediaType, content string }{
		{"text/plain", text},
		{"text/html", html},
	} {
		partWriter, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.mediaType + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return "", err
		}
		if err := writeQuotedPrintable(partWriter, part.content); err != nil {
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": writer.Boundary()}), nil
}

// writeQuotedPrintable writes content to w through a quoted-printable encoder.
func writeQuotedPrintable(w io.Writer, content string) error {
	encoder := quotedprintable.NewWriter(w)
	if _, err := io.WriteString(encoder, content); err != nil {
		return err
	}
	return encoder.Close()
}

// headerReplacer lives for the process, so header builds none per message.
var headerReplacer = strings.NewReplacer("\r", "", "\n", "")

// header strips CR/LF so script-supplied values cannot inject headers.
func header(value string) string {
	return headerReplacer.Replace(value)
}
