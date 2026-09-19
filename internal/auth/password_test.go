package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "secret-password") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected password mismatch")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	_, err := HashPassword("short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
}

// HashPassword normalizes its input, so CheckPassword must normalize too —
// otherwise a password with surrounding whitespace can never be used to log in.
func TestCheckPasswordMatchesHashNormalization(t *testing.T) {
	const typed = "  secret-password  "
	hash, err := HashPassword(typed)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, typed) {
		t.Fatal("expected the same string used at signup to work at login")
	}
	if !CheckPassword(hash, "secret-password") {
		t.Fatal("expected the trimmed form to match")
	}
}

func TestHashPasswordCountsLengthAfterTrimming(t *testing.T) {
	if _, err := HashPassword("      short      "); err == nil {
		t.Fatal("expected whitespace padding not to satisfy the length minimum")
	}
}
