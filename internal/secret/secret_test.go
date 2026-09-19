package secret

import (
	"errors"
	"strings"
	"testing"
)

const testKey = "a-test-session-key-that-is-long-enough-32"

func newBox(t *testing.T, key string) *Box {
	t.Helper()
	b, err := NewBox(key)
	if err != nil {
		t.Fatalf("NewBox: %v", err)
	}
	return b
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	b := newBox(t, testKey)
	for _, plain := range []string{
		"hunter2",
		"123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		"a",
		strings.Repeat("long", 500),
		"unicode: café ☕ 密码",
	} {
		enc, err := b.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%.20q): %v", plain, err)
		}
		if strings.Contains(enc, plain) {
			t.Errorf("ciphertext contains the plaintext for %.20q", plain)
		}
		if !IsEncrypted(enc) {
			t.Errorf("Encrypt output is not tagged: %.30q", enc)
		}
		got, err := b.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Errorf("round trip = %.20q, want %.20q", got, plain)
		}
	}
}

// "Not set" must survive a round trip as "not set".
func TestEmptyRoundTripsAsEmpty(t *testing.T) {
	b := newBox(t, testKey)
	enc, err := b.Encrypt("")
	if err != nil || enc != "" {
		t.Fatalf("Encrypt(\"\") = %q, %v; want empty", enc, err)
	}
	got, err := b.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("Decrypt(\"\") = %q, %v; want empty", got, err)
	}
}

// The same plaintext must not produce the same ciphertext twice, or stored
// values would leak which users share a password.
func TestEncryptionIsNonDeterministic(t *testing.T) {
	b := newBox(t, testKey)
	a, _ := b.Encrypt("same-secret")
	c, _ := b.Encrypt("same-secret")
	if a == c {
		t.Error("two encryptions of the same value produced identical ciphertext")
	}
}

// This is the SESSION_KEY rotation case the plan calls out.
func TestWrongKeyGivesErrUndecryptable(t *testing.T) {
	enc, err := newBox(t, testKey).Encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	other := newBox(t, "a-completely-different-session-key-here-32")
	got, err := other.Decrypt(enc)
	if !errors.Is(err, ErrUndecryptable) {
		t.Errorf("Decrypt with a rotated key = %q, %v; want ErrUndecryptable", got, err)
	}
}

func TestTamperedOrMalformedValuesAreRejected(t *testing.T) {
	b := newBox(t, testKey)
	enc, err := b.Encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"flipped last byte": enc[:len(enc)-2] + flip(enc[len(enc)-2:len(enc)-1]) + enc[len(enc)-1:],
		"truncated":         enc[:len(enc)/2],
		"untagged":          strings.TrimPrefix(enc, prefix),
		"not base64":        prefix + "!!!!not base64!!!!",
		"tag only":          prefix,
		"plaintext":         "hunter2",
	}
	for name, bad := range cases {
		if got, err := b.Decrypt(bad); !errors.Is(err, ErrUndecryptable) {
			t.Errorf("%s: Decrypt = %q, %v; want ErrUndecryptable", name, got, err)
		}
	}
}

func flip(s string) string {
	if s == "A" {
		return "B"
	}
	return "A"
}

// A weak key here would weaken data at rest, so it is rejected outright.
func TestNewBoxRejectsShortKeys(t *testing.T) {
	for _, key := range []string{"", "short", strings.Repeat("x", 31), "   " + strings.Repeat("x", 20)} {
		if _, err := NewBox(key); err == nil {
			t.Errorf("NewBox(%q) should fail", key)
		}
	}
	if _, err := NewBox(strings.Repeat("x", 32)); err != nil {
		t.Errorf("a 32 character key should be accepted: %v", err)
	}
}

func TestHintRevealsOnlyTheTail(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"abc":             "••••",
		"abcd":            "••••",
		"hunter2":         "••••ter2",
		"1234567890token": "••••oken",
	}
	for in, want := range cases {
		if got := Hint(in); got != want {
			t.Errorf("Hint(%q) = %q, want %q", in, got, want)
		}
	}
	// Whatever the input, the hint must never contain the whole secret.
	const token = "123456:ABC-DEF1234ghIkl"
	if strings.Contains(Hint(token), token) {
		t.Error("hint leaked the full secret")
	}
}
