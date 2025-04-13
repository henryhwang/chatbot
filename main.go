package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// --- Configuration and Provider ---

type ModelProvider struct {
	Provider string
	UrlBase  string
	APIKey   string
	APIs     map[string]string
	Model    string
}

// --- API Request/Response Structures ---

// Request structure for the chat API (used for both streaming and non-streaming)
type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"` // Set to true for streaming
}

// Standard message structure
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// --- Structs specifically for STREAMING response handling ---

// Overall structure of a single SSE data line payload
type OpenAIStreamResponse struct {
	Choices []StreamChoice `json:"choices"`
	// Usage *UsageInfo `json:"usage,omitempty"` // Optional: If API provides usage at the end
}

// Structure of a choice within the stream
type StreamChoice struct {
	Delta        Delta   `json:"delta"`                   // The actual changes in this chunk
	FinishReason *string `json:"finish_reason,omitempty"` // e.g., "stop", "length", "tool_calls"
	// Index *int `json:"index,omitempty"` // Usually 0 for chat completions
}

// Structure of the delta (the changes) in a stream chunk
type Delta struct {
	Role      string `json:"role,omitempty"`              // Assistant's role, usually in the first chunk
	Content   string `json:"content,omitempty"`           // Final answer content chunk
	Reasoning string `json:"reasoning_content,omitempty"` // <<< DeepSeek specific reasoning/thinking chunk
	// ToolCalls []*ToolCall `json:"tool_calls,omitempty"` // Standard tool call mechanism (if supported/needed)
}

// --- Optional: Standard Tool Call Structures (If needed in the future) ---
// type ToolCall struct {
//  Index    *int             `json:"index,omitempty"`
//  ID       string           `json:"id,omitempty"`
//  Type     string           `json:"type,omitempty"` // "function"
//  Function ToolCallFunction `json:"function,omitempty"`
// }
// type ToolCallFunction struct {
//  Name      string `json:"name,omitempty"`
//  Arguments string `json:"arguments,omitempty"`
// }

// --- Main Application Logic ---

func main() {
	provider := getProvider() // Load configuration

	fmt.Println("Welcome to the Chatbot! Type '/exit' to quit.")
	fmt.Println("Using Model:", provider.Model)
	fmt.Println("--------------------------------------------")

	reader := bufio.NewReader(os.Stdin)
	// Initialize conversation history (can add a system message if desired)
	messages := []Message{
		// {Role: "system", Content: "You are a helpful assistant."},
	}

	for {
		fmt.Print("You: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.HasPrefix(input, "/") {
			// Pass provider info to command functions
			runCmd(strings.TrimPrefix(input, "/"), provider)
		} else if input != "" {
			// Handle regular chat query, modifying the message history
			queryHandler(&messages, input, provider)
		}
		// No message for empty input to avoid clutter
	}
}

// --- Core Query Handler (Handles Streaming) ---

func queryHandler(messages *[]Message, input string, provider ModelProvider) {
	apiURL := provider.UrlBase + provider.APIs["chat"] // Ensure "chat" key exists in APIS map
	apiKey := provider.APIKey

	// Append the user's message to the history for context
	*messages = append(*messages, Message{Role: "user", Content: input})

	// Prepare the request payload
	requestPayload := OpenAIRequest{
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
			var streamResp OpenAIStreamResponse
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
		finalMessage := Message{Role: assistantRole, Content: fullResponse.String()}
		*messages = append(*messages, finalMessage)
	} else if !reasoningPrinted {
		// Only show this message if NO reasoning AND NO content was generated
		fmt.Println("Bot: Finished processing, but no text content was generated.")
	}
}

// --- Configuration Loading ---

func getProvider() ModelProvider {
	err := godotenv.Load() // Load .env file if present
	if err != nil {
		// Non-fatal warning, allows using direct env vars
		log.Println("Warning: No .env file found, attempting to use environment variables directly.")
	}

	// Read required environment variables
	providerName := os.Getenv("MODEL_PROVIDER") // Optional name
	apiKey := os.Getenv("API_KEY")
	apiBase := os.Getenv("API_URL_BASE")
	apisString := os.Getenv("APIS") // e.g., "chat:/v1/chat/completions,models:/v1/models"
	model := os.Getenv("MODEL")

	// Validate required variables
	if apiKey == "" {
		log.Fatal("FATAL: API_KEY environment variable not set.")
	}
	if apiBase == "" {
		log.Fatal("FATAL: API_URL_BASE environment variable not set.")
	}
	if apisString == "" {
		log.Fatal("FATAL: APIS environment variable not set (e.g., 'chat:/v1/chat/completions').")
	}
	if model == "" {
		log.Fatal("FATAL: MODEL environment variable not set.")
	}

	// Parse the APIS string into a map
	apis := make(map[string]string)
	for _, apiEntry := range strings.Split(apisString, ",") {
		parts := strings.SplitN(strings.TrimSpace(apiEntry), ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" && value != "" {
				apis[key] = value
			} else {
				log.Printf("Warning: Skipping malformed API entry in APIS env var: '%s'", apiEntry)
			}
		} else if len(parts) == 1 && strings.TrimSpace(parts[0]) != "" {
			log.Printf("Warning: API entry missing path in APIS env var: '%s'. Requires 'key:path' format.", apiEntry)
		}
	}

	// Ensure the crucial 'chat' endpoint is defined
	if _, ok := apis["chat"]; !ok {
		log.Fatal("FATAL: APIS environment variable must contain a 'chat' endpoint (e.g., 'chat:/v1/chat/completions').")
	}

	// Return the configured provider struct
	return ModelProvider{
		Provider: providerName,
		UrlBase:  strings.TrimSuffix(apiBase, "/"), // Remove trailing slash for consistency
		APIKey:   apiKey,
		APIs:     apis,
		Model:    model,
	}
}

