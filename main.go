package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"modul/internal/bot"
	"modul/internal/config"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("bot init: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := b.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("bot run: %v", err)
	}
}
