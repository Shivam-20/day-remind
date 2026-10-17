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
	// SentDueAt and SentAdvanceAt record which notifications have already been
	// dispatched, so a reminder is never delivered twice for the same event.
	SentDueAt     *time.Time
	SentAdvanceAt *time.Time
	Recurrence    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateInput struct {
	Title       string
	Description string
	DueAt       time.Time
	Timezone    string
	Priority    string
	ListID      *uuid.UUID
	Recurrence  string
}

type UpdateInput struct {
	Title       *string
	Description *string
	DueAt       *time.Time
	Timezone    *string
	Priority    *string
	Status      *string
	Pinned      *bool
	ListID      *uuid.UUID
	Recurrence  *string
}

type ListFilter struct {
	UserID uuid.UUID
	Status string
	From   *time.Time
	To     *time.Time
	Query  string
	Limit  int
}

// ValidStatus reports whether s is one of the four reminder states.
func ValidStatus(s string) bool {
	switch s {
	case StatusScheduled, StatusPaused, StatusDone, StatusCancelled:
		return true
	}
	return false
}

// ValidPriority reports whether p is one of the four priority levels.
func ValidPriority(p string) bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}
