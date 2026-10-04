package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tggw "github.com/shivam-20/telegram-gateway/pkg/client"
)

// TelegramInlineButton is one button in an inline keyboard row.
type TelegramInlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// TelegramInlineKeyboard is the reply_markup payload for inline buttons.
type TelegramInlineKeyboard struct {
	InlineKeyboard [][]TelegramInlineButton `json:"inline_keyboard"`
}

// Callback actions used on reminder inline keyboards.
const (
	TelegramCallbackDone   = "done"
	TelegramCallbackSnooze = "snooze"
	TelegramCallbackPause  = "pause"
)

// TelegramCallbackData encodes a callback action/id/arg into Telegram's
// callback_data field.
func TelegramCallbackData(action, id, arg string) string {
	if strings.TrimSpace(arg) == "" {
		return action + ":" + id
	}
	return action + ":" + id + ":" + arg
}

// ParseTelegramCallbackData splits a callback_data string back into its parts.
func ParseTelegramCallbackData(data string) (action, id, arg string) {
	parts := strings.SplitN(strings.TrimSpace(data), ":", 3)
	if len(parts) > 0 {
		action = parts[0]
	}
	if len(parts) > 1 {
		id = parts[1]
	}
	if len(parts) > 2 {
		arg = parts[2]
	}
	return action, id, arg
}

// TelegramReminderKeyboard builds the standard alert/list keyboard for a
// reminder, optionally with an "open in app" URL button.
func TelegramReminderKeyboard(id, deepLink string) *TelegramInlineKeyboard {
	rows := [][]TelegramInlineButton{{
		{Text: "Done", CallbackData: TelegramCallbackData(TelegramCallbackDone, id, "")},
		{Text: "Snooze 1h", CallbackData: TelegramCallbackData(TelegramCallbackSnooze, id, "1h")},
		{Text: "Snooze tomorrow", CallbackData: TelegramCallbackData(TelegramCallbackSnooze, id, "tomorrow_9am")},
	}}
	if strings.TrimSpace(deepLink) != "" {
		rows = append(rows, []TelegramInlineButton{{Text: "Open in Day Remind", URL: deepLink}})
	}
	return &TelegramInlineKeyboard{InlineKeyboard: rows}
}

// TelegramUser is the sender/actor of a Telegram update.
type TelegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// TelegramChat identifies the conversation.
type TelegramChat struct {
	ID int64 `json:"id"`
}

// TelegramMessage is a subset of a Telegram message object.
type TelegramMessage struct {
	MessageID int64         `json:"message_id"`
	Chat      TelegramChat  `json:"chat"`
	From      *TelegramUser `json:"from,omitempty"`
	Text      string        `json:"text,omitempty"`
}

