package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrInvalid = errors.New("invalid input")

// emailPattern is deliberately permissive — it rejects the shapes that are
// definitely not addresses rather than trying to implement RFC 5322.
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func validEmail(email string) bool {
	return len(email) <= 254 && emailPattern.MatchString(email)
}

// Roles. An empty role on an existing document is read as RoleUser, so the
// field can be introduced without migrating the collection.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

func normalizeRole(role string) string {
	if role == RoleAdmin {
		return RoleAdmin
	}
	return RoleUser
}

func ValidRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}

// UserTelegram is a user's own bot. It is outbound-only: commands are served
// by the instance bot, because Telegram allows a single getUpdates consumer
// per token and a poller per user would not scale.
type UserTelegram struct {
	Enabled     bool   `bson:"enabled"`
	BotTokenEnc string `bson:"bot_token_enc,omitempty"`
	// OwnChatID is the chat discovered on the user's own bot. A chat id is
	// per-bot, so the id linked through the instance bot cannot be reused here
	// without the user having started their own bot too.
	OwnChatID *int64 `bson:"own_chat_id,omitempty"`
}

func (t UserTelegram) Complete() bool {
	return t.Enabled && t.BotTokenEnc != "" && t.OwnChatID != nil
}

// UserDelivery lets a user route their own reminders through their own mail
// server or bot instead of the instance's.
type UserDelivery struct {
	SMTP     SMTPSettings `bson:"smtp"`
	Telegram UserTelegram `bson:"telegram"`
}

type User struct {
	ID                       uuid.UUID
	Email                    string
	Role                     string
	PasswordHash             string
	DisplayName              string
	Timezone                 string
	Locale                   string
	NotificationsPaused      bool
	NotificationsPausedUntil *time.Time
	Notify                   NotifyPrefs
	TelegramChatID           *int64
	Delivery                 UserDelivery
	Templates                notify.TemplateSet
	MailTemplates            []MailTemplate
	ActiveMailTemplateID     *uuid.UUID
	CreatedAt                time.Time
}

// NotificationsCurrentlyPaused accounts for a vacation window that has expired:
// the flag stays set in the document until someone resumes, but a past
// until-date means delivery should resume on its own.
func (u User) NotificationsCurrentlyPaused(now time.Time) bool {
	if !u.NotificationsPaused {
		return false
	}
	if u.NotificationsPausedUntil != nil && now.After(*u.NotificationsPausedUntil) {
		return false
	}
	return true
}

// NotifyPrefs is a user's delivery configuration. It replaces the unused
// notification_prefs array; documents written before this field existed decode
// to the zero value, which normalizePrefs turns into a sane default.
type NotifyPrefs struct {
	EmailEnabled    bool   `bson:"email_enabled"`
	EmailAddress    string `bson:"email_address"`
	TelegramEnabled bool   `bson:"telegram_enabled"`
	// AdvanceMinutes sends an extra heads-up this long before due_at.
	// Zero means only notify at the due time.
	AdvanceMinutes int `bson:"advance_minutes"`
}

// MaxAdvanceMinutes caps the lead time at four weeks, which also bounds the
// look-ahead window the scheduler has to sweep on every tick.
const MaxAdvanceMinutes = 40320

func normalizePrefs(p NotifyPrefs, fallbackEmail string) NotifyPrefs {
	if p.EmailAddress == "" {
		p.EmailAddress = fallbackEmail
	}
	if p.AdvanceMinutes < 0 {
		p.AdvanceMinutes = 0
	}
	if p.AdvanceMinutes > MaxAdvanceMinutes {
		p.AdvanceMinutes = MaxAdvanceMinutes
	}
	return p
}

type userDoc struct {
	ID                       uuid.UUID          `bson:"_id"`
	Email                    string             `bson:"email"`
	PasswordHash             string             `bson:"password_hash"`
	Role                     string             `bson:"role"`
	DisplayName              string             `bson:"display_name"`
	Timezone                 string             `bson:"timezone"`
	Locale                   string             `bson:"locale"`
	NotificationsPaused      bool               `bson:"notifications_paused"`
	NotificationsPausedUntil *time.Time         `bson:"notifications_paused_until,omitempty"`
	Notify                   NotifyPrefs        `bson:"notify"`
	Delivery                 UserDelivery       `bson:"delivery"`
	Templates                notify.TemplateSet `bson:"templates"`
	MailTemplates            []MailTemplate     `bson:"mail_templates,omitempty"`
	ActiveMailTemplateID     *uuid.UUID         `bson:"active_mail_template_id,omitempty"`
	TelegramChatID           *int64             `bson:"telegram_chat_id,omitempty"`
	TelegramLinkCode         string             `bson:"telegram_link_code,omitempty"`
	TelegramLinkExpiresAt    *time.Time         `bson:"telegram_link_expires_at,omitempty"`
	CreatedAt                time.Time          `bson:"created_at"`
	UpdatedAt                time.Time          `bson:"updated_at"`
}

