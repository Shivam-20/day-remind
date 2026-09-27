package notify

import (
	"testing"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

func TestEffectiveDeliveryGatewayFromEnv(t *testing.T) {
	cfg := config.Config{
		TelegramGatewayURL:         "http://telegram-gateway:8091",
		TelegramGatewayAPIToken:    "tgw_env_token",
		TelegramGatewayInboundMode: "webhook",
	}
	stored := storage.DeliverySettings{}

	d := EffectiveDelivery(cfg, stored)

	if d.TelegramGatewayURL != "http://telegram-gateway:8091" {
		t.Errorf("gateway URL = %q", d.TelegramGatewayURL)
	}
	if d.TelegramGatewayAPIToken != "tgw_env_token" {
		t.Errorf("gateway token = %q", d.TelegramGatewayAPIToken)
	}
	if d.TelegramGatewayInboundMode != "webhook" {
		t.Errorf("gateway inbound mode = %q", d.TelegramGatewayInboundMode)
	}
	if !d.TelegramEnabled {
		t.Error("telegram should be enabled when gateway is configured")
	}
}

func TestEffectiveDeliveryGatewayStoredOverridesEnv(t *testing.T) {
	cfg := config.Config{
		TelegramGatewayURL:      "http://env-url",
		TelegramGatewayAPIToken: "tgw_env",
	}
	stored := storage.DeliverySettings{
		TelegramGatewayURL:         "http://stored-url",
		TelegramGatewayAPIToken:    "tgw_stored",
		TelegramGatewayInboundMode: "webhook",
		UpdatedAt:                  time.Now(),
		TelegramEnabled:            true,
	}

	d := EffectiveDelivery(cfg, stored)

	if d.TelegramGatewayURL != "http://stored-url" {
		t.Errorf("gateway URL = %q, want stored", d.TelegramGatewayURL)
	}
	if d.TelegramGatewayAPIToken != "tgw_stored" {
		t.Errorf("gateway token = %q, want stored", d.TelegramGatewayAPIToken)
	}
	if d.TelegramGatewayInboundMode != "webhook" {
		t.Errorf("inbound mode = %q, want webhook", d.TelegramGatewayInboundMode)
	}
}

func TestEffectiveDeliveryGatewayInboundModeDefaults(t *testing.T) {
	cfg := config.Config{}
	stored := storage.DeliverySettings{
		TelegramGatewayURL:      "http://gw",
		TelegramGatewayAPIToken: "tgw",
	}

	d := EffectiveDelivery(cfg, stored)

	if d.TelegramGatewayInboundMode != "poll" {
		t.Errorf("default inbound mode = %q, want poll", d.TelegramGatewayInboundMode)
	}
}

func TestChannelAvailabilityGateway(t *testing.T) {
	d := DeliveryConfig{
		TelegramEnabled:            true,
		TelegramGatewayURL:         "http://gw",
		TelegramGatewayAPIToken:    "tgw_abc",
		TelegramGatewayInboundMode: "poll",
	}
	avail := ChannelAvailability(d)

	if !avail[storage.ChannelTelegram] {
		t.Error("telegram should be available when gateway is configured")
	}
}

func TestChannelAvailabilityBotToken(t *testing.T) {
	d := DeliveryConfig{
		TelegramEnabled:    true,
		TelegramBotToken:   "123:abc",
		TelegramGatewayURL: "", // gateway not configured
	}
	avail := ChannelAvailability(d)

	if !avail[storage.ChannelTelegram] {
		t.Error("telegram should be available when bot token is set")
	}
}

func TestChannelAvailabilityNotConfigured(t *testing.T) {
	d := DeliveryConfig{}
	avail := ChannelAvailability(d)

	if avail[storage.ChannelTelegram] {
		t.Error("telegram should NOT be available when nothing is configured")
	}
}
