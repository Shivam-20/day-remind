package storage

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GoogleIntegration struct {
	UserID            uuid.UUID
	Email             string
	RefreshToken      string
	CalendarEnabled   bool
	GmailEnabled      bool
	CalendarPaused    bool
	GmailPaused       bool
	GmailLabelIDs     []string
	LastCalendarSync  *time.Time
	LastGmailSync     *time.Time
	UpdatedAt         time.Time
}

type googleIntegrationDoc struct {
	UserID            uuid.UUID  `bson:"_id"`
	Email             string     `bson:"email"`
	RefreshToken      string     `bson:"refresh_token"`
	CalendarEnabled   bool       `bson:"calendar_enabled"`
	GmailEnabled      bool       `bson:"gmail_enabled"`
	CalendarPaused    bool       `bson:"calendar_paused"`
	GmailPaused       bool       `bson:"gmail_paused"`
	GmailLabelIDs     []string   `bson:"gmail_label_ids"`
	LastCalendarSync  *time.Time `bson:"last_calendar_sync,omitempty"`
	LastGmailSync     *time.Time `bson:"last_gmail_sync,omitempty"`
	UpdatedAt         time.Time  `bson:"updated_at"`
}

func googleFromDoc(doc googleIntegrationDoc) GoogleIntegration {
	return GoogleIntegration{
		UserID:           doc.UserID,
		Email:            doc.Email,
		RefreshToken:     doc.RefreshToken,
		CalendarEnabled:  doc.CalendarEnabled,
		GmailEnabled:     doc.GmailEnabled,
		CalendarPaused:   doc.CalendarPaused,
		GmailPaused:      doc.GmailPaused,
		GmailLabelIDs:    doc.GmailLabelIDs,
		LastCalendarSync: doc.LastCalendarSync,
		LastGmailSync:    doc.LastGmailSync,
		UpdatedAt:        doc.UpdatedAt,
	}
}

func (db *DB) GetGoogleIntegration(ctx context.Context, userID uuid.UUID) (GoogleIntegration, error) {
	var doc googleIntegrationDoc
	err := db.collection("google_integrations").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return GoogleIntegration{}, ErrNotFound
	}
	if err != nil {
		return GoogleIntegration{}, err
	}
	return googleFromDoc(doc), nil
}

func (db *DB) SaveGoogleIntegration(ctx context.Context, in GoogleIntegration) (GoogleIntegration, error) {
	now := time.Now().UTC()
	doc := googleIntegrationDoc{
		UserID:           in.UserID,
		Email:            strings.TrimSpace(in.Email),
		RefreshToken:     in.RefreshToken,
		CalendarEnabled:  in.CalendarEnabled,
		GmailEnabled:     in.GmailEnabled,
		CalendarPaused:   in.CalendarPaused,
		GmailPaused:      in.GmailPaused,
		GmailLabelIDs:    in.GmailLabelIDs,
		LastCalendarSync: in.LastCalendarSync,
		LastGmailSync:    in.LastGmailSync,
		UpdatedAt:        now,
	}
	_, err := db.collection("google_integrations").UpdateOne(ctx, bson.M{"_id": in.UserID},
		bson.M{"$set": doc}, options.Update().SetUpsert(true))
	if err != nil {
		return GoogleIntegration{}, err
	}
	return googleFromDoc(doc), nil
}

func (db *DB) DeleteGoogleIntegration(ctx context.Context, userID uuid.UUID) error {
	res, err := db.collection("google_integrations").DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) UpdateGoogleCalendarSync(ctx context.Context, userID uuid.UUID, lastSync time.Time) error {
	res, err := db.collection("google_integrations").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"last_calendar_sync": lastSync.UTC(),
			"updated_at":         time.Now().UTC(),
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

func (db *DB) UpdateGoogleGmailSync(ctx context.Context, userID uuid.UUID, lastSync time.Time) error {
	res, err := db.collection("google_integrations").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"last_gmail_sync": lastSync.UTC(),
			"updated_at":    time.Now().UTC(),
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
