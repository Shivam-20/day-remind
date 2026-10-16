package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// instanceSettingsID is the fixed key of the singleton settings document.
const instanceSettingsID = "instance"

// TLS transport modes for SMTP.
const (
	TLSStartTLS = "starttls"
	TLSImplicit = "implicit"
	TLSNone     = "none"
)

// SMTPSettings describes one mail server. The same shape is used for the
// instance-wide configuration and for a user's own override.
//
// PasswordEnc holds ciphertext produced by internal/secret and must never be
// serialized to an API response.
type SMTPSettings struct {
	Enabled     bool   `bson:"enabled"`
	Host        string `bson:"host,omitempty"`
	Port        int    `bson:"port,omitempty"`
	Username    string `bson:"username,omitempty"`
	PasswordEnc string `bson:"password_enc,omitempty"`
	From        string `bson:"from,omitempty"`
	TLS         string `bson:"tls,omitempty"`
}

// Complete reports whether enough is filled in to attempt a delivery. Enabled
// alone is not sufficient: a half-filled form would fail at send time.
func (s SMTPSettings) Complete() bool {
	return s.Enabled && strings.TrimSpace(s.Host) != "" && strings.TrimSpace(s.From) != ""
}

// Normalize applies defaults and trims input so stored values are predictable.
func (s SMTPSettings) Normalize() SMTPSettings {
	s.Host = strings.TrimSpace(s.Host)
	s.From = strings.TrimSpace(s.From)
	s.Username = strings.TrimSpace(s.Username)
	s.TLS = strings.ToLower(strings.TrimSpace(s.TLS))
	if s.TLS == "" {
		s.TLS = TLSStartTLS
	}
	if s.Port == 0 {
		s.Port = 587
	}
	return s
}

// Validate checks a mail server definition before it is stored.
func (s SMTPSettings) Validate() error {
	s = s.Normalize()
	if !s.Enabled {
		return nil
	}
	if s.Host == "" {
		return fmt.Errorf("%w: SMTP host is required", ErrInvalid)
	}
	if s.From == "" {
		return fmt.Errorf("%w: a from address is required", ErrInvalid)
	}
	if !validEmail(strings.ToLower(s.From)) {
		return fmt.Errorf("%w: the from address is not a valid email address", ErrInvalid)
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalid)
	}
	switch s.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return fmt.Errorf("%w: TLS mode must be starttls, implicit or none", ErrInvalid)
	}
	return nil
}

// TelegramSettings is the instance bot. The token is ciphertext.
type TelegramSettings struct {
	Enabled     bool   `bson:"enabled"`
	BotTokenEnc string `bson:"bot_token_enc,omitempty"`
}

func (t TelegramSettings) Complete() bool {
	return t.Enabled && t.BotTokenEnc != ""
}

// InstanceSettings is the singleton configuration an admin edits. It is the
// authoritative source once written; environment variables only seed it.
type InstanceSettings struct {
	SMTP                 SMTPSettings
	Telegram             TelegramSettings
	Templates            notify.TemplateSet
	MailTemplates        []MailTemplate
	ActiveMailTemplateID *uuid.UUID
	UpdatedAt            time.Time
	UpdatedBy            uuid.UUID
}

type instanceSettingsDoc struct {
	ID                   string             `bson:"_id"`
	SMTP                 SMTPSettings       `bson:"smtp"`
	Telegram             TelegramSettings   `bson:"telegram"`
	Templates            notify.TemplateSet `bson:"templates"`
	MailTemplates        []MailTemplate     `bson:"mail_templates,omitempty"`
	ActiveMailTemplateID *uuid.UUID         `bson:"active_mail_template_id,omitempty"`
	UpdatedAt            time.Time          `bson:"updated_at"`
	UpdatedBy            uuid.UUID          `bson:"updated_by,omitempty"`
}

func instanceFromDoc(d instanceSettingsDoc) InstanceSettings {
	return InstanceSettings{
		SMTP:                 d.SMTP.Normalize(),
		Telegram:             d.Telegram,
		Templates:            d.Templates,
		MailTemplates:        d.MailTemplates,
		ActiveMailTemplateID: d.ActiveMailTemplateID,
		UpdatedAt:            d.UpdatedAt,
		UpdatedBy:            d.UpdatedBy,
	}
}

// GetInstanceSettings returns the current configuration. A missing document is
// not an error: a fresh install simply has nothing configured yet.
func (db *DB) GetInstanceSettings(ctx context.Context) (InstanceSettings, error) {
	var doc instanceSettingsDoc
	err := db.collection("settings").FindOne(ctx, bson.M{"_id": instanceSettingsID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return InstanceSettings{}, nil
	}
	if err != nil {
		return InstanceSettings{}, err
	}
	return instanceFromDoc(doc), nil
}

// SaveInstanceSettings replaces the configuration, recording who changed it.
func (db *DB) SaveInstanceSettings(ctx context.Context, s InstanceSettings, actor uuid.UUID) error {
	if err := s.SMTP.Validate(); err != nil {
		return err
	}
	if errs := notify.Validate(s.Templates); len(errs) > 0 {
		return fmt.Errorf("%w: template %s", ErrInvalid, errs[0].Error())
	}
	doc := instanceSettingsDoc{
		ID:                   instanceSettingsID,
		SMTP:                 s.SMTP.Normalize(),
		Telegram:             s.Telegram,
		Templates:            s.Templates,
		MailTemplates:        s.MailTemplates,
		ActiveMailTemplateID: s.ActiveMailTemplateID,
		UpdatedAt:            time.Now().UTC(),
		UpdatedBy:            actor,
	}
	_, err := db.collection("settings").ReplaceOne(ctx,
		bson.M{"_id": instanceSettingsID}, doc, options.Replace().SetUpsert(true))
	return err
}

// SeedInstanceSettings writes the configuration only when none exists yet, so
// environment variables bootstrap a fresh install and are then ignored. It
// reports whether it actually wrote anything.
func (db *DB) SeedInstanceSettings(ctx context.Context, s InstanceSettings) (bool, error) {
	doc := instanceSettingsDoc{
		ID:        instanceSettingsID,
		SMTP:      s.SMTP.Normalize(),
		Telegram:  s.Telegram,
		Templates: s.Templates,
		UpdatedAt: time.Now().UTC(),
	}
	// InsertOne rather than an upsert: a duplicate key is exactly the "already
	// configured, leave it alone" case, and it is decided by the database
	// rather than by a read followed by a racy write.
	_, err := db.collection("settings").InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
