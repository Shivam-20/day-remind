// Package secret encrypts the credentials that have to be stored in the
// database and read back — SMTP passwords and Telegram bot tokens.
//
// These are the only reversible secrets the application keeps. Account
// passwords are bcrypt hashes and must never be handled here.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// prefix tags the scheme so a future algorithm change can be told apart from
// today's values without guessing.
const prefix = "v1:"

// ErrUndecryptable means the stored value cannot be read with the current key.
// In practice that means SESSION_KEY changed. Callers must treat the affected
// channel as unconfigured and ask the user to re-enter the credential, rather
// than failing the whole request.
var ErrUndecryptable = errors.New("stored credential cannot be decrypted with the current SESSION_KEY")

// Box encrypts and decrypts short strings with AES-256-GCM.
type Box struct {
	aead cipher.AEAD
}

// NewBox derives an encryption key from the session key. Config already
// enforces a 32 character minimum, but this repeats the check because the
// consequence of a weak key here is different: it protects data at rest.
func NewBox(sessionKey string) (*Box, error) {
	if len(strings.TrimSpace(sessionKey)) < 32 {
		return nil, fmt.Errorf("session key must be at least 32 characters to derive an encryption key")
	}
	// HKDF with a fixed info label keeps this key distinct from any other use
	// of SESSION_KEY. There is no salt: it would have to be stored alongside
	// the data, and the key material is already high entropy.
	key := make([]byte, 32)
	r := hkdf.New(sha256.New, []byte(sessionKey), nil, []byte("day-remind/credential-encryption/v1"))
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Encrypt returns a self-describing string safe to store in the database.
// Encrypting an empty string yields an empty string, so "not set" round-trips
// as "not set" rather than as an opaque blob.
func (b *Box) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Any value that cannot be authenticated — a changed
// key, a truncated field, a tampered document — comes back as ErrUndecryptable
// so callers can handle all of those the same way.
func (b *Box) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, prefix) {
		return "", ErrUndecryptable
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", ErrUndecryptable
	}
	if len(raw) < b.aead.NonceSize() {
		return "", ErrUndecryptable
	}
	nonce, ct := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrUndecryptable
	}
	return string(plain), nil
}

// IsEncrypted reports whether a stored value carries this package's tag. It
// exists so callers can tell "never set" from "set but unreadable".
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, prefix)
}

// Hint returns a non-secret fragment safe to show in a UI, so someone can tell
// which credential is stored without revealing it.
func Hint(plain string) string {
	if plain == "" {
		return ""
	}
	if len(plain) <= 4 {
		return "••••"
	}
	return "••••" + plain[len(plain)-4:]
}
