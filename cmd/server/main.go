package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/jobs"
	"github.com/shivam-20/day-remind/internal/scheduler"
	"github.com/shivam-20/day-remind/internal/storage"
	"github.com/shivam-20/day-remind/internal/web"
	"github.com/shivam-20/day-remind/internal/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := storage.Open(cfg.DatabaseURL, cfg.AdminEmails)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hooks := webhook.New(db, cfg.WebhookBaseURL)
	go hooks.Run(ctx)

	sched := scheduler.New(db, cfg)
	sched.Webhooks = hooks
	go sched.Run(ctx)

	go runMorningDigests(ctx, db, cfg)

	srv := web.New(cfg, db)
	srv.SetWebhooks(hooks)
	go srv.RunTelegram(ctx)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
}

func runMorningDigests(ctx context.Context, db *storage.DB, cfg config.Config) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := jobs.RunMorningDigests(ctx, db, cfg); err != nil {
			log.Printf("digest job: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