// TelegramCallback is an inline-button press.
type TelegramCallback struct {
	ID      string           `json:"id"`
	From    *TelegramUser    `json:"from,omitempty"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
}

// TelegramUpdate is a subset of the getUpdates/webhook update object.
type TelegramUpdate struct {
	UpdateID      int64             `json:"update_id"`
	Message       *TelegramMessage  `json:"message,omitempty"`
	CallbackQuery *TelegramCallback `json:"callback_query,omitempty"`
}

// EscapeTelegramHTML escapes the characters Telegram's HTML parse mode treats
// as markup.
func EscapeTelegramHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// IsGateway returns true when the delivery config routes Telegram through the
// telegram-gateway service instead of calling api.telegram.org directly.
func IsGateway(d DeliveryConfig) bool {
	return strings.TrimSpace(d.TelegramGatewayURL) != "" && strings.TrimSpace(d.TelegramGatewayAPIToken) != ""
}

// gatewayClient builds a telegram-gateway Go SDK client from the delivery config.
func gatewayClient(d DeliveryConfig) *tggw.Client {
	return tggw.New(d.TelegramGatewayURL, d.TelegramGatewayAPIToken)
}

// gatewayInboundEvent mirrors the subset of store.InboundEvent returned by the
// gateway's /v1/inbound/updates endpoint.
type gatewayInboundEvent struct {
	ID       int64           `json:"id"`
	UpdateID int64           `json:"update_id"`
	Payload  json.RawMessage `json:"payload"`
}

// callTelegram invokes a Bot API method. When the gateway is configured, it
// routes through the gateway's /v1/telegram/{method} passthrough endpoint.
// Otherwise it calls Telegram directly using the bot token.
func callTelegram(ctx context.Context, d DeliveryConfig, method string, payload any) (json.RawMessage, error) {
	if IsGateway(d) {
		return gatewayClient(d).Call(ctx, method, payload)
	}
	token := strings.TrimSpace(d.TelegramBotToken)
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is not configured")
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var out struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("telegram %s: invalid response", method)
	}
	if !out.OK {
		desc := strings.TrimSpace(out.Description)
		if desc == "" {
			desc = "request failed"
		}
		return nil, fmt.Errorf("telegram %s: %s", method, desc)
	}
	return out.Result, nil
}

func sendTelegramMessage(ctx context.Context, d DeliveryConfig, chatID, text, parseMode string, keyboard *TelegramInlineKeyboard) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("telegram chat id is required")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("telegram message text is required")
	}
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if keyboard != nil && len(keyboard.InlineKeyboard) > 0 {
		payload["reply_markup"] = keyboard
	}
	_, err := callTelegram(ctx, d, "sendMessage", payload)
	return err
}

// SendTelegram sends a plain-text message. Kept for existing alert/digest paths.
func SendTelegram(ctx context.Context, d DeliveryConfig, chatID, text string) error {
	if !d.TelegramEnabled || (!IsGateway(d) && d.TelegramBotToken == "") {
		return fmt.Errorf("telegram delivery is not configured")
	}
	return sendTelegramMessage(ctx, d, chatID, text, "", nil)
}

// SendTelegramMessage sends an HTML-formatted message with an optional inline
// keyboard. It only requires a bot token (used for bot replies as well as
// alerts gated elsewhere).
func SendTelegramMessage(ctx context.Context, d DeliveryConfig, chatID, text string, keyboard *TelegramInlineKeyboard) error {
	return sendTelegramMessage(ctx, d, chatID, text, "HTML", keyboard)
}

// AnswerTelegramCallback acknowledges a button press.
func AnswerTelegramCallback(ctx context.Context, d DeliveryConfig, callbackID, text string) error {
	payload := map[string]any{"callback_query_id": callbackID}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
	}
	_, err := callTelegram(ctx, d, "answerCallbackQuery", payload)
	return err
}

// EditTelegramMessage rewrites a previously sent message.
func EditTelegramMessage(ctx context.Context, d DeliveryConfig, chatID string, messageID int64, text string, keyboard *TelegramInlineKeyboard) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if keyboard == nil {
		keyboard = &TelegramInlineKeyboard{InlineKeyboard: [][]TelegramInlineButton{}}
	}
	payload["reply_markup"] = keyboard
	_, err := callTelegram(ctx, d, "editMessageText", payload)
	return err
}

// GetTelegramUpdates long-polls for new updates. timeoutSec is the server-side
// long-poll timeout (0 for a short poll). When the gateway is configured, this
// delegates to GetGatewayInboundUpdates and auto-acks fetched events.
func GetTelegramUpdates(ctx context.Context, d DeliveryConfig, offset int64, timeoutSec int) ([]TelegramUpdate, error) {
	if IsGateway(d) {
		updates, ids, err := GetGatewayInboundUpdates(ctx, d, offset, 100)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			_ = AckGatewayInbound(ctx, d, ids)
		}
		return updates, nil
	}
	payload := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}
	raw, err := callTelegram(ctx, d, "getUpdates", payload)
	if err != nil {
		return nil, err
	}
	var updates []TelegramUpdate
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &updates); err != nil {
			return nil, fmt.Errorf("telegram getUpdates: invalid result")
		}
	}
	return updates, nil
}

// GetGatewayInboundUpdates pulls unacked updates from the telegram-gateway
// /v1/inbound/updates endpoint. afterID is the last-acked gateway event ID.
// It returns the Telegram updates (extracted from each event's payload) and
// the gateway event IDs (to be acked by the caller via AckGatewayInbound).
func GetGatewayInboundUpdates(ctx context.Context, d DeliveryConfig, afterID int64, limit int) ([]TelegramUpdate, []int64, error) {
	c := gatewayClient(d)
	raw, err := c.InboundUpdates(ctx, afterID, limit)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var events []gatewayInboundEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, nil, fmt.Errorf("telegram-gateway: invalid inbound result")
	}
	ids := make([]int64, 0, len(events))
	updates := make([]TelegramUpdate, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.ID)
		var upd TelegramUpdate
		if len(ev.Payload) > 0 {
			if err := json.Unmarshal(ev.Payload, &upd); err != nil {
				return nil, nil, fmt.Errorf("telegram-gateway: invalid update payload")
			}
		}
		updates = append(updates, upd)
	}
	return updates, ids, nil
}

// AckGatewayInbound acknowledges (consumes) inbound events on the gateway so
// they are not returned again on the next poll.
func AckGatewayInbound(ctx context.Context, d DeliveryConfig, ids []int64) error {
	c := gatewayClient(d)
	_, err := c.AckInbound(ctx, ids)
	return err
}

// DeleteTelegramWebhook removes any registered webhook so polling can run.
// When the gateway is configured, transport management is done on the gateway
// side and this is a no-op.
func DeleteTelegramWebhook(ctx context.Context, d DeliveryConfig) error {
	if IsGateway(d) {
		return nil
	}
	_, err := callTelegram(ctx, d, "deleteWebhook", map[string]any{"drop_pending_updates": false})
	return err
}

// SetTelegramWebhook registers a webhook URL with an optional secret token.
// When the gateway is configured, transport management is done on the gateway
// side and this is a no-op.
func SetTelegramWebhook(ctx context.Context, d DeliveryConfig, url, secret string) error {
	if IsGateway(d) {
		return nil
	}
	payload := map[string]any{
		"url":             url,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if strings.TrimSpace(secret) != "" {
		payload["secret_token"] = secret
	}
	_, err := callTelegram(ctx, d, "setWebhook", payload)
	return err
}

// GetTelegramWebhookInfo returns the current webhook status. When the gateway
// is configured, it queries the gateway's /v1/telegram/getWebhookInfo passthrough.
func GetTelegramWebhookInfo(ctx context.Context, d DeliveryConfig) (map[string]any, error) {
	if IsGateway(d) {
		c := gatewayClient(d)
		raw, err := c.Call(ctx, "getWebhookInfo", nil)
		if err != nil {
			return nil, err
		}
		var info map[string]any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &info); err != nil {
				return nil, fmt.Errorf("telegram-gateway: invalid getWebhookInfo result")
			}
		}
		return info, nil
	}
	raw, err := callTelegram(ctx, d, "getWebhookInfo", nil)
	if err != nil {
		return nil, err
	}
	var info map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &info); err != nil {
			return nil, fmt.Errorf("telegram getWebhookInfo: invalid result")
		}
	}
	return info, nil
}

// GatewayMe calls the gateway's GET /v1/me endpoint (passthrough to Telegram's
// getMe) and returns the raw JSON result. Useful for fetching the bot username
// when the bot token is held by the gateway.
func GatewayMe(ctx context.Context, d DeliveryConfig) (json.RawMessage, error) {
	return gatewayClient(d).Me(ctx)
}

// CallGatewayStatus queries the gateway's GET /v1/status endpoint and returns
// the raw JSON response describing the app's transport state (enabled,
// update_mode, inbound_mode, bot_configured, inbound_url).
func CallGatewayStatus(ctx context.Context, d DeliveryConfig) (json.RawMessage, error) {
	endpoint := strings.TrimRight(d.TelegramGatewayURL, "/") + "/v1/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.TelegramGatewayAPIToken)
	// Status backs an admin page load; don't let a slow gateway hang it.
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram-gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram-gateway: unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