func userFromDoc(doc userDoc) User {
	return User{
		ID:                       doc.ID,
		Email:                    doc.Email,
		Role:                     normalizeRole(doc.Role),
		PasswordHash:             doc.PasswordHash,
		DisplayName:              doc.DisplayName,
		Timezone:                 doc.Timezone,
		Locale:                   doc.Locale,
		NotificationsPaused:      doc.NotificationsPaused,
		NotificationsPausedUntil: doc.NotificationsPausedUntil,
		Notify:                   normalizePrefs(doc.Notify, doc.Email),
		TelegramChatID:           doc.TelegramChatID,
		Delivery:                 UserDelivery{SMTP: doc.Delivery.SMTP.Normalize(), Telegram: doc.Delivery.Telegram},
		Templates:                doc.Templates,
		MailTemplates:            doc.MailTemplates,
		ActiveMailTemplateID:     doc.ActiveMailTemplateID,
		CreatedAt:                doc.CreatedAt,
	}
}

func (db *DB) CreateUser(ctx context.Context, email, passwordHash, displayName, timezone string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !validEmail(email) {
		return User{}, fmt.Errorf("%w: email is invalid", ErrInvalid)
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = defaultTimezone
	}
	id := uuid.New()
	now := time.Now().UTC()
	doc := userDoc{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         RoleUser,
		DisplayName:  strings.TrimSpace(displayName),
		Timezone:     timezone,
		Locale:       "en",
		Notify: NotifyPrefs{
			EmailEnabled: true,
			EmailAddress: email,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.collection("users").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) GetUserByEmail(ctx context.Context, email string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) SetNotificationsPaused(ctx context.Context, userID uuid.UUID, paused bool, until *time.Time) error {
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"notifications_paused":       paused,
			"notifications_paused_until": until,
			"updated_at":                 time.Now().UTC(),
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserRole changes a user's role. It returns the updated user so callers can
// echo the new state back without a second read.
func (db *DB) SetUserRole(ctx context.Context, userID uuid.UUID, role string) (User, error) {
	if !ValidRole(role) {
		return User{}, fmt.Errorf("%w: unknown role %q", ErrInvalid, role)
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"role": role, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}

// CountAdmins is used to refuse the demotion or deletion that would leave the
// instance with nobody able to reach the admin panel.
func (db *DB) CountAdmins(ctx context.Context) (int64, error) {
	return db.collection("users").CountDocuments(ctx, bson.M{"role": RoleAdmin})
}

// DeleteUser removes the user and everything owned by them. Mongo has no
// foreign keys, so the cascade is explicit; reminders and sessions go first so
// a failure midway cannot leave rows pointing at a user that no longer exists.
func (db *DB) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := db.collection("reminders").DeleteMany(ctx, bson.M{"user_id": userID}); err != nil {
		return fmt.Errorf("delete reminders: %w", err)
	}
	if _, err := db.collection("sessions").DeleteMany(ctx, bson.M{"user_id": userID}); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}
	res, err := db.collection("users").DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UserStats is per-user reminder metadata. It deliberately carries no titles or
// descriptions: the admin panel reports on activity, not on content.
type UserStats struct {
	Total     int64
	Scheduled int64
	Paused    int64
	Done      int64
	Cancelled int64
	Overdue   int64
	NextDueAt *time.Time
}

// AdminUser is one row of the admin user list.
type AdminUser struct {
	User
	Stats UserStats
}

// ListUsersForAdmin returns every user with their reminder stats, using one
// query per collection rather than one per user.
func (db *DB) ListUsersForAdmin(ctx context.Context) ([]AdminUser, error) {
	cur, err := db.collection("users").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var users []User
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		users = append(users, userFromDoc(doc))
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	stats, err := db.reminderStatsByUser(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, AdminUser{User: u, Stats: stats[u.ID]})
	}
	return out, nil
}

// reminderStatsByUser aggregates reminder counts per user in a single pass.
func (db *DB) reminderStatsByUser(ctx context.Context) (map[uuid.UUID]UserStats, error) {
	now := time.Now().UTC()
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "user_id", Value: "$user_id"}, {Key: "status", Value: "$status"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "overdue", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$lt", Value: bson.A{"$due_at", now}}}, 1, 0,
			}}}}}},
			{Key: "next_due", Value: bson.D{{Key: "$min", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$gte", Value: bson.A{"$due_at", now}}}, "$due_at", nil,
			}}}}}},
		}}},
	}
	cur, err := db.collection("reminders").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate reminder stats: %w", err)
	}
	defer cur.Close(ctx)

	type row struct {
		ID struct {
			UserID uuid.UUID `bson:"user_id"`
			Status string    `bson:"status"`
		} `bson:"_id"`
		Count   int64      `bson:"count"`
		Overdue int64      `bson:"overdue"`
		NextDue *time.Time `bson:"next_due"`
	}

	out := map[uuid.UUID]UserStats{}
	for cur.Next(ctx) {
		var r row
		if err := cur.Decode(&r); err != nil {
			return nil, err
		}
		st := out[r.ID.UserID]
		st.Total += r.Count
		switch r.ID.Status {
		case reminder.StatusScheduled:
			st.Scheduled += r.Count
			// Only an open reminder can be meaningfully overdue.
			st.Overdue += r.Overdue
			if r.NextDue != nil && (st.NextDueAt == nil || r.NextDue.Before(*st.NextDueAt)) {
				next := *r.NextDue
				st.NextDueAt = &next
			}
		case reminder.StatusPaused:
			st.Paused += r.Count
		case reminder.StatusDone:
			st.Done += r.Count
		case reminder.StatusCancelled:
			st.Cancelled += r.Count
		}
		out[r.ID.UserID] = st
	}
	return out, cur.Err()
}

