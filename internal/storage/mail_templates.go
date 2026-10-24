package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"go.mongodb.org/mongo-driver/bson"
)

const maxMailTemplates = 30
const maxMailTemplateName = 80

// MailTemplate is one saved email and message design. Using it copies Set onto
// the active wording the scheduler already renders.
type MailTemplate struct {
	ID   uuid.UUID          `bson:"id"`
	Name string             `bson:"name"`
	Set  notify.TemplateSet `bson:"set"`
}

func normalizeMailTemplate(name string, set notify.TemplateSet) (MailTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MailTemplate{}, fmt.Errorf("%w: a template name is required", ErrInvalid)
	}
	if utf8.RuneCountInString(name) > maxMailTemplateName {
		return MailTemplate{}, fmt.Errorf("%w: template name is too long", ErrInvalid)
	}
	if errs := notify.Validate(set); len(errs) > 0 {
		return MailTemplate{}, fmt.Errorf("%w: template %s", ErrInvalid, errs[0].Error())
	}
	return MailTemplate{Name: name, Set: set}, nil
}

func findMailTemplate(items []MailTemplate, id uuid.UUID) (MailTemplate, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return MailTemplate{}, false
}

func (db *DB) CreateUserMailTemplate(ctx context.Context, userID uuid.UUID, name string, set notify.TemplateSet) (MailTemplate, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return MailTemplate{}, err
	}
	item, err := normalizeMailTemplate(name, set)
	if err != nil {
		return MailTemplate{}, err
	}
	if len(user.MailTemplates) >= maxMailTemplates {
		return MailTemplate{}, fmt.Errorf("%w: you already have %d mail templates", ErrInvalid, maxMailTemplates)
	}
	item.ID = uuid.New()
	user.MailTemplates = append(user.MailTemplates, item)
	if err := db.saveUserMailLibrary(ctx, user); err != nil {
		return MailTemplate{}, err
	}
	return item, nil
}

func (db *DB) UpdateUserMailTemplate(ctx context.Context, userID, id uuid.UUID, name string, set notify.TemplateSet) (MailTemplate, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return MailTemplate{}, err
	}
	item, err := normalizeMailTemplate(name, set)
	if err != nil {
		return MailTemplate{}, err
	}
	item.ID = id
	found := false
	for i := range user.MailTemplates {
		if user.MailTemplates[i].ID == id {
			user.MailTemplates[i] = item
			found = true
			break
		}
	}
	if !found {
		return MailTemplate{}, ErrNotFound
	}
	if user.ActiveMailTemplateID != nil && *user.ActiveMailTemplateID == id {
		user.Templates = item.Set
	}
	if err := db.saveUserMailLibrary(ctx, user); err != nil {
		return MailTemplate{}, err
	}
	return item, nil
}

func (db *DB) DeleteUserMailTemplate(ctx context.Context, userID, id uuid.UUID) error {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	next := user.MailTemplates[:0]
	found := false
	for _, item := range user.MailTemplates {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return ErrNotFound
	}
	user.MailTemplates = next
	if user.ActiveMailTemplateID != nil && *user.ActiveMailTemplateID == id {
		user.ActiveMailTemplateID = nil
	}
	return db.saveUserMailLibrary(ctx, user)
}

// UseUserMailTemplate copies a saved design onto the wording delivery uses.
func (db *DB) UseUserMailTemplate(ctx context.Context, userID, id uuid.UUID) (User, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	item, ok := findMailTemplate(user.MailTemplates, id)
	if !ok {
		return User{}, ErrNotFound
	}
	user.Templates = item.Set
	user.ActiveMailTemplateID = &item.ID
	if err := db.saveUserMailLibrary(ctx, user); err != nil {
		return User{}, err
	}
	return db.GetUserByID(ctx, userID)
}

func (db *DB) saveUserMailLibrary(ctx context.Context, user User) error {
	set := bson.M{
		"mail_templates": user.MailTemplates,
		"templates":      user.Templates,
		"updated_at":     time.Now().UTC(),
	}
	unset := bson.M{}
	if user.ActiveMailTemplateID == nil {
		unset["active_mail_template_id"] = ""
	} else {
		set["active_mail_template_id"] = *user.ActiveMailTemplateID
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": user.ID}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) CreateInstanceMailTemplate(ctx context.Context, actor uuid.UUID, name string, set notify.TemplateSet) (MailTemplate, error) {
	inst, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return MailTemplate{}, err
	}
	item, err := normalizeMailTemplate(name, set)
	if err != nil {
		return MailTemplate{}, err
	}
	if len(inst.MailTemplates) >= maxMailTemplates {
		return MailTemplate{}, fmt.Errorf("%w: the instance already has %d mail templates", ErrInvalid, maxMailTemplates)
	}
	item.ID = uuid.New()
	inst.MailTemplates = append(inst.MailTemplates, item)
	if err := db.SaveInstanceSettings(ctx, inst, actor); err != nil {
		return MailTemplate{}, err
	}
	return item, nil
}

func (db *DB) UpdateInstanceMailTemplate(ctx context.Context, actor, id uuid.UUID, name string, set notify.TemplateSet) (MailTemplate, error) {
	inst, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return MailTemplate{}, err
	}
	item, err := normalizeMailTemplate(name, set)
	if err != nil {
		return MailTemplate{}, err
	}
	item.ID = id
	found := false
	for i := range inst.MailTemplates {
		if inst.MailTemplates[i].ID == id {
			inst.MailTemplates[i] = item
			found = true
			break
		}
	}
	if !found {
		return MailTemplate{}, ErrNotFound
	}
	if inst.ActiveMailTemplateID != nil && *inst.ActiveMailTemplateID == id {
		inst.Templates = item.Set
	}
	if err := db.SaveInstanceSettings(ctx, inst, actor); err != nil {
		return MailTemplate{}, err
	}
	return item, nil
}

func (db *DB) DeleteInstanceMailTemplate(ctx context.Context, actor, id uuid.UUID) error {
	inst, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return err
	}
	next := inst.MailTemplates[:0]
	found := false
	for _, item := range inst.MailTemplates {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return ErrNotFound
	}
	inst.MailTemplates = next
	if inst.ActiveMailTemplateID != nil && *inst.ActiveMailTemplateID == id {
		inst.ActiveMailTemplateID = nil
	}
	return db.SaveInstanceSettings(ctx, inst, actor)
}

func (db *DB) UseInstanceMailTemplate(ctx context.Context, actor, id uuid.UUID) (InstanceSettings, error) {
	inst, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return InstanceSettings{}, err
	}
	item, ok := findMailTemplate(inst.MailTemplates, id)
	if !ok {
		return InstanceSettings{}, ErrNotFound
	}
	inst.Templates = item.Set
	inst.ActiveMailTemplateID = &item.ID
	if err := db.SaveInstanceSettings(ctx, inst, actor); err != nil {
		return InstanceSettings{}, err
	}
	return db.GetInstanceSettings(ctx)
}