// --- Command Handling ---

// Define the type for command functions
type CommandFunc func(...interface{})

// Map commands (strings) to their corresponding functions
var commands = map[string]CommandFunc{
	"list":      listModels,   // List available models from the provider
	"show":      showProvider, // Show current provider configuration details
	"showModel": showModel,    // Show the currently configured model name
	"exit":      exitCmd,      // Exit the application
	"help":      showHelp,     // Show available commands
	// Add new commands here
}

// Executes a command based on user input
func runCmd(command string, args ...interface{}) {
	if cmdFunc, ok := commands[command]; ok {
		cmdFunc(args...) // Pass the arguments (which include the provider)
	} else {
		fmt.Println("Bot: Unknown command:", command)
		showHelp() // Show help on unknown command
	}
}

// --- Command Implementations ---

// Command to list models (if supported by the API)
func listModels(args ...interface{}) {
	if len(args) == 0 {
		fmt.Println("Bot: Internal error: Provider info missing for listModels.")
		return
	}
	provider, ok := args[0].(ModelProvider)
	if !ok {
		fmt.Println("Bot: Internal error: Invalid argument type for listModels.")
		return
	}

	// Check if a specific 'models' endpoint is defined in APIS map
	modelsPath, pathOk := provider.APIs["models"]
	if !pathOk {
		// Fallback to a common default path, adjust if needed
		modelsPath = "/v1/models"
		log.Printf("Warning: 'models' endpoint not explicitly defined in APIS env var, trying default '%s'", modelsPath)
	}
	url := provider.UrlBase + modelsPath

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Bot: Error creating model list request: %v\n", err)
		return
	}
	req.Header.Add("Authorization", "Bearer "+provider.APIKey)
	// Some APIs might require Content-Type even for GET
	// req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Bot: Error fetching models: %v\n", err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("Bot: Error reading models response body: %v\n", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		fmt.Printf("Bot: Error fetching models (Status %d): %s\n", res.StatusCode, string(body))
		return
	}

	// Attempt to pretty-print the JSON response
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, body, "", "  ") // Use two spaces for indentation
	if err == nil {
		fmt.Println("Available Models:\n", prettyJSON.String())
	} else {
		// If not valid JSON or Indent fails, print the raw response
		fmt.Println("Available Models (raw response):\n", string(body))
	}
}

// Command to show current provider configuration
func showProvider(args ...interface{}) {
	if len(args) == 0 {
		fmt.Println("Bot: Internal error: Provider info missing for showProvider.")
		return
	}
	provider, ok := args[0].(ModelProvider)
	if !ok {
		fmt.Println("Bot: Internal error: Invalid argument type for showProvider.")
		return
	}

	fmt.Println("--- Current Provider Configuration ---")
	fmt.Println("Provider Name:", provider.Provider) // Might be empty if not set in env
	fmt.Println("Base URL:", provider.UrlBase)
	fmt.Println("API Key:", "***********"+provider.APIKey[len(provider.APIKey)-4:]) // Mask key
	fmt.Println("Configured Model:", provider.Model)
	fmt.Println("API Endpoints:")
	for key, path := range provider.APIs {
		fmt.Printf("  - %s: %s\n", key, path)
	}
	fmt.Println("------------------------------------")
}

// Command to show the currently configured model
func showModel(args ...interface{}) {
	if len(args) == 0 {
		fmt.Println("Bot: Internal error: Provider info missing for showModel.")
		return
	}
	provider, ok := args[0].(ModelProvider)
	if !ok {
		fmt.Println("Bot: Internal error: Invalid argument type for showModel.")
		return
	}
	fmt.Println("Bot: Current model configured:", provider.Model)
}

// Command to display help information
func showHelp(args ...interface{}) {
	fmt.Println("Available commands:")
	fmt.Println("  /list      - List available models from the provider.")
	fmt.Println("  /show      - Show the current provider configuration.")
	fmt.Println("  /showModel - Show the currently selected model.")
	fmt.Println("  /help      - Display this help message.")
	fmt.Println("  /exit      - Quit the chatbot.")
}

// Command to exit the application
func exitCmd(args ...interface{}) {
	fmt.Println("Bot: Goodbye!")
	os.Exit(0) // Exit gracefully
}
