package config

import "testing"

func TestResolveListenPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "mongodb://localhost:27017/day_remind")
	t.Setenv("SESSION_KEY", "0123456789abcdef0123456789abcdef")

	t.Run("default", func(t *testing.T) {
		t.Setenv("WEB_PORT", "")
		t.Setenv("SERVER_PORT", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WebPort != 8090 {
			t.Fatalf("port = %d, want 8090", cfg.WebPort)
		}
	})

	t.Run("web port", func(t *testing.T) {
		t.Setenv("WEB_PORT", "8091")
		t.Setenv("SERVER_PORT", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WebPort != 8091 {
			t.Fatalf("port = %d, want 8091", cfg.WebPort)
		}
	})

	t.Run("server port overrides web port", func(t *testing.T) {
		t.Setenv("WEB_PORT", "8090")
		t.Setenv("SERVER_PORT", "25565")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WebPort != 25565 {
			t.Fatalf("port = %d, want 25565", cfg.WebPort)
		}
	})

	t.Run("invalid server port", func(t *testing.T) {
		t.Setenv("SERVER_PORT", "nope")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("port out of range", func(t *testing.T) {
		t.Setenv("SERVER_PORT", "70000")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})
}
