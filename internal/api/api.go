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

	"github.com/henryhwang/chatbot/internal/conversation" // Import the new package
	"github.com/henryhwang/chatbot/internal/types"
)

// --- Core Query Handler (Handles Streaming) ---

// QueryHandler sends the user input and conversation history to the LLM API
// and processes the streaming response. It updates the conversation object
// with the assistant's final response.
func QueryHandler(conv *conversation.Conversation, input string, provider types.ModelProvider) error {
	apiURL := provider.UrlBase + provider.APIs["chat"] // Ensure "chat" key exists in APIS map
	apiKey := provider.APIKey

	// Add user message to conversation history (handles truncation internally)
	conv.AddMessage("user", input)

	// --- Prepare the request payload ---
	// Get the messages to send to the API (respecting the API context limit)
	contextForLLM := conv.GetContext()

	requestBody, err := prepareRequestPayload(provider, contextForLLM) // Pass the potentially limited slice
	if err != nil {
		// No need to manually remove the user message here,
		// as it's already correctly added to the conversation history.
		return fmt.Errorf("error preparing request payload: %w", err)
	}

	// Execute the API request and get the response
	resp, err := executeAPIRequest(apiURL, requestBody, apiKey)
	if err != nil {
		// No need to manually remove the user message here.
		return fmt.Errorf("error executing API request: %w", err) // Propagate error
	}
	defer resp.Body.Close()

	// --- Process the Streaming Response ---
	fullResponse, assistantRole, reasoningPrinted, botPrefixPrinted, streamErr := handleStreamResponse(resp.Body) // Pass resp.Body

	// --- Cleanup after streaming finishes ---

	// Add a final newline for clean prompt display if anything was printed
	if botPrefixPrinted || reasoningPrinted {
		fmt.Println()
	} else {
		// Handle cases where stream ended early or with no valid data
		// Only print this if the stream didn't encounter an error itself
		if streamErr == nil {
			fmt.Println("\nBot: Received no response content.")
		}
	}

	// Check for errors during stream processing
	if streamErr != nil {
		// Don't add potentially incomplete response to history if stream errored
		return fmt.Errorf("error reading stream: %w", streamErr) // Propagate stream error
	}

	// Add the complete assistant message (content only) to the conversation history
	// Only add if there was actual content and no stream error
	if fullResponse.Len() > 0 {
		// Use the conversation's method to add the message (handles truncation)
		conv.AddMessage(assistantRole, fullResponse.String())
	} else if !reasoningPrinted {
		// Only show this message if NO reasoning AND NO content was generated, and no stream error
		fmt.Println("Bot: Finished processing, but no text content was generated.")
	}

	return nil // Indicate success
}

// handleStreamResponse processes the SSE stream from the response body.
// It prints reasoning and content chunks directly to stdout and accumulates
// the final content response.
// Returns the accumulated content, final assistant role, flags indicating if
// reasoning/content was printed, and any error encountered during scanning.
func handleStreamResponse(body io.Reader) (strings.Builder, string, bool, bool, error) {
	var fullResponse strings.Builder
	scanner := bufio.NewScanner(body) // Use the passed reader
	assistantRole := "assistant"      // Default role
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
			err := json.Unmarshal([]byte(data), &streamResp)
			if err != nil {
				// Log the error but attempt to continue processing the stream
				log.Printf("Error unmarshalling stream data: %v. Data: '%s'", err, data)
				continue
			}

			if len(streamResp.Choices) > 0 {
				choice := streamResp.Choices[0]
				delta := choice.Delta

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

				// Check for finish reason if needed (optional)
				// if choice.FinishReason != nil {
				//     log.Printf("Stream finished with reason: %s", *choice.FinishReason)
				// }
			}
		}
	}

	// Check for scanner errors after the loop finishes
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stream: %v", err)
		return fullResponse, assistantRole, reasoningPrinted, botPrefixPrinted, err // Return scanner error
	}

	return fullResponse, assistantRole, reasoningPrinted, botPrefixPrinted, nil // No error
}

// executeAPIRequest sends the prepared request to the API endpoint and checks the response status.
func executeAPIRequest(apiURL string, requestBody []byte, apiKey string) (*http.Response, error) {
	req, err := prepareRequest(apiURL, requestBody, apiKey)
	if err != nil {
		// No need to print here, error is returned
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// No need to print here, error is returned
		return nil, fmt.Errorf("failed to contact LLM API: %w", err)
	}

	// Check for non-OK status codes *before* trying to process the body
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close() // Ensure body is closed even on error
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			// Log reading error, but return the original status error
			log.Printf("Error reading error response body: %v", readErr)
			return nil, fmt.Errorf("LLM API returned error status %d (failed to read body)", resp.StatusCode)
		}
		// Return an error with the status code and response body
		return nil, fmt.Errorf("LLM API returned error status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Return the successful response (caller is responsible for closing the body)
	return resp, nil
}

// prepareRequestPayload creates the JSON body for the API request.
// It now accepts a slice of messages directly, not a pointer to a slice.
func prepareRequestPayload(provider types.ModelProvider, messages []types.Message) ([]byte, error) {
	requestPayload := types.OpenAIRequest{
		Model:    provider.Model,
		Messages: messages, // Use the passed slice directly
		Stream:   true,
	}

	requestBody, err := json.Marshal(requestPayload)
	return requestBody, err
}

// prepareRequest creates a new HTTP request object with necessary headers.
func prepareRequest(apiURL string, requestBody []byte, apiKey string) (*http.Request, error) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		// Return error instead of printing and returning bool
		return nil, fmt.Errorf("error creating HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream") // Necessary for SSE
	req.Header.Set("Connection", "keep-alive")    // Good practice for streaming
	return req, nil                               // Return request and nil error
}
