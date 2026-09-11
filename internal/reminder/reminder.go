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
	ID           uuid.UUID
	UserID       uuid.UUID
	ListID       *uuid.UUID
	Title        string
	Description  string
	DueAt        time.Time
	Timezone     string
	Status       string
	Priority     string
	Pinned       bool
	SnoozedUntil *time.Time
	PausedAt     *time.Time
	ResumeAt     *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateInput struct {
	Title       string
	Description string
	DueAt       time.Time
	Timezone    string
	Priority    string
	ListID      *uuid.UUID
}

type UpdateInput struct {
	Title       *string
	Description *string
	DueAt       *time.Time
	Timezone    *string
	Priority    *string
	Status      *string
	ListID      *uuid.UUID
}

type ListFilter struct {
	UserID uuid.UUID
	Status string
	From   *time.Time
	To     *time.Time
	Query  string
	Limit  int
}
