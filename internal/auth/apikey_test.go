package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "dr_") {
		t.Fatalf("expected dr_ prefix, got %q", key)
	}
	hash := HashAPIKey(key)
	if len(hash) != 64 {
		t.Fatalf("expected sha256 hex, got len %d", len(hash))
	}
	if HashAPIKey(key) != hash {
		t.Fatal("hash should be deterministic")
	}
}
