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

const (
	GroupRoleOwner  = "owner"
	GroupRoleMember = "member"
)

type Group struct {
	ID      uuid.UUID
	OwnerID uuid.UUID
	Name    string
}

type GroupMember struct {
	GroupID uuid.UUID
	UserID  *uuid.UUID
	Email   string
	Role    string
}

type groupDoc struct {
	ID        uuid.UUID `bson:"_id"`
	OwnerID   uuid.UUID `bson:"owner_id"`
	Name      string    `bson:"name"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type groupMemberDoc struct {
	ID        uuid.UUID  `bson:"_id"`
	GroupID   uuid.UUID  `bson:"group_id"`
	UserID    *uuid.UUID `bson:"user_id,omitempty"`
	Email     string     `bson:"email"`
	Role      string     `bson:"role"`
	CreatedAt time.Time  `bson:"created_at"`
}

func groupFromDoc(doc groupDoc) Group {
	return Group{
		ID:      doc.ID,
		OwnerID: doc.OwnerID,
		Name:    doc.Name,
	}
}

func memberFromDoc(doc groupMemberDoc) GroupMember {
	return GroupMember{
		GroupID: doc.GroupID,
		UserID:  doc.UserID,
		Email:   doc.Email,
		Role:    doc.Role,
	}
}

func (db *DB) CreateGroup(ctx context.Context, ownerID uuid.UUID, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, fmt.Errorf("name is required")
	}
	now := time.Now().UTC()
	groupID := uuid.New()
	doc := groupDoc{
		ID:        groupID,
		OwnerID:   ownerID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.collection("groups").InsertOne(ctx, doc)
	if err != nil {
		return Group{}, err
	}
	owner, err := db.GetUserByID(ctx, ownerID)
	if err != nil {
		return Group{}, err
	}
	member := groupMemberDoc{
		ID:        uuid.New(),
		GroupID:   groupID,
		UserID:    &ownerID,
		Email:     owner.Email,
		Role:      GroupRoleOwner,
		CreatedAt: now,
	}
	_, err = db.collection("group_members").InsertOne(ctx, member)
	if err != nil {
		return Group{}, err
	}
	return groupFromDoc(doc), nil
}

func (db *DB) GetGroup(ctx context.Context, userID, groupID uuid.UUID) (Group, error) {
	if _, err := db.getGroupMembership(ctx, groupID, userID); err != nil {
		return Group{}, err
	}
	var doc groupDoc
	err := db.collection("groups").FindOne(ctx, bson.M{"_id": groupID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Group{}, ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	return groupFromDoc(doc), nil
}

func (db *DB) ListGroups(ctx context.Context, userID uuid.UUID) ([]Group, error) {
	cur, err := db.collection("group_members").Find(ctx, bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"email": db.userEmail(ctx, userID)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	groupIDs := make([]uuid.UUID, 0)
	for cur.Next(ctx) {
		var m groupMemberDoc
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, m.GroupID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	if len(groupIDs) == 0 {
		return nil, nil
	}

	gcur, err := db.collection("groups").Find(ctx, bson.M{"_id": bson.M{"$in": groupIDs}},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer gcur.Close(ctx)

	var out []Group
	for gcur.Next(ctx) {
		var doc groupDoc
		if err := gcur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, groupFromDoc(doc))
	}
	return out, gcur.Err()
}

func (db *DB) UpdateGroup(ctx context.Context, ownerID, groupID uuid.UUID, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, fmt.Errorf("name is required")
	}
	res, err := db.collection("groups").UpdateOne(ctx, bson.M{
		"_id":      groupID,
		"owner_id": ownerID,
	}, bson.M{"$set": bson.M{
		"name":       name,
		"updated_at": time.Now().UTC(),
	}})
	if err != nil {
		return Group{}, err
	}
	if res.MatchedCount == 0 {
		return Group{}, ErrNotFound
	}
	var doc groupDoc
	err = db.collection("groups").FindOne(ctx, bson.M{"_id": groupID}).Decode(&doc)
	if err != nil {
		return Group{}, err
	}
	return groupFromDoc(doc), nil
}

func (db *DB) DeleteGroup(ctx context.Context, ownerID, groupID uuid.UUID) error {
	res, err := db.collection("groups").DeleteOne(ctx, bson.M{
		"_id":      groupID,
		"owner_id": ownerID,
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	_, err = db.collection("group_members").DeleteMany(ctx, bson.M{"group_id": groupID})
	return err
}

func (db *DB) InviteGroupMember(ctx context.Context, ownerID, groupID uuid.UUID, email, role string) (GroupMember, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return GroupMember{}, fmt.Errorf("email is required")
	}
	if role == "" {
		role = GroupRoleMember
	}
	if role != GroupRoleOwner && role != GroupRoleMember {
		return GroupMember{}, fmt.Errorf("invalid role")
	}
	group, err := db.GetGroup(ctx, ownerID, groupID)
	if err != nil {
		return GroupMember{}, err
	}
	if group.OwnerID != ownerID {
		return GroupMember{}, ErrNotFound
	}
	var userID *uuid.UUID
	u, err := db.GetUserByEmail(ctx, email)
	if err == nil {
		userID = &u.ID
	} else if err != ErrNotFound {
		return GroupMember{}, err
	}
	doc := groupMemberDoc{
		ID:        uuid.New(),
		GroupID:   groupID,
		UserID:    userID,
		Email:     email,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	_, err = db.collection("group_members").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return GroupMember{}, ErrConflict
		}
		return GroupMember{}, err
	}
	return memberFromDoc(doc), nil
}

func (db *DB) ListGroupMembers(ctx context.Context, userID, groupID uuid.UUID) ([]GroupMember, error) {
	if _, err := db.getGroupMembership(ctx, groupID, userID); err != nil {
		return nil, err
	}
	cur, err := db.collection("group_members").Find(ctx, bson.M{"group_id": groupID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []GroupMember
	for cur.Next(ctx) {
		var doc groupMemberDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, memberFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) getGroupMembership(ctx context.Context, groupID, userID uuid.UUID) (groupMemberDoc, error) {
	email := db.userEmail(ctx, userID)
	filter := bson.M{
		"group_id": groupID,
		"$or": []bson.M{
			{"user_id": userID},
			{"email": email},
		},
	}
	var doc groupMemberDoc
	err := db.collection("group_members").FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return groupMemberDoc{}, ErrNotFound
	}
	if err != nil {
		return groupMemberDoc{}, err
	}
	return doc, nil
}

func (db *DB) userEmail(ctx context.Context, userID uuid.UUID) string {
	u, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return ""
	}
	return u.Email
}
