package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL string
	WebPort     int
	SessionKey  string
}

func Load() (Config, error) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	sessionKey := strings.TrimSpace(os.Getenv("SESSION_KEY"))
	if len(sessionKey) < 32 {
		return Config{}, fmt.Errorf("SESSION_KEY must be at least 32 characters")
	}
	port := 8090
	if raw := strings.TrimSpace(os.Getenv("WEB_PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid WEB_PORT: %q", raw)
		}
		port = n
	}
	return Config{
		DatabaseURL: dbURL,
		WebPort:     port,
		SessionKey:  sessionKey,
	}, nil
}
