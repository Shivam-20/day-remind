package auth

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// Normalize is applied to a password both when hashing and when checking, so
// the two can never disagree about what the user actually typed.
func Normalize(password string) string {
	return strings.TrimSpace(password)
}

func HashPassword(password string) (string, error) {
	password = Normalize(password)
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(Normalize(password))) == nil
}
