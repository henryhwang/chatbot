package conversation

import (
	"log"

	"github.com/henryhwang/chatbot/internal/types"
)

const (
	// maxHistoryMessages defines the maximum number of messages (user + assistant)
	// to keep in the conversation history sent to the API.
	// Keeping the last 10 turns (user + assistant).
	maxHistoryMessages = 20
)

// Conversation manages the history of messages in a chat session.
type Conversation struct {
	Messages []types.Message
}

// NewConversation creates a new Conversation instance.
// Optionally initializes with a system message.
func NewConversation(initialMessages ...types.Message) *Conversation {
	c := &Conversation{
		Messages: make([]types.Message, 0, maxHistoryMessages+1), // Pre-allocate slightly
	}
	c.Messages = append(c.Messages, initialMessages...)
	return c
}

// AddMessage appends a new message to the conversation history
// and truncates the history if it exceeds the maximum limit.
func (c *Conversation) AddMessage(role, content string) {
	c.Messages = append(c.Messages, types.Message{Role: role, Content: content})
	c.truncateHistory()
}

// GetMessages returns the current slice of messages, suitable for sending to the API.
func (c *Conversation) GetMessages() []types.Message {
	// Return a copy to prevent external modification of the internal slice?
	// For now, returning the direct slice is simpler and likely sufficient.
	// If more complex state management is needed later, this could return a copy.
	return c.Messages
}

// truncateHistory ensures the conversation history does not exceed the maximum length.
func (c *Conversation) truncateHistory() {
	if len(c.Messages) > maxHistoryMessages {
		startIndex := len(c.Messages) - maxHistoryMessages
		c.Messages = c.Messages[startIndex:]
		log.Printf("History truncated to the last %d messages.", maxHistoryMessages)
	}
}
