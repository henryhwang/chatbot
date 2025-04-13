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

type ModelProvider struct {
	Provider string
	UrlBase  string
	APIKey   string
	APIs     map[string]string
	Model    string
}

type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type OpenAIStreamResponse struct {
	Choices []StreamChoice `json:"choices"`
}

type StreamChoice struct {
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

func main() {
	provider := getProvider()

	fmt.Println("Welcome to the Chatbot! Type '/exit' to quit.")
	fmt.Println("--------------------------------------------")

	reader := bufio.NewReader(os.Stdin)
	messages := []Message{} // keep track of the conversation

	for {
		fmt.Print("You: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.HasPrefix(input, "/") {
			runCmd(strings.TrimPrefix(input, "/"), provider)
		} else if input != "" {
			queryHandler(&messages, input, provider)
		} else {
			fmt.Println("Bot: Hey, say something! I'm ready to chat.")
		}
	}
}

func queryHandler(messages *[]Message, input string, provider ModelProvider) {
	apiURL := provider.UrlBase + provider.APIs["chat"]
	apiKey := provider.APIKey
	*messages = append(*messages, Message{Role: "user", Content: input})

	requestBody, err := json.Marshal(OpenAIRequest{
		Model:    provider.Model,
		Messages: *messages,
		Stream:   true,
	})
	if err != nil {
		fmt.Printf("Bot: Error preparing request: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		fmt.Printf("Bot: Error creating request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream") // Ensure the response is streamed
	req.Header.Set("Connection", "keep-alive")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Bot: Error contacting LLM: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("Bot: Error response from LLM (Status %d): %s\n", resp.StatusCode, string(bodyBytes))
		return
	}
	fmt.Print("Bot: ")               // Print prefix once before streaming starts
	var fullResponse strings.Builder // Accumulate the full response text
	scanner := bufio.NewScanner(resp.Body)
	assistantRole := "assistant" // Default role

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break // Stream finished
			}

			var streamResp OpenAIStreamResponse
			err = json.Unmarshal([]byte(data), &streamResp)
			if err != nil {
				// Log the problematic data and error
				log.Printf("Error unmarshalling stream data: %v. Data: '%s'", err, data)
				continue // Skip this chunk if parsing fails
			}

			if len(streamResp.Choices) > 0 {
				// Capture role if present (usually in the first chunk)
				if streamResp.Choices[0].Delta.Role != "" {
					assistantRole = streamResp.Choices[0].Delta.Role
				}
				// Print the content chunk and add to the full response
				contentChunk := streamResp.Choices[0].Delta.Content
				fmt.Print(contentChunk) // Print immediately
				fullResponse.WriteString(contentChunk)
			}
		}
	} // End scanner loop

	fmt.Println() // Add a newline after the streaming is complete

	if err := scanner.Err(); err != nil {
		fmt.Printf("\nBot: Error reading stream: %v\n", err)
		// Don't add potentially incomplete response to history
		return
	}

	// Add the complete assistant message to the history *after* streaming is done
	if fullResponse.Len() > 0 {
		*messages = append(*messages, Message{Role: assistantRole, Content: fullResponse.String()})
	} else {
		// Handle cases where the stream might have ended without content or with errors
		fmt.Println("Bot: Received an empty or incomplete response.")
	}
	// reader := bufio.NewReader(resp.Body)
	// for {
	// 	line, err := reader.ReadString('\n')
	// 	if err != nil && err != io.EOF {
	// 		fmt.Printf("Bot: Error reading response: %v\n", err)
	// 		return
	// 	}
	// 	if line == "" {
	// 		break
	// 	}
	//
	// 	var streamResponse map[string]interface{}
	// 	err = json.Unmarshal([]byte(line), &streamResponse)
	// 	if err != nil {
	// 		fmt.Printf("Bot: Error parsing response: %v\n", err)
	// 		return
	// 	}
	//
	// 	choices, ok := streamResponse["choices"].([]interface{})
	// 	if !ok || len(choices) == 0 {
	// 		continue
	// 	}
	//
	// 	choice := choices[0].(map[string]interface{})
	// 	message, ok := choice["delta"].(map[string]interface{})
	// 	if !ok {
	// 		continue
	// 	}
	//
	// 	content, ok := message["content"].(string)
	// 	if ok && content != "" {
	// 		fmt.Print(content)
	// 		*messages = append(*messages, Message{Role: "assistant", Content: content})
	// 	}
	// }
}

func getProvider() ModelProvider {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Warning: No .env file found, using environment variables directly")
	}
	provider := os.Getenv("MODEL_PROVIDER")
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable not set")
	}
	apiBase := os.Getenv("API_URL_BASE")
	if apiBase == "" {
		log.Fatal("API_URL_BASE environment variable not set")
	}
	apisString := os.Getenv("APIS")
	model := os.Getenv("MODEL")
	apis := map[string]string{}
	for _, api := range strings.Split(apisString, ",") {
		keyvalue := strings.Split(api, ":")
		if len(keyvalue) == 2 {
			apis[keyvalue[0]] = keyvalue[1]
		}
	}

	return ModelProvider{
		Provider: provider,
		UrlBase:  apiBase,
		APIKey:   apiKey,
		APIs:     apis,
		Model:    model,
	}
}

func listModels(args ...interface{}) {
	provider := args[0].(ModelProvider)
	url := provider.UrlBase + "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Bot: Error creating model list request: %v\n", err)
		return
	}
	req.Header.Add("Authorization", "Bearer "+provider.APIKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Bot: Error fetching models: %v\n", err)
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("Bot: Error reading models: %v\n", err)
		return
	}

	fmt.Println("Models: ", string(body))
}

func showProvider(args ...interface{}) {
	provider := args[0].(ModelProvider)

	fmt.Println("provider base url: " + provider.UrlBase)
}

func showModel(args ...interface{}) {
	provider := args[0].(ModelProvider)
	fmt.Println("Bot: Current model: " + provider.Model)
}

func exitCmd(args ...interface{}) {
	fmt.Println("Bot: Goodbye!")
	os.Exit(0)
}

type CommandFunc func(...interface{})

var commands = map[string]CommandFunc{
	"list":      listModels,
	"show":      showProvider,
	"showModel": showModel,
	"exit":      exitCmd,
}

func runCmd(command string, args ...interface{}) {
	if cmd, ok := commands[command]; ok {
		cmd(args...)
	} else {
		fmt.Println("Bot: Unknown command: ", command)
	}
}
