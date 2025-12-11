// Package cache provides checking requested info from handlers firstly
// from cache, and if there isn't any - forwards request to DB-layer
package cache

import (
	"context"

	"github.com/UnendingLoop/delayed-notifier/internal/repository"
)

type StatusCache interface {
	SetByUID(ctx context.Context, uid string, pointer *repository.Notification) error
	GetByUID(ctx context.Context, uid string) (*repository.Notification, error)
	DeleteByUID(ctx context.Context, uid string) error
}
