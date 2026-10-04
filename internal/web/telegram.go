package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/nlp"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/scheduler"
	"github.com/shivam-20/day-remind/internal/storage"
)

const telegramHelpText = `Commands:
/add [text] - create a reminder (e.g. /add pay bill tomorrow 6pm)
/list [today|week|overdue|all] - show your reminders
/done [id] - mark a reminder done
/snooze [id] [15m|1h|3h|tomorrow] - snooze a reminder
/pause [id] /resume [id] - pause or resume a reminder
/cancel [id] - cancel a reminder
/settings - show your alert settings
/digest - toggle the morning digest
/vacation [7d|off] - pause all alerts
/unlink - disconnect this chat
/help - show this help`

func (s *Server) handleTelegramLink(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	link, err := s.db.CreateTelegramLinkToken(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create link token")
		return
	}
	stored, _ := s.db.GetDeliverySettings(r.Context())
	botHint := stored.TelegramBotUsername
	if botHint == "" {
		delivery, derr := s.deliveryConfig(r.Context())
		if derr == nil && notify.IsGateway(delivery) {
			if raw, err := notify.GatewayMe(r.Context(), delivery); err == nil {
				var info struct {
					Result struct {
						Username string `json:"username"`
					} `json:"result"`
				}
				if json.Unmarshal(raw, &info) == nil {
					botHint = info.Result.Username
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      link.Token,
		"expires_at": link.ExpiresAt.UTC().Format(time.RFC3339),
		"command":    "/start " + link.Token,
		"bot_hint":   botHint,
	})
}

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	delivery, err := s.deliveryConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "telegram not configured")
		return
	}
	// This route is public (not SSO-gated) and an update's chat ID decides whose
	// reminders a command touches, so only accept pushes when a webhook
	// transport is actually configured, and always require the shared secret.
	// In gateway mode the gateway forwards X-Telegram-Bot-Api-Secret-Token.
	if notify.IsGateway(delivery) {
		mode := strings.TrimSpace(delivery.TelegramGatewayInboundMode)
		if mode != "webhook" && mode != "both" {
			writeError(w, http.StatusNotFound, "telegram webhook disabled")
			return
		}
	} else {
		if s.cfg.TelegramUpdateMode != "webhook" {
			writeError(w, http.StatusNotFound, "telegram webhook disabled")
			return
		}
		if delivery.TelegramBotToken == "" {
			writeError(w, http.StatusServiceUnavailable, "telegram not configured")
			return
		}
	}
	secret := strings.TrimSpace(s.cfg.TelegramWebhookSecret)
	if secret == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram webhook secret not configured")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid secret")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	var update notify.TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	go func(d notify.DeliveryConfig, u notify.TelegramUpdate) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.handleTelegramUpdate(ctx, d, u)
	}(delivery, update)
}

// RunTelegram starts the configured Telegram transport (polling or webhook).
func (s *Server) RunTelegram(ctx context.Context) {
	delivery, err := s.deliveryConfig(ctx)
	if err != nil || (!notify.IsGateway(delivery) && delivery.TelegramBotToken == "") {
		log.Printf("telegram: not configured")
		return
	}
	if notify.IsGateway(delivery) {
		// Gateway manages the Telegram connection. We only need to receive
		// updates: either poll the gateway's inbound queue or accept webhook
		// pushes (the latter needs no extra goroutine).
		mode := strings.TrimSpace(delivery.TelegramGatewayInboundMode)
		if mode == "poll" || mode == "both" {
			go s.runGatewayPolling(ctx)
		}
		log.Printf("telegram: gateway mode (inbound=%s)", mode)
		return
	}
	switch s.cfg.TelegramUpdateMode {
	case "off":
		return
	case "webhook":
		s.bootstrapTelegramWebhook(ctx)
	default:
		s.runTelegramPolling(ctx)
	}
}

