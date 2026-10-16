package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shivam-20/day-remind/internal/bot"
	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/scheduler"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
	"github.com/shivam-20/day-remind/internal/web"
)

func main() {
	config.LoadEnvFiles()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := storage.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// Shut down cleanly on Ctrl-C so the bot's long poll and any in-flight
	// delivery are not killed mid-request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	box, err := secret.NewBox(cfg.SessionKey)
	if err != nil {
		log.Fatalf("credential encryption: %v", err)
	}

	// Environment variables seed a fresh install; after that the database is
	// authoritative and the admin panel is the place to change things.
	if seeded, err := db.SeedInstanceSettings(ctx, seedFromEnv(cfg, box)); err != nil {
		log.Fatalf("seed settings: %v", err)
	} else if seeded {
		log.Println("settings: seeded from environment (edit them in the admin panel from now on)")
	}

	resolver := delivery.NewResolver(db, box, cfg.TelegramAPIBase)
	if snap, err := resolver.Snapshot(ctx); err != nil {
		log.Printf("settings: could not read delivery configuration: %v", err)
	} else {
		inst := snap.Settings()
		log.Printf("email: %s", describe(inst.SMTP.Complete(), inst.SMTP.Host))
		log.Printf("telegram: %s", describe(inst.Telegram.Complete(), "bot"))
		if snap.SMTPUnreadable || snap.TelegramUnreadable {
			log.Println("settings: a stored credential could not be decrypted — SESSION_KEY has changed; re-enter it in the admin panel")
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.New(db, cfg.SchedulerInterval, resolver).Run(ctx)
	}()

	// The bot reads its token from the database on every cycle, so an admin can
	// change it without restarting the process.
	wg.Add(1)
	go func() {
		defer wg.Done()
		bot.New(db, resolver, cfg.TelegramAPIBase).Run(ctx)
	}()

	srv := web.New(cfg, db, resolver, box)
	httpSrv := srv.HTTPServer()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Println("shutting down…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	log.Printf("day-remind listening on http://localhost%s", httpSrv.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("http: %v", err)
		stop()
		wg.Wait()
		os.Exit(1)
	}

	// Stop the background workers, then wait for them to finish.
	stop()
	wg.Wait()
	log.Println("bye")
}

// seedFromEnv converts the legacy environment configuration into the initial
// settings document. It is only ever used when nothing is stored yet.
func seedFromEnv(cfg config.Config, box *secret.Box) storage.InstanceSettings {
	var s storage.InstanceSettings
	if cfg.SMTP.Enabled() {
		enc, err := box.Encrypt(cfg.SMTP.Password)
		if err != nil {
			log.Printf("seed: could not encrypt the SMTP password: %v", err)
		}
		s.SMTP = storage.SMTPSettings{
			Enabled: true, Host: cfg.SMTP.Host, Port: cfg.SMTP.Port,
			Username: cfg.SMTP.Username, PasswordEnc: enc,
			From: cfg.SMTP.From, TLS: cfg.SMTP.TLS,
		}
	}
	if cfg.TelegramEnabled() {
		enc, err := box.Encrypt(cfg.TelegramBotToken)
		if err != nil {
			log.Printf("seed: could not encrypt the bot token: %v", err)
		}
		s.Telegram = storage.TelegramSettings{Enabled: true, BotTokenEnc: enc}
	}
	return s
}

func describe(enabled bool, what string) string {
	if enabled {
		return "enabled (" + what + ")"
	}
	return "not configured — set it up in the admin panel"
}
