package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/henryhwang/chatbot/internal/types"
)

const (
	// maxHistoryMessages defines the maximum number of messages (user + assistant)
	// to keep in the conversation history sent to the API.
	maxHistoryMessages = 20 // Keep the last 10 turns (user + assistant)
)

// --- Core Query Handler (Handles Streaming) ---

func QueryHandler(messages *[]types.Message, input string, provider types.ModelProvider) {
	apiURL := provider.UrlBase + provider.APIs["chat"] // Ensure "chat" key exists in APIS map
	apiKey := provider.APIKey

	// Append the user's message to the history for context
	*messages = append(*messages, types.Message{Role: "user", Content: input})

	// --- Limit History Size ---
	// Ensure we don't send an excessively long history to the API
	if len(*messages) > maxHistoryMessages {
		// Keep only the last 'maxHistoryMessages' messages
		startIndex := len(*messages) - maxHistoryMessages
		*messages = (*messages)[startIndex:]
		// Optional: Log that truncation happened
		// log.Printf("History truncated to the last %d messages.", maxHistoryMessages)
	}

	// --- Prepare the request payload ---
	// Send the potentially truncated conversation history
	// Enable streaming
	requestBody, err := prepareRequestPayload(provider, messages)
	if err != nil {
		fmt.Printf("\nBot: Error preparing request: %v\n", err)
		// Optional: Remove the last user message if request prep fails
		*messages = (*messages)[:len(*messages)-1]
		return
	}

	// Create the HTTP request
	// Set necessary headers for OpenAI-compatible streaming APIs
	// Crucial for SSE
	req, shouldReturn := prepareRequest(apiURL, requestBody, apiKey)
	if shouldReturn {
		return
	} // Good practice for streaming

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("\nBot: Error contacting LLM: %v\n", err)
		return
	}
	defer resp.Body.Close() // Ensure the response body is closed

	// Check for non-200 status codes (errors)
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body) // Try reading error details
		fmt.Printf("\nBot: Error response from LLM (Status %d): %s\n", resp.StatusCode, string(bodyBytes))
		// Optional: Remove the last user message if the call failed
		// *messages = (*messages)[:len(*messages)-1]
		return
	}

	// --- Process the Streaming Response ---
	// Accumulate final *content* for chat history
	// Default role for the assistant's message
	// Prefix for reasoning output
	// Prefix for the final answer output
	// State: Are we currently printing reasoning chunks?
	// State: Has any reasoning been printed at all this turn?
	// State: Has "Bot: " prefix been printed for the final answer?
	// Process Server-Sent Event (SSE) lines
	// Check for the stream termination signal
	// Exit the loop, stream is finished
	// Unmarshal the JSON data payload of the SSE line
	// Log errors during parsing but try to continue
	// Process the first choice in the response (common case)
	// Get the delta (changes)
	// Capture the assistant's role (usually in the first delta chunk)
	// --- Process Reasoning Field ---
	// Print reasoning prefix only when reasoning starts
	// If content was just being printed, add a newline for separation
	// Now in reasoning mode
	// Mark that some reasoning was output
	// Reset bot prefix flag as we switched mode
	// Stream the reasoning chunk
	// Reasoning is NOT added to fullResponse for history
	// --- Process Content Field ---
	// If switching from reasoning to content, print a newline
	// Newline after reasoning block ends
	// Exited reasoning mode
	// Print the "Bot: " prefix only once before the first content chunk
	// Mark prefix as printed
	// Stream the content chunk
	// Append *only content* to history buffer
	// end if len(streamResp.Choices) > 0
	// end if strings.HasPrefix(line, "data: ")
	fullResponse, scanner, assistantRole, reasoningPrinted, botPrefixPrinted := handleStreamResponse(resp, err) // End scanner loop (for scanner.Scan())

	// --- Cleanup after streaming finishes ---

	// Add a final newline for clean prompt display if anything was printed
	if botPrefixPrinted || reasoningPrinted {
		fmt.Println()
	} else {
		// Handle cases where stream ended early or with no valid data
		fmt.Println("\nBot: Received no response content.")
	}

	// Check for errors during scanning (e.g., network issues)
	if err := scanner.Err(); err != nil {
		fmt.Printf("\nBot: Error reading stream: %v\n", err)
		// Don't add potentially incomplete response to history
		return
	}

	// Add the complete assistant message (content only) to the history
	if fullResponse.Len() > 0 {
		finalMessage := types.Message{Role: assistantRole, Content: fullResponse.String()}
		*messages = append(*messages, finalMessage)
	} else if !reasoningPrinted {
		// Only show this message if NO reasoning AND NO content was generated
		fmt.Println("Bot: Finished processing, but no text content was generated.")
	}
}

func handleStreamResponse(resp *http.Response, err error) (strings.Builder, *bufio.Scanner, string, bool, bool) {
	var fullResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	assistantRole := "assistant"
	reasoningPrefix := "🤔 Reasoning: "
	botPrefix := "Bot: "
	currentlyReasoning := false
	reasoningPrinted := false
	botPrefixPrinted := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				break
			}

			var streamResp types.OpenAIStreamResponse
			err = json.Unmarshal([]byte(data), &streamResp)
			if err != nil {

				log.Printf("Error unmarshalling stream data: %v. Data: '%s'", err, data)
				continue
			}

			if len(streamResp.Choices) > 0 {
				delta := streamResp.Choices[0].Delta

				if delta.Role != "" {
					assistantRole = delta.Role
				}

				if delta.Reasoning != "" {

					if !currentlyReasoning {

						if botPrefixPrinted {
							fmt.Println()
						}
						fmt.Print(reasoningPrefix)
						currentlyReasoning = true
						reasoningPrinted = true
						botPrefixPrinted = false
					}
					fmt.Print(delta.Reasoning)

				}

				if delta.Content != "" {

					if currentlyReasoning {
						fmt.Println()
						currentlyReasoning = false
					}

					if !botPrefixPrinted {
						fmt.Print(botPrefix)
						botPrefixPrinted = true
					}
					fmt.Print(delta.Content)
					fullResponse.WriteString(delta.Content)
				}
			}
		}
	}
	return fullResponse, scanner, assistantRole, reasoningPrinted, botPrefixPrinted
}

func prepareRequestPayload(provider types.ModelProvider, messages *[]types.Message) ([]byte, error) {
	requestPayload := types.OpenAIRequest{
		Model:    provider.Model,
		Messages: *messages,
		Stream:   true,
	}

	requestBody, err := json.Marshal(requestPayload)
	return requestBody, err
}

func prepareRequest(apiURL string, requestBody []byte, apiKey string) (*http.Request, bool) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		fmt.Printf("\nBot: Error creating request: %v\n", err)
		return nil, true
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")
	return req, false
}