// runGatewayPolling pulls updates from the telegram-gateway /v1/inbound/updates
// endpoint and acknowledges them after processing.
func (s *Server) runGatewayPolling(ctx context.Context) {
	log.Printf("telegram: gateway polling started")
	var lastAckedID int64
	for {
		if ctx.Err() != nil {
			return
		}
		delivery, err := s.deliveryConfig(ctx)
		if err != nil || !notify.IsGateway(delivery) {
			if !sleepCtx(ctx, 15*time.Second) {
				return
			}
			continue
		}
		updates, ids, err := notify.GetGatewayInboundUpdates(ctx, delivery, lastAckedID, 50)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram gateway poll: %v", err)
			if !sleepCtx(ctx, 5*time.Second) {
				return
			}
			continue
		}
		for _, update := range updates {
			uctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			s.handleTelegramUpdate(uctx, delivery, update)
			cancel()
		}
		if len(ids) > 0 {
			if err := notify.AckGatewayInbound(ctx, delivery, ids); err != nil {
				log.Printf("telegram gateway ack: %v", err)
			}
			lastAckedID = ids[len(ids)-1]
		} else if !sleepCtx(ctx, 2*time.Second) {
			return
		}
	}
}

func (s *Server) bootstrapTelegramWebhook(ctx context.Context) {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/")
	if !strings.HasPrefix(base, "https://") {
		log.Printf("telegram: webhook mode needs an https WEBHOOK_BASE_URL; falling back to polling")
		s.runTelegramPolling(ctx)
		return
	}
	if strings.TrimSpace(s.cfg.TelegramWebhookSecret) == "" {
		log.Printf("telegram: webhook mode needs TELEGRAM_WEBHOOK_SECRET; falling back to polling")
		s.runTelegramPolling(ctx)
		return
	}
	delivery, err := s.deliveryConfig(ctx)
	if err != nil || delivery.TelegramBotToken == "" {
		log.Printf("telegram: webhook mode enabled but no bot token configured")
		return
	}
	url := base + "/api/webhooks/telegram"
	if err := notify.SetTelegramWebhook(ctx, delivery, url, s.cfg.TelegramWebhookSecret); err != nil {
		log.Printf("telegram: setWebhook failed: %v", err)
		return
	}
	log.Printf("telegram: webhook registered at %s", url)
}

