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
	"modul/internal/db"
	"modul/internal/ruvds"
	"modul/internal/service"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	gormDB, err := db.Init()
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	repo := service.NewRepository(gormDB)
	client := ruvds.New(cfg.RuvdsToken)
	svc := service.New(cfg, repo, client)

	b, err := bot.New(cfg, svc)
	if err != nil {
		log.Fatalf("bot init: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := b.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("bot run: %v", err)
	}
}
