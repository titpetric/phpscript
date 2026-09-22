package mail

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
)

var _ model.MailProvider = (*Memory)(nil)

// Message is a queued email, with the name of the server it was addressed to.
type Message struct {
	Server    string
	Recipient string
	Subject   string
	Body      string
}

// Memory is a Provider that appends messages to an in-memory queue instead
// of delivering them. A host binds one through runner.Options.Mail to capture
// what scripts send: tests, local runs, dry runs.
//
// It is a provider rather than a sender spliced in further down, so a test
// using it still goes through name resolution and still exercises the refusal
// a script gets for a server nobody configured.
type Memory struct {
	mu       sync.Mutex
	servers  map[string]bool
	messages []Message
}

// NewMemory creates an empty in-memory provider configured for the given
// server names. Naming none configures every name, which is the capture
// everything a dry run wants; naming some lets a test assert the refusal for
// the rest.
func NewMemory(names ...string) *Memory {
	result := &Memory{}
	if len(names) == 0 {
		return result
	}
	result.servers = make(map[string]bool, len(names))
	for _, name := range names {
		result.servers[strings.ToLower(name)] = true
	}
	return result
}

// Configured reports why name cannot be delivered through, and nil when it can.
func (m *Memory) Configured(name string) error {
	if name == "" {
		name = DefaultName
	}
	if m.servers == nil || m.servers[strings.ToLower(name)] {
		return nil
	}
	return fmt.Errorf("no configuration found for mail server: %s", name)
}

// Send queues a message, reporting the same refusal a real delivery would for a
// server this provider does not hold.
func (m *Memory) Send(_ context.Context, name, recipient, subject, body string) error {
	if name == "" {
		name = DefaultName
	}
	if err := m.Configured(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
		Server:    name,
		Recipient: recipient,
		Subject:   subject,
		Body:      body,
	})
	return nil
}

// Messages returns the queued messages in send order.
func (m *Memory) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message{}, m.messages...)
}

// Next pops the oldest queued message, reporting whether one was queued.
func (m *Memory) Next() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return Message{}, false
	}
	message := m.messages[0]
	m.messages = m.messages[1:]
	return message, true
}

// Reset empties the queue.
func (m *Memory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}