func (s *Server) runTelegramPolling(ctx context.Context) {
	var offset int64
	if delivery, err := s.deliveryConfig(ctx); err == nil && delivery.TelegramBotToken != "" {
		if err := notify.DeleteTelegramWebhook(ctx, delivery); err != nil {
			log.Printf("telegram: deleteWebhook: %v", err)
		}
	}
	for {
		if ctx.Err() != nil {
			return
		}
		delivery, err := s.deliveryConfig(ctx)
		if err != nil || delivery.TelegramBotToken == "" {
			if !sleepCtx(ctx, 15*time.Second) {
				return
			}
			continue
		}
		updates, err := notify.GetTelegramUpdates(ctx, delivery, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram poll: %v", err)
			if !sleepCtx(ctx, 5*time.Second) {
				return
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			uctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			s.handleTelegramUpdate(uctx, delivery, update)
			cancel()
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Server) handleTelegramUpdate(ctx context.Context, delivery notify.DeliveryConfig, update notify.TelegramUpdate) {
	if update.CallbackQuery != nil {
		s.handleTelegramCallback(ctx, delivery, *update.CallbackQuery)
		return
	}
	msg := update.Message
	if msg == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	cmd, args := parseTelegramCommand(text)
	if cmd == "" {
		return
	}
	firstName, username := "", ""
	if msg.From != nil {
		firstName = msg.From.FirstName
		username = msg.From.Username
	}

	switch {
	case cmd == "/start":
		if len(args) > 0 {
			if err := s.db.ConsumeTelegramLinkToken(ctx, args[0], chatID, username, firstName); err == nil {
				s.telegramWelcome(ctx, delivery, chatID, firstName, true)
			} else {
				s.replyHTML(ctx, delivery, chatID, "That link is invalid or expired. Generate a new one in Day Remind → Settings → Alerts.")
			}
		} else {
			s.telegramWelcome(ctx, delivery, chatID, firstName, false)
		}
	case cmd == "/help":
		s.replyHTML(ctx, delivery, chatID, telegramHelpText)
	case cmd == "/add":
		s.telegramAdd(ctx, delivery, chatID, strings.TrimSpace(strings.Join(args, " ")))
	case cmd == "/list":
		filter := ""
		if len(args) > 0 {
			filter = args[0]
		}
		s.telegramList(ctx, delivery, chatID, filter)
	case cmd == "/done":
		s.telegramDone(ctx, delivery, chatID, args)
	case cmd == "/snooze":
		s.telegramSnooze(ctx, delivery, chatID, args)
	case cmd == "/pause":
		s.telegramPause(ctx, delivery, chatID, args)
	case cmd == "/resume":
		s.telegramResume(ctx, delivery, chatID, args)
	case cmd == "/cancel":
		s.telegramCancel(ctx, delivery, chatID, args)
	case cmd == "/settings":
		s.telegramSettings(ctx, delivery, chatID)
	case cmd == "/digest":
		s.telegramDigest(ctx, delivery, chatID)
	case cmd == "/vacation":
		s.telegramVacation(ctx, delivery, chatID, args)
	case cmd == "/unlink":
		s.telegramUnlink(ctx, delivery, chatID)
	default:
		s.replyHTML(ctx, delivery, chatID, "Unknown command. Type /help to see what I can do.")
	}
}

func (s *Server) telegramWelcome(ctx context.Context, delivery notify.DeliveryConfig, chatID, name string, linked bool) {
	greeting := "Welcome to Day Remind!"
	if strings.TrimSpace(name) != "" {
		greeting = "Welcome, " + notify.EscapeTelegramHTML(name) + "!"
	}
	var b strings.Builder
	b.WriteString(greeting)
	b.WriteString("\n\n")
	if linked {
		b.WriteString("Your Telegram account is now linked. Reminder alerts will arrive here.\n\n")
	} else if _, err := s.db.GetUserByTelegramChatID(ctx, chatID); err != nil {
		b.WriteString("Link your Day Remind account to start creating reminders here: open the web app, go to Settings → Alerts → Link Telegram, and send the code shown.\n\n")
	}
	b.WriteString("I can create and manage reminders right here in Telegram.\n\n")
	b.WriteString(telegramHelpText)
	var keyboard *notify.TelegramInlineKeyboard
	if url := s.appURL(); url != "" {
		keyboard = &notify.TelegramInlineKeyboard{
			InlineKeyboard: [][]notify.TelegramInlineButton{{{Text: "Open Day Remind", URL: url}}},
		}
	}
	_ = notify.SendTelegramMessage(ctx, delivery, chatID, b.String(), keyboard)
}

func (s *Server) telegramAdd(ctx context.Context, delivery notify.DeliveryConfig, chatID, text string) {
	if text == "" {
		s.replyHTML(ctx, delivery, chatID, "Usage: /add [text]\nExample: /add pay electricity bill tomorrow 6pm")
		return
	}
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return
	}
	loc := loadLocation(user.Timezone)
	now := time.Now().UTC()
	parsed := nlp.Parse(text, now, loc)
	title := strings.TrimSpace(parsed.Title)
	if title == "" {
		title = text
	}
	dueAt := parsed.DueAt
	if dueAt.IsZero() {
		local := now.In(loc)
		dueAt = time.Date(local.Year(), local.Month(), local.Day()+1, 9, 0, 0, 0, loc)
	}
	priority := parsed.Priority
	if priority == "" {
		priority = reminder.PriorityNormal
	}
	item, err := s.db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title:          title,
		DueAt:          dueAt,
		Timezone:       user.Timezone,
		Priority:       priority,
		RecurrenceRule: parsed.RecurrenceRule,
	})
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not create reminder: "+notify.EscapeTelegramHTML(err.Error()))
		return
	}
	msg := "Added: <b>" + notify.EscapeTelegramHTML(item.Title) + "</b>\nDue: " + item.DueAt.In(loc).Format("Mon Jan 2 15:04") + " (" + user.Timezone + ")"
	_ = notify.SendTelegramMessage(ctx, delivery, chatID, msg, notify.TelegramReminderKeyboard(item.ID.String(), s.reminderURL(item.ID)))
}

