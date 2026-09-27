package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const aiSettingsID = "ai"

type AISettings struct {
	Provider          string
	OpenRouterAPIKey  string
	OpenRouterModel   string
	GeminiAPIKey      string
	GeminiModel       string
	UpdatedAt         time.Time
}

type AISettingsUpdate struct {
	Provider         *string
	OpenRouterAPIKey *string
	OpenRouterModel  *string
	GeminiAPIKey     *string
	GeminiModel      *string
}

type aiSettingsDoc struct {
	ID               string    `bson:"_id"`
	Provider         string    `bson:"provider,omitempty"`
	OpenRouterAPIKey string    `bson:"openrouter_api_key,omitempty"`
	OpenRouterModel  string    `bson:"openrouter_model,omitempty"`
	GeminiAPIKey     string    `bson:"gemini_api_key,omitempty"`
	GeminiModel      string    `bson:"gemini_model,omitempty"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

func (db *DB) GetAISettings(ctx context.Context) (AISettings, error) {
	var doc aiSettingsDoc
	err := db.collection("system_settings").FindOne(ctx, bson.M{"_id": aiSettingsID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return AISettings{}, nil
	}
	if err != nil {
		return AISettings{}, err
	}
	return aiSettingsFromDoc(doc), nil
}

func (db *DB) UpdateAISettings(ctx context.Context, in AISettingsUpdate) (AISettings, error) {
	cur, err := db.GetAISettings(ctx)
	if err != nil {
		return AISettings{}, err
	}
	if in.Provider != nil {
		provider := strings.ToLower(strings.TrimSpace(*in.Provider))
		switch provider {
		case "openrouter", "gemini", "":
			cur.Provider = provider
		default:
			return AISettings{}, fmt.Errorf("ai provider must be openrouter or gemini")
		}
	}
	if in.OpenRouterAPIKey != nil {
		key := strings.TrimSpace(*in.OpenRouterAPIKey)
		if key != "" {
			cur.OpenRouterAPIKey = key
		}
	}
	if in.OpenRouterModel != nil {
		cur.OpenRouterModel = strings.TrimSpace(*in.OpenRouterModel)
	}
	if in.GeminiAPIKey != nil {
		key := strings.TrimSpace(*in.GeminiAPIKey)
		if key != "" {
			cur.GeminiAPIKey = key
		}
	}
	if in.GeminiModel != nil {
		cur.GeminiModel = strings.TrimSpace(*in.GeminiModel)
	}
	cur.UpdatedAt = time.Now().UTC()
	doc := aiSettingsDoc{
		ID:               aiSettingsID,
		Provider:         cur.Provider,
		OpenRouterAPIKey: cur.OpenRouterAPIKey,
		OpenRouterModel:  cur.OpenRouterModel,
		GeminiAPIKey:     cur.GeminiAPIKey,
		GeminiModel:      cur.GeminiModel,
		UpdatedAt:        cur.UpdatedAt,
	}
	opts := options.Update().SetUpsert(true)
	_, err = db.collection("system_settings").UpdateOne(ctx, bson.M{"_id": aiSettingsID}, bson.M{"$set": doc}, opts)
	if err != nil {
		return AISettings{}, err
	}
	return cur, nil
}

func aiSettingsFromDoc(doc aiSettingsDoc) AISettings {
	return AISettings{
		Provider:         doc.Provider,
		OpenRouterAPIKey: doc.OpenRouterAPIKey,
		OpenRouterModel:  doc.OpenRouterModel,
		GeminiAPIKey:     doc.GeminiAPIKey,
		GeminiModel:      doc.GeminiModel,
		UpdatedAt:        doc.UpdatedAt,
	}
}
