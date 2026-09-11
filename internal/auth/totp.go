package auth

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret creates a new TOTP secret for 2FA enrollment.
func GenerateTOTPSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Day Remind",
		AccountName: "user",
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return key.Secret(), nil
}

// ValidateTOTP checks a TOTP code against the stored secret.
func ValidateTOTP(secret, code string) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}

// ProvisioningURI returns an otpauth:// URI for authenticator app enrollment.
func ProvisioningURI(secret, accountName string) string {
	secret = strings.TrimSpace(secret)
	accountName = strings.TrimSpace(accountName)
	if accountName == "" {
		accountName = "user"
	}
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&period=30&digits=6",
		url.PathEscape("Day Remind"),
		url.PathEscape(accountName),
		secret,
		url.QueryEscape("Day Remind"),
	)
}

// ValidateTOTPWithWindow validates a code allowing a small clock skew window.
func ValidateTOTPWithWindow(secret, code string, skew uint) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	now := time.Now()
	for i := -int(skew); i <= int(skew); i++ {
		t := now.Add(time.Duration(i) * 30 * time.Second)
		expected, err := totp.GenerateCode(secret, t)
		if err == nil && expected == code {
			return true
		}
	}
	return false
}