func (s *Server) telegramList(ctx context.Context, delivery notify.DeliveryConfig, chatID, filter string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return
	}
	loc := loadLocation(user.Timezone)
	now := time.Now().In(loc)
	f := reminder.ListFilter{UserID: user.ID, Limit: 10, Status: reminder.StatusScheduled}
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		f.From, f.To = &start, &end
	case "week":
		start := now
		end := now.AddDate(0, 0, 7)
		f.From, f.To = &start, &end
	case "overdue":
		end := now
		f.To = &end
	case "all":
		f.Status = ""
		f.Limit = 15
	case "":
	default:
		s.replyHTML(ctx, delivery, chatID, "Usage: /list [today|week|overdue|all]")
		return
	}
	items, err := s.db.ListReminders(ctx, f)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not load reminders.")
		return
	}
	if len(items) == 0 {
		s.replyHTML(ctx, delivery, chatID, "No reminders found.")
		return
	}
	var b strings.Builder
	label := "Upcoming"
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "today":
		label = "Today"
	case "week":
		label = "This week"
	case "overdue":
		label = "Overdue"
	case "all":
		label = "All reminders"
	}
	b.WriteString(label + ":\n")
	for _, item := range items {
		id := item.ID.String()
		short := id
		if len(short) > 8 {
			short = short[:8]
		}
		b.WriteString("- <b>" + notify.EscapeTelegramHTML(item.Title) + "</b> (" + short + ")\n  ")
		b.WriteString(item.DueAt.In(loc).Format("Mon Jan 2 15:04"))
		if item.Status != reminder.StatusScheduled {
			b.WriteString(" · " + item.Status)
		}
		b.WriteString("\n")
		if b.Len() > 3200 {
			b.WriteString("…")
			break
		}
	}
	_ = notify.SendTelegramMessage(ctx, delivery, chatID, b.String(), nil)
}

func (s *Server) telegramDone(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		s.replyHTML(ctx, delivery, chatID, "Usage: /done [reminder id]")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	status := reminder.StatusDone
	done, err := s.db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &status})
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not update reminder.")
		return
	}
	s.hooks.EmitDone(ctx, done)
	scheduler.ScheduleNextRecurrenceIfNeeded(ctx, s.db, user.ID, item.ID)
	s.replyHTML(ctx, delivery, chatID, "Marked done: "+notify.EscapeTelegramHTML(item.Title))
}

func (s *Server) telegramCancel(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		s.replyHTML(ctx, delivery, chatID, "Usage: /cancel [reminder id]")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	status := reminder.StatusCancelled
	if _, err := s.db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &status}); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not update reminder.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Cancelled: "+notify.EscapeTelegramHTML(item.Title))
}

func (s *Server) telegramPause(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		s.replyHTML(ctx, delivery, chatID, "Usage: /pause [reminder id]")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	if _, err := s.db.PauseReminder(ctx, user.ID, item.ID, nil); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not pause reminder.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Paused: "+notify.EscapeTelegramHTML(item.Title))
}

func (s *Server) telegramResume(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		s.replyHTML(ctx, delivery, chatID, "Usage: /resume [reminder id]")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	if _, err := s.db.ResumeReminder(ctx, user.ID, item.ID); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not resume reminder.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Resumed: "+notify.EscapeTelegramHTML(item.Title))
}

func (s *Server) telegramSnooze(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	if len(args) == 0 {
		s.replyHTML(ctx, delivery, chatID, "Usage: /snooze [reminder id] [15m|1h|3h|tomorrow]")
		return
	}
	user, item, ok := s.findTelegramReminder(ctx, delivery, chatID, args[0])
	if !ok {
		return
	}
	preset := "15m"
	if len(args) > 1 {
		preset = normalizeSnoozePreset(args[1])
	}
	until, err := reminder.SnoozeUntil(preset, time.Now().UTC(), item.Timezone)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Unknown snooze option. Use 15m, 1h, 3h or tomorrow.")
		return
	}
	if _, err := s.db.SnoozeReminder(ctx, user.ID, item.ID, until); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not snooze reminder.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Snoozed: "+notify.EscapeTelegramHTML(item.Title))
}

func normalizeSnoozePreset(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "tomorrow", "tomorrow9am", "tomorrow_9am", "tomorrow 9am":
		return "tomorrow_9am"
	case "+15", "15", "15min", "15m":
		return "15m"
	case "+1h", "1h", "1hr":
		return "1h"
	case "+3h", "3h", "3hr":
		return "3h"
	default:
		return v
	}
}

