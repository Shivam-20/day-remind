// Package bot runs the Telegram command interface: long-polls for updates and
// maps chat commands onto the same storage operations the web API uses.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Bot consumes Telegram updates. Authorization is by linked chat: a chat that
// is not bound to an account can only run /start and /help.
type Bot struct {
	db       *storage.DB
	resolver *delivery.Resolver
	apiBase  string

	// client and token are refreshed from the database each cycle, so an admin
	// can change the bot token without restarting the process.
	client *notify.TelegramClient
	token  string

	offset int64
	now    func() time.Time
}

func New(db *storage.DB, resolver *delivery.Resolver, apiBase string) *Bot {
	return &Bot{
		db:       db,
		resolver: resolver,
		apiBase:  apiBase,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// refreshClient reconciles the running client with the stored token. It reports
// whether the bot is configured and ready to poll.
func (b *Bot) refreshClient(ctx context.Context) bool {
	snap, err := b.resolver.Snapshot(ctx)
	if err != nil {
		log.Printf("bot: could not read settings: %v", err)
		return false
	}
	token := snap.InstanceTelegramToken()
	if token == "" {
		if b.client != nil {
			log.Println("bot: token removed, stopping polling")
			b.client, b.token = nil, ""
		}
		return false
	}
	if b.client != nil && token == b.token {
		return true
	}

	client := notify.NewTelegramClient(token, b.apiBase)
	username, err := client.GetMe(ctx)
	if err != nil {
		log.Printf("bot: token rejected: %v", err)
		b.client, b.token = nil, ""
		return false
	}
	b.client, b.token = client, token

	// A different bot has a different update stream. Skip whatever is queued
	// rather than replaying it, which would re-run commands sent hours ago.
	b.offset = 0
	if updates, err := client.GetUpdates(ctx, -1, 0); err == nil && len(updates) > 0 {
		b.offset = updates[len(updates)-1].UpdateID + 1
	}
	log.Printf("bot: connected as @%s", username)
	return true
}

// Run long-polls until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	log.Println("bot: started")

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			log.Println("bot: stopped")
			return
		}
		if !b.refreshClient(ctx) {
			// Not configured, or the token was rejected. Wait and look again
			// so switching it on in the admin panel takes effect by itself.
			select {
			case <-ctx.Done():
				log.Println("bot: stopped")
				return
			case <-time.After(15 * time.Second):
			}
			continue
		}
		updates, err := b.client.GetUpdates(ctx, b.offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("bot: stopped")
				return
			}
			log.Printf("bot: getUpdates: %v (retrying in %s)", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			// Back off to a ceiling so a persistent outage does not hot-loop.
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		for _, u := range updates {
			// Advance past this update even if handling fails, so one bad
			// message cannot wedge the loop forever.
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			b.handle(ctx, u)
		}
	}
}

func (b *Bot) handle(ctx context.Context, u notify.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("bot: panic handling update %d: %v", u.UpdateID, r)
		}
	}()

	switch {
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u)
	case u.Message != nil && strings.TrimSpace(u.Message.Text) != "":
		b.handleMessage(ctx, u.Message.Chat.ID, strings.TrimSpace(u.Message.Text))
	}
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if err := b.client.SendText(ctx, chatID, text); err != nil {
		log.Printf("bot: reply to %d failed: %v", chatID, err)
	}
}

const helpText = `*Day Remind*

/start <code> — link this chat to your account
/list — your next reminders
/add <when> | <title> — create a reminder
/done <number> — complete one from /list
/snooze <number> <duration> — push one back
/unlink — disconnect this chat
/help — this message

*Examples*
/add tomorrow 9am | Pay electricity bill
/add 2026-12-25 08:00 | Christmas
/add in 3h | Call back
/snooze 2 1h`

func (b *Bot) handleMessage(ctx context.Context, chatID int64, text string) {
	cmd, rest := splitCommand(text)

	// /start and /help are the only commands available before linking.
	switch cmd {
	case "/start":
		b.cmdStart(ctx, chatID, rest)
		return
	case "/help":
		b.reply(ctx, chatID, helpText)
		return
	}

	user, err := b.db.GetUserByTelegramChat(ctx, chatID)
	if errors.Is(err, storage.ErrNotFound) {
		b.reply(ctx, chatID, escape("This chat is not linked yet. Open Settings in Day Remind, get your code, then send: /start <code>"))
		return
	}
	if err != nil {
		log.Printf("bot: lookup chat %d: %v", chatID, err)
		b.reply(ctx, chatID, escape("Something went wrong. Please try again."))
		return
	}

	switch cmd {
	case "/list":
		b.cmdList(ctx, chatID, user)
	case "/add":
		b.cmdAdd(ctx, chatID, user, rest)
	case "/done":
		b.cmdDone(ctx, chatID, user, rest)
	case "/snooze":
		b.cmdSnooze(ctx, chatID, user, rest)
	case "/unlink":
		b.cmdUnlink(ctx, chatID, user)
	default:
		b.reply(ctx, chatID, escape("Unknown command. Send /help to see what I understand."))
	}
}

