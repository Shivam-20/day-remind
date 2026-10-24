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

// TelegramClient talks to the Bot API. It is used both to deliver notifications
// and, by the bot loop, to poll for and answer incoming commands.
type TelegramClient struct {
	Token   string
	APIBase string
	HTTP    *http.Client
}

func NewTelegramClient(token, apiBase string) *TelegramClient {
	if apiBase == "" {
		apiBase = "https://api.telegram.org"
	}
	return &TelegramClient{
		Token:   token,
		APIBase: strings.TrimRight(apiBase, "/"),
		// Long polling holds the request open, so the ceiling is generous.
		HTTP: &http.Client{Timeout: 70 * time.Second},
	}
}

func (t *TelegramClient) Name() string { return ChannelTelegram }

// Send delivers a notification, with inline Done and Snooze buttons so the
// reminder can be actioned without leaving the chat.
func (t *TelegramClient) Send(ctx context.Context, dest string, m Message, r Rendered) error {
	if dest == "" {
		return fmt.Errorf("no telegram chat linked")
	}
	payload := map[string]any{
		"chat_id": dest,
		"text":    r.Telegram,
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{{
				{"text": "✓ Done", "callback_data": "done:" + m.ReminderID},
				{"text": "+1h", "callback_data": "snooze1h:" + m.ReminderID},
				{"text": "Tomorrow 9am", "callback_data": "snooze9am:" + m.ReminderID},
			}},
		},
	}
	// Plain mode exists for template authors who would rather not deal with
	// MarkdownV2 escaping; omitting parse_mode makes Telegram treat the text
	// literally and stops it rejecting the message over stray punctuation.
	if !r.TelegramPlain {
		payload["parse_mode"] = "MarkdownV2"
	}
	_, err := t.call(ctx, "sendMessage", payload)
	return err
}

// SendText posts a plain message with no buttons, used for command replies.
func (t *TelegramClient) SendText(ctx context.Context, chatID int64, text string) error {
	_, err := t.call(ctx, "sendMessage", map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	})
	return err
}

// AnswerCallback clears the loading spinner on an inline button.
func (t *TelegramClient) AnswerCallback(ctx context.Context, callbackID, text string) error {
	_, err := t.call(ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
	})
	return err
}

// Update is the subset of a Telegram update this bot acts on.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			FirstName string `json:"first_name"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message *struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// GetUpdates long-polls for new updates starting at offset.
func (t *TelegramClient) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	raw, err := t.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	})
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, fmt.Errorf("decode updates: %w", err)
	}
	return updates, nil
}

// GetMe verifies the token at startup so a typo surfaces immediately rather
// than as silent non-delivery later.
func (t *TelegramClient) GetMe(ctx context.Context) (string, error) {
	raw, err := t.call(ctx, "getMe", nil)
	if err != nil {
		return "", err
	}
	var me struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (t *TelegramClient) call(ctx context.Context, method string, payload any) (json.RawMessage, error) {
	if t.Token == "" {
		return nil, fmt.Errorf("telegram is not configured")
	}
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	}
	url := fmt.Sprintf("%s/bot%s/%s", t.APIBase, t.Token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	defer res.Body.Close()

	// Cap the read: a malformed or hostile endpoint should not exhaust memory.
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("telegram %s: bad response (status %d)", method, res.StatusCode)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("telegram %s: %s (code %d)", method, parsed.Description, parsed.ErrorCode)
	}
	return parsed.Result, nil
}

// escapeMarkdown escapes every character MarkdownV2 treats as special. Telegram
// rejects the whole message if one is left unescaped, so this is not optional.
func escapeMarkdown(s string) string {
	const special = "_*[]()~`>#+-=|{}.!\\"
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