func (s *Server) telegramSettings(ctx context.Context, delivery notify.DeliveryConfig, chatID string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return
	}
	prefs, _ := s.db.GetUserNotificationPrefs(ctx, user.ID)
	morning := false
	var quietStart, quietEnd string
	if settings, err := s.db.LoadUserSettings(ctx, user.ID); err == nil {
		morning = settings.MorningDigestEnabled
	}
	quietStart, quietEnd, _ = s.db.GetUserQuietHours(ctx, user.ID)
	emailEnabled, telegramEnabled := false, false
	for _, p := range prefs {
		switch p.Channel {
		case storage.ChannelEmail:
			emailEnabled = p.Enabled
		case storage.ChannelTelegram:
			telegramEnabled = p.Enabled
		}
	}
	var b strings.Builder
	b.WriteString("Your alert settings:\n")
	b.WriteString("- Email alerts: " + yesNo(emailEnabled) + "\n")
	b.WriteString("- Telegram alerts: " + yesNo(telegramEnabled) + "\n")
	b.WriteString("- Morning digest: " + yesNo(morning) + "\n")
	if quietStart != "" || quietEnd != "" {
		b.WriteString("- Quiet hours: " + quietStart + "–" + quietEnd + "\n")
	}
	if user.NotificationsPaused {
		b.WriteString("- Status: PAUSED")
		if user.NotificationsPausedUntil != nil {
			b.WriteString(" until " + user.NotificationsPausedUntil.UTC().Format("Jan 2 15:04") + " UTC")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nChange details in the web app, or use /digest and /vacation here.")
	keyboard := &notify.TelegramInlineKeyboard{InlineKeyboard: [][]notify.TelegramInlineButton{
		{
			{Text: "Toggle digest", CallbackData: notify.TelegramCallbackData("digest", "toggle", "")},
			{Text: "Vacation off", CallbackData: notify.TelegramCallbackData("vacation", "off", "")},
		},
	}}
	_ = notify.SendTelegramMessage(ctx, delivery, chatID, b.String(), keyboard)
}

func (s *Server) telegramDigest(ctx context.Context, delivery notify.DeliveryConfig, chatID string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return
	}
	settings, err := s.db.LoadUserSettings(ctx, user.ID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not load settings.")
		return
	}
	enabled := !settings.MorningDigestEnabled
	if err := s.db.UpdateMorningDigest(ctx, user.ID, enabled, settings.MorningDigestTime); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not update the digest.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Morning digest is now "+onOff(enabled)+".")
}

func (s *Server) telegramVacation(ctx context.Context, delivery notify.DeliveryConfig, chatID string, args []string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return
	}
	arg := ""
	if len(args) > 0 {
		arg = strings.ToLower(strings.TrimSpace(args[0]))
	}
	if arg == "off" || arg == "resume" || arg == "0" || arg == "0d" {
		if err := s.db.SetNotificationsPaused(ctx, user.ID, false, nil); err != nil {
			s.replyHTML(ctx, delivery, chatID, "Could not resume alerts.")
			return
		}
		s.replyHTML(ctx, delivery, chatID, "Alerts resumed.")
		return
	}
	days := 7
	if arg != "" {
		trimmed := strings.TrimSuffix(arg, "d")
		if n, err := strconv.Atoi(trimmed); err == nil && n > 0 {
			days = n
		}
	}
	until := time.Now().UTC().AddDate(0, 0, days)
	if err := s.db.SetNotificationsPaused(ctx, user.ID, true, &until); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not pause alerts.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "Alerts paused for "+strconv.Itoa(days)+" day(s). Send /vacation off to resume early.")
}

func (s *Server) telegramUnlink(ctx context.Context, delivery notify.DeliveryConfig, chatID string) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "This chat is not linked to a Day Remind account.")
		return
	}
	if err := s.db.UnlinkTelegram(ctx, user.ID); err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not unlink. Try again later.")
		return
	}
	s.replyHTML(ctx, delivery, chatID, "This chat has been unlinked. Send /start to link again.")
}

