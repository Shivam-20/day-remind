package web

import (
	"strings"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Non-secret fixtures: provider validation only needs non-empty strings.
// Real Resend keys are never embedded in source (see AGENTS.md / env config).
const (
	resendFixture    = "fixture-resend-value-not-usable"
	envResendFixture = "fixture-env-value-not-usable"
)

func TestValidateDeliveryProviderSelection(t *testing.T) {
	cases := []struct {
		name    string
		cur     storage.DeliverySettings
		cfg     config.Config
		body    adminDeliveryUpdateRequest
		wantErr string // empty means no error
	}{
		{
			name: "switch to resend with only smtp configured is rejected",
			cur: storage.DeliverySettings{
				EmailProvider: "smtp",
				SMTPHost:      "smtp.fixture.test",
			},
			body: adminDeliveryUpdateRequest{EmailProvider: strPtr("resend")},
			wantErr: "no Resend API key",
		},
		{
			name: "switch to resend with key in same save is allowed",
			cur: storage.DeliverySettings{
				EmailProvider: "smtp",
				SMTPHost:      "smtp.fixture.test",
			},
			body: adminDeliveryUpdateRequest{EmailProvider: strPtr("resend"), ResendAPIKey: strPtr(resendFixture)},
		},
		{
			name: "switch to smtp with no host while resend configured is rejected",
			cur: storage.DeliverySettings{
				EmailProvider: "resend",
				ResendAPIKey:  resendFixture,
			},
			body: adminDeliveryUpdateRequest{EmailProvider: strPtr("smtp")},
			wantErr: "no SMTP host",
		},
		{
			name: "staying on smtp with smtp host is allowed",
			cur: storage.DeliverySettings{
				EmailProvider: "smtp",
				SMTPHost:      "smtp.fixture.test",
			},
			body: adminDeliveryUpdateRequest{EmailFrom: strPtr("alerts@fixture.test")},
		},
		{
			name: "resend key from env fallback satisfies resend selection",
			cur:  storage.DeliverySettings{EmailProvider: "smtp", SMTPHost: "smtp.fixture.test"},
			cfg:  config.Config{ResendAPIKey: envResendFixture},
			body: adminDeliveryUpdateRequest{EmailProvider: strPtr("resend")},
		},
		{
			name: "no provider credentials at all is allowed (email simply off)",
			cur:  storage.DeliverySettings{},
			body: adminDeliveryUpdateRequest{EmailProvider: strPtr("resend")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDeliveryProviderSelection(tc.cur, tc.cfg, tc.body)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
