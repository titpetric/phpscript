package mail

// Message is a queued email, with the name of the server it was addressed to.
type Message struct {
	Server    string
	Recipient string
	Subject   string
	Body      string
}
