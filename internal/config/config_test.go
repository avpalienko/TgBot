package config

import (
    "testing"
)

// t.Setenv is incompatible with t.Parallel -- config tests run sequentially.

func setCleanEnv(t *testing.T) {
    t.Helper()
    for _, key := range []string{
        "TELEGRAM_BOT_TOKEN", "OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_BASE_URL",
        "ALLOWED_USERS", "MAX_HISTORY", "MAX_CONCURRENCY", "LOG_LEVEL", "LOG_FORMAT",
    } {
        t.Setenv(key, "")
    }
}

func TestLoadMinimal(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "tok123")
    t.Setenv("OPENAI_API_KEY", "key456")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.TelegramToken != "tok123" {
        t.Fatalf("expected token %q, got %q", "tok123", cfg.TelegramToken)
    }
    if cfg.OpenAIKey != "key456" {
        t.Fatalf("expected key %q, got %q", "key456", cfg.OpenAIKey)
    }
    if cfg.OpenAIModel != "gpt-4o" {
        t.Fatalf("expected default model %q, got %q", "gpt-4o", cfg.OpenAIModel)
    }
    if cfg.MaxHistory != 20 {
        t.Fatalf("expected default maxHistory 20, got %d", cfg.MaxHistory)
    }
    if cfg.MaxConcurrency != 20 {
        t.Fatalf("expected default maxConcurrency 20, got %d", cfg.MaxConcurrency)
    }
    if len(cfg.AllowedUsers) != 0 {
        t.Fatalf("expected no allowed users, got %v", cfg.AllowedUsers)
    }
}

func TestLoadFull(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
    t.Setenv("OPENAI_API_KEY", "api-key")
    t.Setenv("OPENAI_MODEL", "gpt-4-turbo")
    t.Setenv("OPENAI_BASE_URL", "https://custom.api")
    t.Setenv("ALLOWED_USERS", "100,200,300")
    t.Setenv("MAX_HISTORY", "50")
    t.Setenv("MAX_CONCURRENCY", "10")
    t.Setenv("LOG_LEVEL", "debug")
    t.Setenv("LOG_FORMAT", "json")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.OpenAIModel != "gpt-4-turbo" {
        t.Fatalf("expected model %q, got %q", "gpt-4-turbo", cfg.OpenAIModel)
    }
    if cfg.OpenAIBaseURL != "https://custom.api" {
        t.Fatalf("expected base URL %q, got %q", "https://custom.api", cfg.OpenAIBaseURL)
    }
    if cfg.MaxHistory != 50 {
        t.Fatalf("expected maxHistory 50, got %d", cfg.MaxHistory)
    }
    if cfg.MaxConcurrency != 10 {
        t.Fatalf("expected maxConcurrency 10, got %d", cfg.MaxConcurrency)
    }
    if len(cfg.AllowedUsers) != 3 || cfg.AllowedUsers[0] != 100 || cfg.AllowedUsers[1] != 200 || cfg.AllowedUsers[2] != 300 {
        t.Fatalf("expected [100 200 300], got %v", cfg.AllowedUsers)
    }
    if cfg.LogLevel != "debug" {
        t.Fatalf("expected log level %q, got %q", "debug", cfg.LogLevel)
    }
    if cfg.LogFormat != "json" {
        t.Fatalf("expected log format %q, got %q", "json", cfg.LogFormat)
    }
}

func TestLoadMissingToken(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("OPENAI_API_KEY", "key")

    _, err := Load()
    if err == nil {
        t.Fatalf("expected error for missing TELEGRAM_BOT_TOKEN")
    }
}

func TestLoadMissingAPIKey(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "tok")

    _, err := Load()
    if err == nil {
        t.Fatalf("expected error for missing OPENAI_API_KEY")
    }
}

func TestLoadInvalidAllowedUsers(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
    t.Setenv("OPENAI_API_KEY", "key")
    t.Setenv("ALLOWED_USERS", "123,abc,456")

    _, err := Load()
    if err == nil {
        t.Fatalf("expected error for non-numeric ALLOWED_USERS value")
    }
}

func TestLoadInvalidMaxHistory(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
    t.Setenv("OPENAI_API_KEY", "key")
    t.Setenv("MAX_HISTORY", "not-a-number")

    _, err := Load()
    if err == nil {
        t.Fatalf("expected error for non-numeric MAX_HISTORY")
    }
}

func TestLoadAllowedUsersParsing(t *testing.T) {
    setCleanEnv(t)
    t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
    t.Setenv("OPENAI_API_KEY", "key")
    t.Setenv("ALLOWED_USERS", "123, 456, , 789")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    want := []int64{123, 456, 789}
    if len(cfg.AllowedUsers) != len(want) {
        t.Fatalf("expected %d users, got %d: %v", len(want), len(cfg.AllowedUsers), cfg.AllowedUsers)
    }
    for i, id := range want {
        if cfg.AllowedUsers[i] != id {
            t.Fatalf("AllowedUsers[%d] = %d, want %d", i, cfg.AllowedUsers[i], id)
        }
    }
}
