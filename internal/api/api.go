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

// --- Core Query Handler (Handles Streaming) ---

func QueryHandler(messages *[]types.Message, input string, provider types.ModelProvider) {
	apiURL := provider.UrlBase + provider.APIs["chat"] // Ensure "chat" key exists in APIS map
	apiKey := provider.APIKey

	// Append the user's message to the history for context
	*messages = append(*messages, types.Message{Role: "user", Content: input})

	// Prepare the request payload
	requestPayload := types.OpenAIRequest{
		Model:    provider.Model,
		Messages: *messages, // Send the whole conversation history
		Stream:   true,      // Enable streaming
	}

	requestBody, err := json.Marshal(requestPayload)
	if err != nil {
		fmt.Printf("\nBot: Error preparing request: %v\n", err)
		// Optional: Remove the last user message if request prep fails
		// *messages = (*messages)[:len(*messages)-1]
		return
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		fmt.Printf("\nBot: Error creating request: %v\n", err)
		return
	}

	// Set necessary headers for OpenAI-compatible streaming APIs
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream") // Crucial for SSE
	req.Header.Set("Connection", "keep-alive")    // Good practice for streaming

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
	var fullResponse strings.Builder // Accumulate final *content* for chat history
	scanner := bufio.NewScanner(resp.Body)
	assistantRole := "assistant"       // Default role for the assistant's message
	reasoningPrefix := "🤔 Reasoning: " // Prefix for reasoning output
	botPrefix := "Bot: "               // Prefix for the final answer output
	currentlyReasoning := false        // State: Are we currently printing reasoning chunks?
	reasoningPrinted := false          // State: Has any reasoning been printed at all this turn?
	botPrefixPrinted := false          // State: Has "Bot: " prefix been printed for the final answer?

	for scanner.Scan() {
		line := scanner.Text()

		// Process Server-Sent Event (SSE) lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Check for the stream termination signal
			if data == "[DONE]" {
				break // Exit the loop, stream is finished
			}

			// Unmarshal the JSON data payload of the SSE line
			var streamResp types.OpenAIStreamResponse
			err = json.Unmarshal([]byte(data), &streamResp)
			if err != nil {
				// Log errors during parsing but try to continue
				log.Printf("Error unmarshalling stream data: %v. Data: '%s'", err, data)
				continue
			}

			// Process the first choice in the response (common case)
			if len(streamResp.Choices) > 0 {
				delta := streamResp.Choices[0].Delta // Get the delta (changes)

				// Capture the assistant's role (usually in the first delta chunk)
				if delta.Role != "" {
					assistantRole = delta.Role
				}

				// --- Process Reasoning Field ---
				if delta.Reasoning != "" {
					// Print reasoning prefix only when reasoning starts
					if !currentlyReasoning {
						// If content was just being printed, add a newline for separation
						if botPrefixPrinted {
							fmt.Println()
						}
						fmt.Print(reasoningPrefix)
						currentlyReasoning = true // Now in reasoning mode
						reasoningPrinted = true   // Mark that some reasoning was output
						botPrefixPrinted = false  // Reset bot prefix flag as we switched mode
					}
					fmt.Print(delta.Reasoning) // Stream the reasoning chunk
					// Reasoning is NOT added to fullResponse for history
				}

				// --- Process Content Field ---
				if delta.Content != "" {
					// If switching from reasoning to content, print a newline
					if currentlyReasoning {
						fmt.Println()              // Newline after reasoning block ends
						currentlyReasoning = false // Exited reasoning mode
					}
					// Print the "Bot: " prefix only once before the first content chunk
					if !botPrefixPrinted {
						fmt.Print(botPrefix)
						botPrefixPrinted = true // Mark prefix as printed
					}
					fmt.Print(delta.Content)                // Stream the content chunk
					fullResponse.WriteString(delta.Content) // Append *only content* to history buffer
				}
			} // end if len(streamResp.Choices) > 0
		} // end if strings.HasPrefix(line, "data: ")
	} // End scanner loop (for scanner.Scan())

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
