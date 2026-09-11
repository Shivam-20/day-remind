package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

func (s *Server) handleTelegramLink(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	link, err := s.db.CreateTelegramLinkToken(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create link token")
		return
	}
	stored, _ := s.db.GetDeliverySettings(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      link.Token,
		"expires_at": link.ExpiresAt.UTC().Format(time.RFC3339),
		"command":    "/start " + link.Token,
		"bot_hint":   stored.TelegramBotUsername,
	})
}

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	delivery, err := s.effectiveDelivery(r)
	if err != nil || delivery.TelegramBotToken == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram not configured")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var update struct {
		Message *struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &update); err != nil || update.Message == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
	text := strings.TrimSpace(update.Message.Text)
	parts := strings.Fields(text)
	if len(parts) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch {
	case strings.HasPrefix(cmd, "/start"):
		if len(args) > 0 {
			if err := s.db.ConsumeTelegramLinkToken(r.Context(), args[0], chatID); err == nil {
				replyTelegram(r.Context(), delivery, chatID, "Telegram linked! You will receive reminder alerts here.")
			} else {
				replyTelegram(r.Context(), delivery, chatID, "Invalid or expired link token. Generate a new link in Day Remind settings.")
			}
		} else {
			replyTelegram(r.Context(), delivery, chatID, "Send /start <token> from Day Remind Settings → Alert channels → Link Telegram.")
		}
	case cmd == "/help":
		replyTelegram(r.Context(), delivery, chatID, "Commands: /list, /done <id>, /snooze <id>, /help")
	case cmd == "/list":
		s.telegramList(r.Context(), delivery, chatID)
	case cmd == "/done":
		s.telegramDone(r.Context(), delivery, chatID, args)
	case cmd == "/snooze":
		s.telegramSnooze(r.Context(), delivery, chatID, args)
	default:
		replyTelegram(r.Context(), delivery, chatID, "Unknown command. Try /help")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) telegramList(ctx context.Context, delivery notify.DeliveryConfig, chatID string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		replyTelegram(ctx, delivery, chatID, "Link your account first in Day Remind settings.")
		return
	}
	items, err := s.db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID, Status: reminder.StatusScheduled, Limit: 10})
	if err != nil || len(items) == 0 {
		replyTelegram(ctx, delivery, chatID, "No upcoming reminders.")
		return
	}
	var b strings.Builder
	b.WriteString("Upcoming:\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item.ID.String()[:8])
		b.WriteString(" ")
		b.WriteString(item.Title)
		b.WriteString(" @ ")
		b.WriteString(item.DueAt.UTC().Format("Jan 2 15:04"))
		b.WriteString("\n")
	}
	replyTelegram(ctx, delivery, chatID, b.String())
}

func (s *Server) telegramDone(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		replyTelegram(ctx, delivery, chatID, "Usage: /done <reminder-id-prefix>")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	status := reminder.StatusDone
	if _, err := s.db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &status}); err != nil {
		replyTelegram(ctx, delivery, chatID, "Could not update reminder.")
		return
	}
	replyTelegram(ctx, delivery, chatID, "Marked done: "+item.Title)
}

func (s *Server) telegramSnooze(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		replyTelegram(ctx, delivery, chatID, "Usage: /snooze <reminder-id-prefix>")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	until, err := reminder.SnoozeUntil("+15m", time.Now().UTC(), item.Timezone)
	if err != nil {
		replyTelegram(ctx, delivery, chatID, "Could not snooze.")
		return
	}
	if _, err := s.db.SnoozeReminder(ctx, user.ID, item.ID, until); err != nil {
		replyTelegram(ctx, delivery, chatID, "Could not snooze.")
		return
	}
	replyTelegram(ctx, delivery, chatID, "Snoozed 15m: "+item.Title)
}

func (s *Server) findTelegramReminder(ctx context.Context, delivery notify.DeliveryConfig, chatID, prefix string) (storage.User, reminder.Reminder, bool) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		replyTelegram(ctx, delivery, chatID, "Link your account first.")
		return storage.User{}, reminder.Reminder{}, false
	}
	items, err := s.db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID, Limit: 100})
	if err != nil {
		replyTelegram(ctx, delivery, chatID, "Could not load reminders.")
		return storage.User{}, reminder.Reminder{}, false
	}
	prefix = strings.ToLower(prefix)
	for _, item := range items {
		id := strings.ToLower(item.ID.String())
		if strings.HasPrefix(id, prefix) || strings.HasPrefix(id, strings.ReplaceAll(prefix, "-", "")) {
			return user, item, true
		}
	}
	replyTelegram(ctx, delivery, chatID, "Reminder not found.")
	return storage.User{}, reminder.Reminder{}, false
}

func replyTelegram(ctx context.Context, delivery notify.DeliveryConfig, chatID, text string) {
	_ = notify.SendTelegram(ctx, delivery, chatID, text)
}
