package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Telegram
	TelegramToken string

	// OpenAI
	OpenAIKey        string
	OpenAIModel      string
	OpenAIBaseURL    string
	OpenAIMaxRetries int

	// Access control
	AllowedUsers []int64

	// Session
	MaxHistory int
	SessionTTL time.Duration

	// Concurrency
	MaxConcurrency int

	// Timeouts
	RequestTimeout time.Duration

	// Input validation
	MaxPromptLength int

	// Logging
	LogLevel  string // "debug", "info", "warn", "error"
	LogFormat string // "text", "json"
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if exists (ignore error if not found)
	_ = godotenv.Load()

	maxHistory, err := getEnvIntOrDefault("MAX_HISTORY", 20)
	if err != nil {
		return nil, err
	}
	maxConcurrency, err := getEnvIntOrDefault("MAX_CONCURRENCY", 20)
	if err != nil {
		return nil, err
	}
	openAIMaxRetries, err := getEnvIntOrDefault("OPENAI_MAX_RETRIES", 3)
	if err != nil {
		return nil, err
	}
	sessionTTL, err := getEnvDurationOrDefault("SESSION_TTL", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	requestTimeout, err := getEnvDurationOrDefault("REQUEST_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, err
	}
	maxPromptLength, err := getEnvIntOrDefault("MAX_PROMPT_LENGTH", 4000)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		TelegramToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
		OpenAIKey:        os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:      getEnvOrDefault("OPENAI_MODEL", "gpt-4o"),
		OpenAIBaseURL:    getEnvOrDefault("OPENAI_BASE_URL", ""),
		OpenAIMaxRetries: openAIMaxRetries,
		MaxHistory:       maxHistory,
		SessionTTL:       sessionTTL,
		MaxConcurrency:   maxConcurrency,
		RequestTimeout:   requestTimeout,
		MaxPromptLength:  maxPromptLength,
		LogLevel:         getEnvOrDefault("LOG_LEVEL", "info"),
		LogFormat:        getEnvOrDefault("LOG_FORMAT", "text"),
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

func getEnvIntOrDefault(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %q", key, value)
	}
	return intValue, nil
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %q (examples: 24h, 1h30m, 720h)", key, value)
	}
	return d, nil
}