func (s *Server) handleTelegramCallback(ctx context.Context, delivery notify.DeliveryConfig, cb notify.TelegramCallback) {
	if cb.Message == nil {
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "This action is no longer available.")
		return
	}
	chatID := strconv.FormatInt(cb.Message.Chat.ID, 10)
	action, id, arg := notify.ParseTelegramCallbackData(cb.Data)
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Link your account first.")
		return
	}
	if action == "digest" {
		settings, err := s.db.LoadUserSettings(ctx, user.ID)
		if err == nil {
			_ = s.db.UpdateMorningDigest(ctx, user.ID, !settings.MorningDigestEnabled, settings.MorningDigestTime)
		}
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Digest toggled.")
		s.telegramSettings(ctx, delivery, chatID)
		return
	}
	if action == "vacation" || (action == "resume" && id == "all") {
		_ = s.db.SetNotificationsPaused(ctx, user.ID, false, nil)
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Alerts resumed.")
		s.telegramSettings(ctx, delivery, chatID)
		return
	}
	reminderID, err := uuid.Parse(id)
	if err != nil {
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Unknown action.")
		return
	}
	item, err := s.db.GetReminder(ctx, user.ID, reminderID)
	if err != nil {
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Reminder not found.")
		return
	}
	switch action {
	case notify.TelegramCallbackDone:
		status := reminder.StatusDone
		done, err := s.db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &status})
		if err != nil {
			_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Could not update.")
			return
		}
		s.hooks.EmitDone(ctx, done)
		scheduler.ScheduleNextRecurrenceIfNeeded(ctx, s.db, user.ID, item.ID)
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Done")
		s.editAlert(ctx, delivery, chatID, cb.Message.MessageID, "Done: "+notify.EscapeTelegramHTML(item.Title))
	case notify.TelegramCallbackSnooze:
		if arg == "" {
			arg = "1h"
		}
		until, err := reminder.SnoozeUntil(normalizeSnoozePreset(arg), time.Now().UTC(), item.Timezone)
		if err != nil {
			_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Unknown snooze option.")
			return
		}
		if _, err := s.db.SnoozeReminder(ctx, user.ID, item.ID, until); err != nil {
			_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Could not snooze.")
			return
		}
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Snoozed")
		s.editAlert(ctx, delivery, chatID, cb.Message.MessageID, "Snoozed: "+notify.EscapeTelegramHTML(item.Title))
	default:
		_ = notify.AnswerTelegramCallback(ctx, delivery, cb.ID, "Unknown action.")
	}
}

func (s *Server) editAlert(ctx context.Context, delivery notify.DeliveryConfig, chatID string, messageID int64, text string) {
	_ = notify.EditTelegramMessage(ctx, delivery, chatID, messageID, text, nil)
}

func (s *Server) findTelegramReminder(ctx context.Context, delivery notify.DeliveryConfig, chatID, prefix string) (storage.User, reminder.Reminder, bool) {
	user, err := s.db.GetUserByTelegramChatID(ctx, chatID)
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Link your account first from Day Remind → Settings → Alerts.")
		return storage.User{}, reminder.Reminder{}, false
	}
	if id, err := uuid.Parse(strings.TrimSpace(prefix)); err == nil {
		if item, err := s.db.GetReminder(ctx, user.ID, id); err == nil {
			return user, item, true
		}
	}
	items, err := s.db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID, Limit: 100})
	if err != nil {
		s.replyHTML(ctx, delivery, chatID, "Could not load reminders.")
		return storage.User{}, reminder.Reminder{}, false
	}
	prefix = strings.ToLower(prefix)
	for _, item := range items {
		id := strings.ToLower(item.ID.String())
		if strings.HasPrefix(id, prefix) || strings.HasPrefix(id, strings.ReplaceAll(prefix, "-", "")) {
			return user, item, true
		}
	}
	s.replyHTML(ctx, delivery, chatID, "Reminder not found.")
	return storage.User{}, reminder.Reminder{}, false
}

func (s *Server) replyHTML(ctx context.Context, delivery notify.DeliveryConfig, chatID, text string) {
	_ = notify.SendTelegramMessage(ctx, delivery, chatID, text, nil)
}

func (s *Server) appURL() string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/"
}

func (s *Server) reminderURL(id uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/#/reminders?open=" + id.String()
}

func parseTelegramCommand(text string) (cmd string, args []string) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return "", nil
	}
	cmd = strings.ToLower(parts[0])
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if len(parts) == 1 {
		return cmd, nil
	}
	return cmd, parts[1:]
}

func loadLocation(tz string) *time.Location {
	if loc, err := time.LoadLocation(strings.TrimSpace(tz)); err == nil && loc != nil {
		return loc
	}
	return time.UTC
}

func yesNo(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func onOff(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}
