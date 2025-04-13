package config

import (
	"log"
	"os"
	"strings"

	"github.com/henryhwang/chatbot/internal/types"

	"github.com/joho/godotenv"
)

// --- Configuration Loading ---

func Load() (types.ModelProvider, error) {
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
	return types.ModelProvider{
		Provider: providerName,
		UrlBase:  strings.TrimSuffix(apiBase, "/"), // Remove trailing slash for consistency
		APIKey:   apiKey,
		APIs:     apis,
		Model:    model,
	}, nil
}
