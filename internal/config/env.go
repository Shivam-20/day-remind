package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// LoadEnvFiles merges optional dotenv files into the process environment.
// Variables already set by the host (Pterodactyl panel, Docker, systemd) are
// never overwritten — those values win.
//
// Lookup order (first existing file wins per key; later files only fill gaps):
//   1. DOTENV_PATH when set
//   2. /home/container/.env (Pterodactyl default)
//   3. $HOME/.env
//   4. ./.env (local dev)
//
// Set DAY_REMIND_SKIP_DOTENV=1 to disable file loading entirely.
func LoadEnvFiles() {
	if truthy(os.Getenv("DAY_REMIND_SKIP_DOTENV")) {
		return
	}

	var paths []string
	if p := strings.TrimSpace(os.Getenv("DOTENV_PATH")); p != "" {
		paths = append(paths, p)
	}
	paths = append(paths, "/home/container/.env")
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		paths = append(paths, filepath.Join(home, ".env"))
	}
	paths = append(paths, ".env")

	existing := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return
	}
	_ = godotenv.Load(existing...)
}
