// Package repository provides methods to manage tasks from storage(in-memory or postgres)
package repository

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("notification not found")

type Notification struct {
	SendAt    time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string
	// Recipient  string
	// Channel    string
	Text       string
	Status     string
	RetryCount int
	//	LastError  *string
	Delivered bool
}

const (
	StQueued    = "queued"     // set by worker; notification is queued for sending - sendAT <= now & previous status = 'planned'
	StSent      = "sent"       // set by sender; notification is sent to receiver - sendAT <= now
	StCancelled = "cancelled"  // set by service; notification cancelled
	StDead      = "deadqueued" // set by service; notification cancelled
)

type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) error
	GetByID(ctx context.Context, id string) (*Notification, error)
	GetPending(ctx context.Context, until time.Time, limit int) ([]*Notification, error)
	GetAll(ctx context.Context) ([]*Notification, error)
	MarkDelivered(ctx context.Context, id string) error
	IncrementRetry(ctx context.Context, id string, errMsg string) (*Notification, error) // возвращает новый retry_count
	UpdateStatus(ctx context.Context, id string, status string) (*Notification, error)
}
