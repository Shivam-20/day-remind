package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEmailVerificationTokenLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	email := "verify-" + time.Now().Format("150405.000000000") + "@example.com"
	user, err := db.CreateUser(ctx, email, "hash-not-a-real-credential", "Verify", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if user.EmailVerifiedAt != nil {
		t.Fatal("new users must start unverified")
	}

	token, err := db.CreateEmailVerificationToken(ctx, user.ID)
	if err != nil || token == "" {
		t.Fatalf("create token: %v %q", err, token)
	}

	// A second token invalidates the first (one outstanding link max).
	token2, err := db.CreateEmailVerificationToken(ctx, user.ID)
	if err != nil || token2 == "" || token2 == token {
		t.Fatalf("second token: %v", err)
	}
	if err := db.ConsumeEmailVerificationToken(ctx, token); err != ErrNotFound {
		t.Fatalf("old token should be gone, got %v", err)
	}

	if err := db.ConsumeEmailVerificationToken(ctx, token2); err != nil {
		t.Fatalf("consume: %v", err)
	}
	got, err := db.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EmailVerifiedAt == nil {
		t.Fatal("user should be verified after consuming token")
	}
	if err := db.ConsumeEmailVerificationToken(ctx, token2); err != ErrNotFound {
		t.Fatalf("token should be single-use, got %v", err)
	}

	if err := db.SetUserEmailVerified(ctx, user.ID, false); err != nil {
		t.Fatalf("unset verified: %v", err)
	}
	got, err = db.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EmailVerifiedAt != nil {
		t.Fatal("verified flag should be cleared")
	}

	if err := db.ConsumeEmailVerificationToken(ctx, "not-a-hex-token-at-all"); err != ErrNotFound {
		t.Fatalf("malformed token must be rejected, got %v", err)
	}
	if err := db.ConsumeEmailVerificationToken(ctx, uuid.New().String()); err != ErrNotFound {
		t.Fatalf("unknown token must be ErrNotFound, got %v", err)
	}
}
