package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/auth"
	"github.com/user/tgbot/internal/bot"
	"github.com/user/tgbot/internal/config"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/session"
	"github.com/user/tgbot/internal/version"
)

func main() {
	// Log version info first (before anything else)
	log := logger.Default()
	ver := version.Get()
	log.Info("TgBot starting", ver.LogFields()...)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Reinitialize logger with config settings
	log = logger.New(logger.Config{
		Level:  cfg.LogLevel,
		Format: cfg.LogFormat,
	})
	logger.SetGlobal(log)

	log.Info("configuration loaded", "model", cfg.OpenAIModel, "allowed_users", len(cfg.AllowedUsers))

	// Initialize components
	whitelist := auth.NewWhitelist(cfg.AllowedUsers, log.With("component", "auth"))
	sessions := session.NewManager(cfg.MaxHistory)
	aiProvider := ai.NewOpenAIProvider(cfg.OpenAIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL)

	// Create bot
	tgBot, err := bot.New(cfg.TelegramToken, aiProvider, sessions, whitelist, log.With("component", "bot"), cfg.MaxConcurrency)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Run bot
	if err := tgBot.Run(ctx); err != nil && err != context.Canceled {
		log.Error("bot error", "error", err)
		os.Exit(1)
	}

	log.Info("bot stopped")
}
