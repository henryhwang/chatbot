package conversation

import (
	"log"
	"time" // Import time package

	"github.com/henryhwang/chatbot/internal/types"
)

const (
	// maxMessagesForAPI defines the maximum number of messages (user + assistant)
	// to keep in the conversation history *sent to the API*.
	// Keeping the last 10 turns (user + assistant).
	maxMessagesForAPI = 20
)

// Conversation manages the history of messages in a chat session.
type Conversation struct {
	Messages []types.Message
}

// NewConversation creates a new Conversation instance.
// Optionally initializes with a system message.
func NewConversation(initialMessages ...types.Message) *Conversation {
	// Ensure initial messages also have timestamps if needed, or set them here.
	// For simplicity, we assume initial messages might not need precise timestamps
	// or they are set by the caller if required.
	now := time.Now()
	for i := range initialMessages {
		if initialMessages[i].Timestamp.IsZero() {
			initialMessages[i].Timestamp = now // Set timestamp if not already set
		}
	}

	c := &Conversation{
		// Pre-allocate based on typical API limit, but history can grow beyond this
		Messages: make([]types.Message, 0, maxMessagesForAPI+1),
	}
	c.Messages = append(c.Messages, initialMessages...)
	return c
}

// AddMessage appends a new message with the current timestamp to the conversation history.
// History is no longer truncated here.
func (c *Conversation) AddMessage(role, content string) {
	c.Messages = append(c.Messages, types.Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(), // Add timestamp
	})
	// History is no longer truncated here. Full history is preserved.
}

// GetMessagesForAPI returns the most recent slice of messages suitable for sending to the API,
// respecting the maxMessagesForAPI limit, without modifying the full history.
func (c *Conversation) GetMessagesForAPI() []types.Message {
	numMessages := len(c.Messages)
	if numMessages <= maxMessagesForAPI {
		// If the total history is within the limit, return all messages
		return c.Messages
	}
	// Otherwise, return only the last 'maxMessagesForAPI' messages
	startIndex := numMessages - maxMessagesForAPI
	return c.Messages[startIndex:]
}

// truncateHistory is removed as we now keep the full history.
// The GetMessagesForAPI method handles sending only the relevant recent part.
