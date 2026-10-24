package config

import (
	"os"
	"path/filepath"
	"testing"
)

func chdirTemp(t *testing.T) string {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

func TestLoadEnvFiles_systemEnvWinsOverFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile(".env", []byte("DATABASE_URL=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "from-system")
	t.Setenv("DAY_REMIND_SKIP_DOTENV", "")

	LoadEnvFiles()

	if got := os.Getenv("DATABASE_URL"); got != "from-system" {
		t.Fatalf("DATABASE_URL = %q, want from-system (host env must win)", got)
	}
}

func TestLoadEnvFiles_loadsLocalDotenv(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile(".env", []byte("DATABASE_URL=from-file\nSESSION_KEY=from-file-session-key-32chars!!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("SESSION_KEY")
	t.Setenv("DAY_REMIND_SKIP_DOTENV", "")

	LoadEnvFiles()

	if got := os.Getenv("DATABASE_URL"); got != "from-file" {
		t.Fatalf("DATABASE_URL = %q, want from-file", got)
	}
	if got := os.Getenv("SESSION_KEY"); got != "from-file-session-key-32chars!!" {
		t.Fatalf("SESSION_KEY = %q, want value from .env", got)
	}
}

func TestLoadEnvFiles_dotenvPath(t *testing.T) {
	dir := t.TempDir()
	panel := filepath.Join(dir, "panel.env")
	if err := os.WriteFile(panel, []byte("WEB_PORT=9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirTemp(t)
	t.Setenv("DOTENV_PATH", panel)
	os.Unsetenv("WEB_PORT")

	LoadEnvFiles()

	if got := os.Getenv("WEB_PORT"); got != "9090" {
		t.Fatalf("WEB_PORT = %q, want 9090 from DOTENV_PATH", got)
	}
}

func TestLoadEnvFiles_skipDotenv(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile(".env", []byte("DATABASE_URL=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("DATABASE_URL")
	t.Setenv("DAY_REMIND_SKIP_DOTENV", "1")

	LoadEnvFiles()

	if got := os.Getenv("DATABASE_URL"); got != "" {
		t.Fatalf("DATABASE_URL = %q, want empty when DAY_REMIND_SKIP_DOTENV=1", got)
	}
}
