package conversation

import (
	"errors"
	"strings"
	"time" // Import time package

	"github.com/henryhwang/chatbot/internal/types"
)

func estimateTokens(text string) int {
	const baseCost = 5
	return baseCost + len(text)/4
}

type ContextGenerationStrategy interface {
	Generate(conversation *Conversation) ([]types.Message, error)
}

type SimpleTruncationStrategy struct{}

func (s *SimpleTruncationStrategy) Generate(conversation *Conversation) ([]types.Message, error) {
	fullHistory := conversation.fullHistory
	systemPrompt := conversation.systemPrompt
	maxTokens := conversation.maxTokens
	currentTokens := 0

	if systemPrompt != nil {
		systemTokens := estimateTokens(systemPrompt.Content)
		if systemTokens > maxTokens {
			return nil, errors.New("maxTokens is smaller than the system prompt alone")
		}
		currentTokens += systemTokens
	}

	conversationContext := []types.Message{}
	for i := len(fullHistory) - 1; i >= 0; i-- {
		message := fullHistory[i]
		messageTokens := estimateTokens(message.Content)

		if currentTokens+messageTokens <= maxTokens {
			conversationContext = append(conversationContext, message)
			currentTokens += messageTokens
		} else {
			break
		}
	}

	contextLen := len(conversationContext)
	finalConversation := make([]types.Message, contextLen)
	for i, msg := range conversationContext {
		finalConversation[contextLen-1-i] = msg
	}

	finalContext := []types.Message{}
	if systemPrompt != nil {
		finalContext = append(finalContext, *systemPrompt)
	}
	finalContext = append(finalContext, finalConversation...)

	return finalContext, nil
}

// Conversation manages the history of messages in a chat session.
type Conversation struct {
	systemPrompt *types.Message
	fullHistory  []types.Message
	strategy     ContextGenerationStrategy
	maxTokens    int
}

// NewConversation creates a new Conversation instance.
// Optionally initializes with a system message.
func NewConversation(systemPromptText string, strategy ContextGenerationStrategy, maxTokens int) *Conversation {
	var systemMsg *types.Message
	if strings.TrimSpace(systemPromptText) != "" {
		systemMsg = &types.Message{Timestamp: time.Now(), Role: "system", Content: systemPromptText}
	}
	return &Conversation{
		systemPrompt: systemMsg,
		fullHistory:  []types.Message{},
		strategy:     strategy,
		maxTokens:    maxTokens,
	}
}

// AddMessage appends a new message with the current timestamp to the conversation history.
// History is no longer truncated here.
func (c *Conversation) AddMessage(role, content string) {
	c.fullHistory = append(c.fullHistory, types.Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(), // Add timestamp
	})
}

func (c *Conversation) AddUserMessage(role, content string) {
	c.fullHistory = append(c.fullHistory, types.Message{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now(), // Add timestamp
	})
}

func (c *Conversation) AddAssistantMessage(role, content string) {
	c.fullHistory = append(c.fullHistory, types.Message{
		Role:      "assistant",
		Content:   content,
		Timestamp: time.Now(), // Add timestamp
	})
}

// GetFullHistory returns the most recent slice of messages suitable for sending to the API,
// respecting the maxMessagesForAPI limit, without modifying the full history.
func (c *Conversation) GetFullHistory() []types.Message {
	historyCopy := make([]types.Message, len(c.fullHistory))
	copy(historyCopy, c.fullHistory)

	return historyCopy
}

func (c *Conversation) GetContext() []types.Message {
	context, _ := c.strategy.Generate(c)
	return context
}
