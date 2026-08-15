package smtp

import (
	"sync"
)

// Message is a queued email.
type Message struct {
	Recipient string
	Subject   string
	Body      string
}

// Memory is a Sender that appends messages to an in-memory queue instead of
// delivering them. Hosts bind one with SenderContext to capture what scripts
// send (tests, local runs, dry runs).
type Memory struct {
	mu       sync.Mutex
	messages []Message
}

// NewMemory creates an empty in-memory sender.
func NewMemory() *Memory {
	return &Memory{}
}

// Send queues a message.
func (m *Memory) Send(recipient, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
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
