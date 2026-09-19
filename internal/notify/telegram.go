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

// callTelegram invokes a Bot API method and returns its raw "result" field.
func callTelegram(ctx context.Context, token, method string, payload any) (json.RawMessage, error) {
	token = strings.TrimSpace(token)
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
	_, err := callTelegram(ctx, d.TelegramBotToken, "sendMessage", payload)
	return err
}

// SendTelegram sends a plain-text message. Kept for existing alert/digest paths.
func SendTelegram(ctx context.Context, d DeliveryConfig, chatID, text string) error {
	if !d.TelegramEnabled || d.TelegramBotToken == "" {
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
	_, err := callTelegram(ctx, d.TelegramBotToken, "answerCallbackQuery", payload)
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
	_, err := callTelegram(ctx, d.TelegramBotToken, "editMessageText", payload)
	return err
}

// GetTelegramUpdates long-polls for new updates. timeoutSec is the server-side
// long-poll timeout (0 for a short poll).
func GetTelegramUpdates(ctx context.Context, d DeliveryConfig, offset int64, timeoutSec int) ([]TelegramUpdate, error) {
	payload := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}
	raw, err := callTelegram(ctx, d.TelegramBotToken, "getUpdates", payload)
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

// DeleteTelegramWebhook removes any registered webhook so polling can run.
func DeleteTelegramWebhook(ctx context.Context, d DeliveryConfig) error {
	_, err := callTelegram(ctx, d.TelegramBotToken, "deleteWebhook", map[string]any{"drop_pending_updates": false})
	return err
}

// SetTelegramWebhook registers a webhook URL with an optional secret token.
func SetTelegramWebhook(ctx context.Context, d DeliveryConfig, url, secret string) error {
	payload := map[string]any{
		"url":             url,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if strings.TrimSpace(secret) != "" {
		payload["secret_token"] = secret
	}
	_, err := callTelegram(ctx, d.TelegramBotToken, "setWebhook", payload)
	return err
}

// GetTelegramWebhookInfo returns the current webhook status.
func GetTelegramWebhookInfo(ctx context.Context, d DeliveryConfig) (map[string]any, error) {
	raw, err := callTelegram(ctx, d.TelegramBotToken, "getWebhookInfo", nil)
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