func (b *Bot) cmdStart(ctx context.Context, chatID int64, rest string) {
	code := strings.ToUpper(strings.TrimSpace(rest))
	if code == "" {
		b.reply(ctx, chatID, escape("Welcome to Day Remind. Open Settings in the app, copy your link code, then send: /start <code>"))
		return
	}
	user, err := b.db.RedeemTelegramLinkCode(ctx, code, chatID)
	if errors.Is(err, storage.ErrNotFound) {
		b.reply(ctx, chatID, escape("That code is not valid or has expired. Generate a fresh one in Settings."))
		return
	}
	if err != nil {
		log.Printf("bot: redeem code: %v", err)
		b.reply(ctx, chatID, escape("Something went wrong linking this chat. Please try again."))
		return
	}
	name := user.DisplayName
	if name == "" {
		name = user.Email
	}
	b.reply(ctx, chatID, "*Linked\\.* "+escape("You'll get reminders here, "+name+". Send /help for commands."))
}

func (b *Bot) cmdUnlink(ctx context.Context, chatID int64, user storage.User) {
	if _, err := b.db.UnlinkTelegram(ctx, user.ID); err != nil {
		log.Printf("bot: unlink: %v", err)
		b.reply(ctx, chatID, escape("Could not unlink. Please try again."))
		return
	}
	b.reply(ctx, chatID, escape("This chat is no longer linked. Telegram notifications are off."))
}

// listWindow is how many upcoming reminders /list shows, and the set that
// /done and /snooze index into.
const listWindow = 10

func (b *Bot) cmdList(ctx context.Context, chatID int64, user storage.User) {
	items, err := b.openReminders(ctx, user)
	if err != nil {
		log.Printf("bot: list: %v", err)
		b.reply(ctx, chatID, escape("Could not load your reminders."))
		return
	}
	if len(items) == 0 {
		b.reply(ctx, chatID, escape("Nothing scheduled. Add one with: /add tomorrow 9am | Title"))
		return
	}
	var sb strings.Builder
	sb.WriteString("*Your next reminders*\n")
	for i, r := range items {
		mark := ""
		if storage.FireTime(r).Before(b.now()) {
			mark = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("\n%d\\. %s\n    %s%s\n", i+1, escape(r.Title), escape(formatWhen(r, user)), mark))
	}
	sb.WriteString("\n" + escape("Use /done <number> or /snooze <number> <duration>."))
	b.reply(ctx, chatID, sb.String())
}

func (b *Bot) cmdAdd(ctx context.Context, chatID int64, user storage.User, rest string) {
	whenRaw, title, ok := strings.Cut(rest, "|")
	if !ok {
		b.reply(ctx, chatID, escape("Use: /add <when> | <title>   for example  /add tomorrow 9am | Pay the bill"))
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		b.reply(ctx, chatID, escape("The title is empty. Use: /add <when> | <title>"))
		return
	}
	loc := userLocation(user)
	due, err := ParseWhen(strings.TrimSpace(whenRaw), b.now().In(loc), loc)
	if err != nil {
		b.reply(ctx, chatID, escape("I could not read \""+strings.TrimSpace(whenRaw)+"\" as a time. Try: tomorrow 9am, in 2h, or 2026-12-25 08:00"))
		return
	}

	item, err := b.db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title:    title,
		DueAt:    due,
		Timezone: user.Timezone,
	})
	if err != nil {
		log.Printf("bot: add: %v", err)
		b.reply(ctx, chatID, escape("Could not create that reminder."))
		return
	}
	b.reply(ctx, chatID, "*Added\\.* "+escape(item.Title)+"\n"+escape(formatWhen(item, user)))
}

func (b *Bot) cmdDone(ctx context.Context, chatID int64, user storage.User, rest string) {
	item, ok := b.resolveIndex(ctx, chatID, user, strings.TrimSpace(rest), "/done <number>")
	if !ok {
		return
	}
	status := reminder.StatusDone
	if _, err := b.db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &status}); err != nil {
		log.Printf("bot: done: %v", err)
		b.reply(ctx, chatID, escape("Could not update that reminder."))
		return
	}
	b.reply(ctx, chatID, "✓ "+escape("Done: "+item.Title))
}