// InstanceStats is the headline counts shown at the top of the admin panel.
type InstanceStats struct {
	Users           int64
	Admins          int64
	UsersPaused     int64
	Reminders       int64
	RemindersOpen   int64
	RemindersDone   int64
	ActiveSessions  int64
	RemindersDueNow int64
}

func (db *DB) InstanceStatsFor(ctx context.Context) (InstanceStats, error) {
	var st InstanceStats
	now := time.Now().UTC()
	counts := []struct {
		dst    *int64
		coll   string
		filter bson.M
	}{
		{&st.Users, "users", bson.M{}},
		{&st.Admins, "users", bson.M{"role": RoleAdmin}},
		{&st.UsersPaused, "users", bson.M{"notifications_paused": true}},
		{&st.Reminders, "reminders", bson.M{}},
		{&st.RemindersOpen, "reminders", bson.M{"status": bson.M{"$in": []string{reminder.StatusScheduled, reminder.StatusPaused}}}},
		{&st.RemindersDone, "reminders", bson.M{"status": reminder.StatusDone}},
		{&st.RemindersDueNow, "reminders", bson.M{"status": reminder.StatusScheduled, "due_at": bson.M{"$lt": now}}},
		{&st.ActiveSessions, "sessions", bson.M{"expires_at": bson.M{"$gt": now}}},
	}
	for _, c := range counts {
		n, err := db.collection(c.coll).CountDocuments(ctx, c.filter)
		if err != nil {
			return InstanceStats{}, fmt.Errorf("count %s: %w", c.coll, err)
		}
		*c.dst = n
	}
	return st, nil
}

// SetNotifyPrefs replaces a user's delivery configuration.
func (db *DB) SetNotifyPrefs(ctx context.Context, userID uuid.UUID, p NotifyPrefs) (User, error) {
	p.EmailAddress = strings.TrimSpace(strings.ToLower(p.EmailAddress))
	if p.EmailEnabled && !validEmail(p.EmailAddress) {
		return User{}, fmt.Errorf("%w: notification email is invalid", ErrInvalid)
	}
	if p.AdvanceMinutes < 0 || p.AdvanceMinutes > MaxAdvanceMinutes {
		return User{}, fmt.Errorf("%w: advance_minutes must be between 0 and %d", ErrInvalid, MaxAdvanceMinutes)
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"notify": p, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}

// telegramLinkTTL is how long a /start code stays valid.
const telegramLinkTTL = 15 * time.Minute

// CreateTelegramLinkCode issues a short single-use code the user sends to the
// bot as "/start <code>". Codes are stored on the user and expire, so a leaked
// code cannot be redeemed indefinitely.
func (db *DB) CreateTelegramLinkCode(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	code, err := newLinkCode()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(telegramLinkTTL)
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"telegram_link_code":       code,
			"telegram_link_expires_at": expires,
			"updated_at":               time.Now().UTC(),
		},
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if res.MatchedCount == 0 {
		return "", time.Time{}, ErrNotFound
	}
	return code, expires, nil
}

