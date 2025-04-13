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
		Model: provider.Model,

		Messages: *messages,
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

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Bot: Error contacting LLM: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Bot: Error reading response: %v\n", err)
		return
	}

	var response OpenAIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		fmt.Printf("Bot: Error parsing response: %v\n", err)
		return
	}

	if len(response.Choices) > 0 {
		fmt.Println("Bot: ", response.Choices[0].Message.Content)
		*messages = append(*messages, response.Choices[0].Message)
	} else {
		fmt.Println("Bot: No response received, try again")
	}
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