func (b *Bot) cmdSnooze(ctx context.Context, chatID int64, user storage.User, rest string) {
	numPart, durPart, _ := strings.Cut(strings.TrimSpace(rest), " ")
	item, ok := b.resolveIndex(ctx, chatID, user, numPart, "/snooze <number> <duration>")
	if !ok {
		return
	}
	durPart = strings.TrimSpace(durPart)
	if durPart == "" {
		durPart = "1h"
	}
	d, err := ParseDuration(durPart)
	if err != nil {
		b.reply(ctx, chatID, escape("I could not read \""+durPart+"\" as a duration. Try 30m, 2h or 1d."))
		return
	}
	until := b.now().Add(d)
	if _, err := b.db.SnoozeReminder(ctx, user.ID, item.ID, until); err != nil {
		log.Printf("bot: snooze: %v", err)
		b.reply(ctx, chatID, escape("Could not snooze that reminder."))
		return
	}
	b.reply(ctx, chatID, escape(fmt.Sprintf("Snoozed %q until %s", item.Title, until.In(userLocation(user)).Format("Mon 2 Jan 15:04"))))
}

// resolveIndex maps the 1-based number a user typed onto the same ordered list
// /list showed them.
func (b *Bot) resolveIndex(ctx context.Context, chatID int64, user storage.User, raw, usage string) (reminder.Reminder, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		b.reply(ctx, chatID, escape("Use: "+usage+"   (the number comes from /list)"))
		return reminder.Reminder{}, false
	}
	items, err := b.openReminders(ctx, user)
	if err != nil {
		log.Printf("bot: resolve: %v", err)
		b.reply(ctx, chatID, escape("Could not load your reminders."))
		return reminder.Reminder{}, false
	}
	if n > len(items) {
		b.reply(ctx, chatID, escape(fmt.Sprintf("There is no number %d. /list shows %d.", n, len(items))))
		return reminder.Reminder{}, false
	}
	return items[n-1], true
}

// openReminders is the canonical ordering shared by /list, /done and /snooze,
// so a number always refers to the same reminder.
func (b *Bot) openReminders(ctx context.Context, user storage.User) ([]reminder.Reminder, error) {
	items, err := b.db.ListReminders(ctx, reminder.ListFilter{
		UserID: user.ID,
		Status: reminder.StatusScheduled,
		Limit:  listWindow,
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (b *Bot) handleCallback(ctx context.Context, u notify.Update) {
	cb := u.CallbackQuery
	var chatID int64
	if cb.Message != nil {
		chatID = cb.Message.Chat.ID
	} else {
		chatID = cb.From.ID
	}

	user, err := b.db.GetUserByTelegramChat(ctx, chatID)
	if err != nil {
		_ = b.client.AnswerCallback(ctx, cb.ID, "This chat is not linked.")
		return
	}
	action, rawID, _ := strings.Cut(cb.Data, ":")
	id, err := uuid.Parse(rawID)
	if err != nil {
		_ = b.client.AnswerCallback(ctx, cb.ID, "Unknown reminder.")
		return
	}

	var note string
	switch action {
	case "done":
		status := reminder.StatusDone
		_, err = b.db.UpdateReminder(ctx, user.ID, id, reminder.UpdateInput{Status: &status})
		note = "Marked done"
	case "snooze1h":
		_, err = b.db.SnoozeReminder(ctx, user.ID, id, b.now().Add(time.Hour))
		note = "Snoozed 1 hour"
	case "snooze9am":
		_, err = b.db.SnoozeReminder(ctx, user.ID, id, nextMorning(b.now(), userLocation(user)))
		note = "Snoozed to 9am tomorrow"
	default:
		_ = b.client.AnswerCallback(ctx, cb.ID, "Unknown action.")
		return
	}

	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			note = "That reminder is gone."
		} else {
			log.Printf("bot: callback %s: %v", action, err)
			note = "Something went wrong."
		}
	}
	if err := b.client.AnswerCallback(ctx, cb.ID, note); err != nil {
		log.Printf("bot: answerCallback: %v", err)
	}
}

func splitCommand(text string) (cmd, rest string) {
	cmd, rest, _ = strings.Cut(text, " ")
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	// Group chats address commands as /list@BotName.
	if at := strings.Index(cmd, "@"); at > 0 {
		cmd = cmd[:at]
	}
	return cmd, strings.TrimSpace(rest)
}

func userLocation(u storage.User) *time.Location {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func formatWhen(r reminder.Reminder, u storage.User) string {
	return storage.FireTime(r).In(userLocation(u)).Format("Mon 2 Jan, 15:04")
}

func nextMorning(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc).AddDate(0, 0, 1)
	return time.Date(local.Year(), local.Month(), local.Day(), 9, 0, 0, 0, loc).UTC()
}

// escape shields user-supplied text from MarkdownV2 parsing.
func escape(s string) string {
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
