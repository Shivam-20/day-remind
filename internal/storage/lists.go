package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type List struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Color     string
	SortOrder int
	Paused    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ListCreateInput struct {
	Name      string
	Color     string
	SortOrder int
}

type ListUpdateInput struct {
	Name      *string
	Color     *string
	SortOrder *int
}

type listDoc struct {
	ID        uuid.UUID `bson:"_id"`
	UserID    uuid.UUID `bson:"user_id"`
	Name      string    `bson:"name"`
	Color     string    `bson:"color"`
	SortOrder int       `bson:"sort_order"`
	Paused    bool      `bson:"paused"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func listFromDoc(doc listDoc) List {
	return List{
		ID:        doc.ID,
		UserID:    doc.UserID,
		Name:      doc.Name,
		Color:     doc.Color,
		SortOrder: doc.SortOrder,
		Paused:    doc.Paused,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}

func (db *DB) CreateList(ctx context.Context, userID uuid.UUID, in ListCreateInput) (List, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return List{}, fmt.Errorf("name is required")
	}
	color := strings.TrimSpace(in.Color)
	if color == "" {
		color = "#3b82f6"
	}
	now := time.Now().UTC()
	doc := listDoc{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		Color:     color,
		SortOrder: in.SortOrder,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.collection("lists").InsertOne(ctx, doc)
	if err != nil {
		return List{}, err
	}
	return listFromDoc(doc), nil
}

func (db *DB) GetListByID(ctx context.Context, userID, id uuid.UUID) (List, error) {
	var doc listDoc
	err := db.collection("lists").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return List{}, ErrNotFound
	}
	if err != nil {
		return List{}, err
	}
	return listFromDoc(doc), nil
}

func (db *DB) ListLists(ctx context.Context, userID uuid.UUID) ([]List, error) {
	opts := options.Find().SetSort(bson.D{{Key: "sort_order", Value: 1}, {Key: "name", Value: 1}})
	cur, err := db.collection("lists").Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []List
	for cur.Next(ctx) {
		var doc listDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, listFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) UpdateList(ctx context.Context, userID, id uuid.UUID, in ListUpdateInput) (List, error) {
	cur, err := db.GetListByID(ctx, userID, id)
	if err != nil {
		return List{}, err
	}
	name := cur.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return List{}, fmt.Errorf("name is required")
		}
	}
	color := cur.Color
	if in.Color != nil {
		color = strings.TrimSpace(*in.Color)
	}
	sortOrder := cur.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	set := bson.M{
		"name":       name,
		"color":      color,
		"sort_order": sortOrder,
		"updated_at": time.Now().UTC(),
	}
	res, err := db.collection("lists").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return List{}, err
	}
	if res.MatchedCount == 0 {
		return List{}, ErrNotFound
	}
	return db.GetListByID(ctx, userID, id)
}

func (db *DB) DeleteList(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("lists").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) PauseList(ctx context.Context, userID, id uuid.UUID) (List, error) {
	now := time.Now().UTC()
	res, err := db.collection("lists").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{
		"$set": bson.M{
			"paused":     true,
			"updated_at": now,
		},
	})
	if err != nil {
		return List{}, err
	}
	if res.MatchedCount == 0 {
		return List{}, ErrNotFound
	}
	return db.GetListByID(ctx, userID, id)
}

func (db *DB) ResumeList(ctx context.Context, userID, id uuid.UUID) (List, error) {
	now := time.Now().UTC()
	res, err := db.collection("lists").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{
		"$set": bson.M{
			"paused":     false,
			"updated_at": now,
		},
	})
	if err != nil {
		return List{}, err
	}
	if res.MatchedCount == 0 {
		return List{}, ErrNotFound
	}
	return db.GetListByID(ctx, userID, id)
}

// CountRemindersInList reports how many reminders reference the list, so the
// UI can warn before deleting one that is still in use.
func (db *DB) CountRemindersInList(ctx context.Context, userID, listID uuid.UUID) (int64, error) {
	return db.collection("reminders").CountDocuments(ctx, bson.M{"user_id": userID, "list_id": listID})
}
