package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/henryhwang/chatbot/internal/api"
	"github.com/henryhwang/chatbot/internal/commands"
	"github.com/henryhwang/chatbot/internal/config"
	"github.com/henryhwang/chatbot/internal/types"
)

// --- Main Application Logic ---

func main() {
	provider, err := config.Load() // Load configuration
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Println("Welcome to the Chatbot! Type '/exit' to quit.")
	fmt.Println("Using Model:", provider.Model)
	fmt.Println("--------------------------------------------")

	reader := bufio.NewReader(os.Stdin)
	// Initialize conversation history (can add a system message if desired)
	messages := []types.Message{
		// {Role: "system", Content: "You are a helpful assistant."},
	}

	for {
		fmt.Print("You: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.HasPrefix(input, "/") {
			// Pass provider info to command functions
			commands.RunCmd(strings.TrimPrefix(input, "/"), provider)
		} else if input != "" {
			// Handle regular chat query, modifying the message history
			api.QueryHandler(&messages, input, provider)
		}
		// No message for empty input to avoid clutter
	}
}