// RedeemTelegramLinkCode attaches a chat to the user holding this code. The
// code is cleared in the same update, so it cannot be replayed, and the filter
// enforces expiry server-side rather than trusting a prior read.
func (db *DB) RedeemTelegramLinkCode(ctx context.Context, code string, chatID int64) (User, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return User{}, ErrNotFound
	}
	// One chat can only drive one account; drop the link from any other user.
	if _, err := db.collection("users").UpdateMany(ctx,
		bson.M{"telegram_chat_id": chatID},
		bson.M{"$unset": bson.M{"telegram_chat_id": ""}},
	); err != nil {
		return User{}, err
	}

	var doc userDoc
	err := db.collection("users").FindOneAndUpdate(ctx,
		bson.M{
			"telegram_link_code":       code,
			"telegram_link_expires_at": bson.M{"$gt": time.Now().UTC()},
		},
		bson.M{
			"$set":   bson.M{"telegram_chat_id": chatID, "notify.telegram_enabled": true, "updated_at": time.Now().UTC()},
			"$unset": bson.M{"telegram_link_code": "", "telegram_link_expires_at": ""},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

// UnlinkTelegram detaches the chat and stops Telegram delivery.
func (db *DB) UnlinkTelegram(ctx context.Context, userID uuid.UUID) (User, error) {
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set":   bson.M{"notify.telegram_enabled": false, "updated_at": time.Now().UTC()},
		"$unset": bson.M{"telegram_chat_id": "", "telegram_link_code": "", "telegram_link_expires_at": ""},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}

// GetUserByTelegramChat resolves an incoming bot message to an account.
func (db *DB) GetUserByTelegramChat(ctx context.Context, chatID int64) (User, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"telegram_chat_id": chatID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

// ListNotifiableUsers returns users with at least one channel switched on.
// The scheduler loads these once per tick and matches reminders against them
// in memory, rather than issuing a query per user.
func (db *DB) ListNotifiableUsers(ctx context.Context) ([]User, error) {
	cur, err := db.collection("users").Find(ctx, bson.M{
		"$or": []bson.M{
			{"notify.email_enabled": true},
			{"notify.telegram_enabled": true},
		},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []User
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, userFromDoc(doc))
	}
	return out, cur.Err()
}

func newLinkCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("link code: %w", err)
	}
	// Crockford-style alphabet: no vowels, no easily confused characters.
	const alphabet = "0123456789BCDFGHJKLMNPQRSTVWXZ"
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = alphabet[int(c)%len(alphabet)]
	}
	return string(out), nil
}

// SetUserDelivery stores a user's own mail server and bot. Credentials arrive
// already encrypted; this layer never sees plaintext.
func (db *DB) SetUserDelivery(ctx context.Context, userID uuid.UUID, d UserDelivery) (User, error) {
	if err := d.SMTP.Validate(); err != nil {
		return User{}, err
	}
	if d.Telegram.Enabled && d.Telegram.BotTokenEnc == "" {
		return User{}, fmt.Errorf("%w: a bot token is required to use your own bot", ErrInvalid)
	}
	d.SMTP = d.SMTP.Normalize()
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"delivery": d, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}

// SetUserTemplates stores a user's template overrides. Empty fields fall
// through to the instance templates, so clearing a field is how a user reverts
// one part without losing the rest.
func (db *DB) SetUserTemplates(ctx context.Context, userID uuid.UUID, ts notify.TemplateSet) (User, error) {
	if errs := notify.Validate(ts); len(errs) > 0 {
		return User{}, fmt.Errorf("%w: template %s", ErrInvalid, errs[0].Error())
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"templates": ts, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}

// SetUserTelegramOwnChat records the chat discovered on a user's own bot.
func (db *DB) SetUserTelegramOwnChat(ctx context.Context, userID uuid.UUID, chatID int64) (User, error) {
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"delivery.telegram.own_chat_id": chatID, "updated_at": time.Now().UTC()},
	})
	if err != nil {
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}
	return db.GetUserByID(ctx, userID)
}
