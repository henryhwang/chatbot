package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/henryhwang/chatbot/internal/types"
)

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
func RunCmd(command string, args ...interface{}) {
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
	provider, ok := args[0].(types.ModelProvider)
	if !ok {
		fmt.Println("Bot: Internal error: Invalid argument type for listModels.")
		return
	}

	// Check if a specific 'models' endpoint is defined in APIS map
	modelsPath, pathOk := provider.APIs["list"]
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
	provider, ok := args[0].(types.ModelProvider)
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
	provider, ok := args[0].(types.ModelProvider)
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
