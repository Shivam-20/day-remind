package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const maxMailWordings = 30

// MailWording is a saved email and message design owned by one user.
type MailWording struct {
	ID           uuid.UUID `bson:"id"`
	Name         string    `bson:"name"`
	EmailSubject string    `bson:"email_subject,omitempty"`
	EmailHTML    string    `bson:"email_html,omitempty"`
	EmailCSS     string    `bson:"email_css,omitempty"`
	EmailText    string    `bson:"email_text,omitempty"`
	TelegramText string    `bson:"telegram_text,omitempty"`
}

type mailWordingDoc struct {
	MailWordings        []MailWording `bson:"mail_wordings,omitempty"`
	ActiveMailWordingID *uuid.UUID    `bson:"active_mail_wording_id,omitempty"`
}

func (db *DB) loadMailWordings(ctx context.Context, userID uuid.UUID) (mailWordingDoc, error) {
	var doc mailWordingDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return mailWordingDoc{}, ErrNotFound
	}
	if err != nil {
		return mailWordingDoc{}, err
	}
	return doc, nil
}

func normalizeWording(name, subject, html, css, text, telegram string) (MailWording, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MailWording{}, fmt.Errorf("a template name is required")
	}
	if utf8.RuneCountInString(name) > 80 {
		return MailWording{}, fmt.Errorf("template name is too long")
	}
	item := MailWording{
		Name:         name,
		EmailSubject: subject,
		EmailHTML:    html,
		EmailCSS:     css,
		EmailText:    text,
		TelegramText: telegram,
	}
	return item, nil
}

func (db *DB) ListMailWordings(ctx context.Context, userID uuid.UUID) ([]MailWording, *uuid.UUID, error) {
	doc, err := db.loadMailWordings(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if doc.MailWordings == nil {
		doc.MailWordings = []MailWording{}
	}
	return doc.MailWordings, doc.ActiveMailWordingID, nil
}

func (db *DB) MailWordingByID(ctx context.Context, userID, id uuid.UUID) (*MailWording, error) {
	items, _, err := db.ListMailWordings(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			copy := item
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

func (db *DB) ActiveMailWording(ctx context.Context, userID uuid.UUID) (*MailWording, error) {
	items, active, err := db.ListMailWordings(ctx, userID)
	if err != nil || active == nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == *active {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (db *DB) saveMailWordings(ctx context.Context, userID uuid.UUID, items []MailWording, active *uuid.UUID) error {
	set := bson.M{
		"mail_wordings": items,
		"updated_at":    time.Now().UTC(),
	}
	update := bson.M{"$set": set}
	if active == nil {
		update["$unset"] = bson.M{"active_mail_wording_id": ""}
	} else {
		set["active_mail_wording_id"] = *active
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) CreateMailWording(ctx context.Context, userID uuid.UUID, name, subject, html, css, text, telegram string) (MailWording, error) {
	items, active, err := db.ListMailWordings(ctx, userID)
	if err != nil {
		return MailWording{}, err
	}
	if len(items) >= maxMailWordings {
		return MailWording{}, fmt.Errorf("you already have %d mail templates", maxMailWordings)
	}
	item, err := normalizeWording(name, subject, html, css, text, telegram)
	if err != nil {
		return MailWording{}, err
	}
	item.ID = uuid.New()
	items = append(items, item)
	if err := db.saveMailWordings(ctx, userID, items, active); err != nil {
		return MailWording{}, err
	}
	return item, nil
}

func (db *DB) UpdateMailWording(ctx context.Context, userID, id uuid.UUID, name, subject, html, css, text, telegram string) (MailWording, error) {
	items, active, err := db.ListMailWordings(ctx, userID)
	if err != nil {
		return MailWording{}, err
	}
	item, err := normalizeWording(name, subject, html, css, text, telegram)
	if err != nil {
		return MailWording{}, err
	}
	item.ID = id
	found := false
	for i := range items {
		if items[i].ID == id {
			items[i] = item
			found = true
			break
		}
	}
	if !found {
		return MailWording{}, ErrNotFound
	}
	if err := db.saveMailWordings(ctx, userID, items, active); err != nil {
		return MailWording{}, err
	}
	return item, nil
}

func (db *DB) DeleteMailWording(ctx context.Context, userID, id uuid.UUID) error {
	items, active, err := db.ListMailWordings(ctx, userID)
	if err != nil {
		return err
	}
	next := make([]MailWording, 0, len(items))
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return ErrNotFound
	}
	if active != nil && *active == id {
		active = nil
	}
	return db.saveMailWordings(ctx, userID, next, active)
}

func (db *DB) UseMailWording(ctx context.Context, userID, id uuid.UUID) error {
	items, _, err := db.ListMailWordings(ctx, userID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == id {
			return db.saveMailWordings(ctx, userID, items, &id)
		}
	}
	return ErrNotFound
}
