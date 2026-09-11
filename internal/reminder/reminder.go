package reminder

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusScheduled = "scheduled"
	StatusPaused    = "paused"
	StatusDone      = "done"
	StatusCancelled = "cancelled"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

type Reminder struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	ListID             *uuid.UUID
	Title              string
	Description        string
	DueAt              time.Time
	Timezone           string
	Status             string
	Priority           string
	Pinned             bool
	TagIDs             []uuid.UUID
	RecurrenceRule     string
	SeriesID           *uuid.UUID
	RecurrenceUntil    *time.Time
	ChannelOverrides   map[string]bool
	RequireAllSubtasks bool
	AttachmentURL      string
	AssigneeID         *uuid.UUID
	GroupID            *uuid.UUID
	SnoozedUntil       *time.Time
	PausedAt           *time.Time
	ResumeAt           *time.Time
	CompletedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateInput struct {
	Title              string
	Description        string
	DueAt              time.Time
	Timezone           string
	Priority           string
	ListID             *uuid.UUID
	TagIDs             []uuid.UUID
	RecurrenceRule     string
	SeriesID           *uuid.UUID
	RecurrenceUntil    *time.Time
	ChannelOverrides   map[string]bool
	RequireAllSubtasks bool
	AttachmentURL      string
	AssigneeID         *uuid.UUID
	GroupID            *uuid.UUID
}

type UpdateInput struct {
	Title              *string
	Description        *string
	DueAt              *time.Time
	Timezone           *string
	Priority           *string
	Status             *string
	ListID             *uuid.UUID
	TagIDs             *[]uuid.UUID
	RecurrenceRule     *string
	SeriesID           *uuid.UUID
	RecurrenceUntil    *time.Time
	ChannelOverrides   *map[string]bool
	RequireAllSubtasks *bool
	AttachmentURL      *string
	AssigneeID         *uuid.UUID
	GroupID            *uuid.UUID
}

type ListFilter struct {
	UserID   uuid.UUID
	Status   string
	From     *time.Time
	To       *time.Time
	Query    string
	ListID   *uuid.UUID
	Priority string
	TagID    *uuid.UUID
	Limit    int
}
