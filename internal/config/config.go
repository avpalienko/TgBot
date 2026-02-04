package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	TelegramToken string
	OpenAIKey     string
	OpenAIModel   string
	OpenAIBaseURL string
	AllowedUsers  []int64
	MaxHistory    int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		OpenAIKey:     os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:   getEnvOrDefault("OPENAI_MODEL", "gpt-4o"),
		OpenAIBaseURL: getEnvOrDefault("OPENAI_BASE_URL", ""),
		MaxHistory:    getEnvIntOrDefault("MAX_HISTORY", 20),
	}

	// Validate required fields
	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.OpenAIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}

	// Parse allowed users
	allowedStr := os.Getenv("ALLOWED_USERS")
	if allowedStr != "" {
		for _, idStr := range strings.Split(allowedStr, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid user ID in ALLOWED_USERS: %s", idStr)
			}
			cfg.AllowedUsers = append(cfg.AllowedUsers, id)
		}
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
