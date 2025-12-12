// Package service provides business logic and access to cache(Redis) and storage(map or pgql)
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/UnendingLoop/delayed-notifier/internal/cache"
	"github.com/UnendingLoop/delayed-notifier/internal/repository"
	"github.com/wb-go/wbf/rabbitmq"
)

type NotificationService struct {
	repo  repository.NotificationRepository
	cache cache.StatusCache
	q     *rabbitmq.Publisher
}

var ( // user-depending errors
	ErrIncorrectTime    = errors.New("incorrect time provided")
	ErrEmptyDescription = errors.New("no text/description provided for notification")
	ErrEmptyID          = errors.New("empty id is provided")
)

func NewNotificationService(repo repository.NotificationRepository, cache cache.StatusCache, q *rabbitmq.Publisher) *NotificationService {
	return &NotificationService{
		repo:  repo,
		cache: cache,
		q:     q,
	}
}

// Create новую задачу
func (s *NotificationService) Create(reqCTX context.Context, text string, sendAt time.Time) (*repository.Notification, error) {
	if sendAt.IsZero() || time.Now().UTC().After(sendAt.UTC()) { // не пропускаем время, если оно имеет нулевое значение, или если оно в прошлом
		return nil, ErrIncorrectTime
	}
	if len(text) == 0 {
		return nil, ErrEmptyDescription
	}
	note := repository.Notification{
		Text:   text,
		SendAt: sendAt.UTC(),
		Status: repository.StQueued,
	}

	ctx, cancel := withTimeout(reqCTX, 1*time.Second)
	defer cancel()

	if err := s.repo.Create(ctx, &note); err != nil { // отправляем в базу
		log.Printf("Failed to save notification %q in DB: %v", note.ID, err)
		return nil, fmt.Errorf("failed to save a notification in DB: %w", err) // 500
	}

	ttl := time.Until(note.SendAt)
	if err := s.q.Publish(ctx, []byte(note.ID), "delayed_notifications", rabbitmq.WithExpiration(ttl)); err != nil {
		log.Println("Failed to publish notification to RabbitMQ:", err)
	}

	if err := s.cache.SetByUID(ctx, note.ID, &note); err != nil { // кладем в кэш
		log.Printf("Failed to cache notification %q in Redis: %v", note.ID, err) // 500
	}

	return &note, nil
}

// GetByID возвращает задачу
func (s *NotificationService) GetByID(reqCTX context.Context, id string) (*repository.Notification, error) {
	if len(id) == 0 {
		return nil, ErrEmptyID
	}

	ctx, cancel := withTimeout(reqCTX, 1*time.Second)
	defer cancel()

	cTask, err := s.cache.GetByUID(ctx, id) // проверяем в кеше
	if err != nil {
		log.Printf("Failed to cache notification %q in Redis: %v", id, err) // 500
	} else if cTask != nil {
		return cTask, nil
	}

	dbTask, err := s.repo.GetByID(ctx, id) // просим у БД
	if err != nil {
		return nil, err
	}

	if err := s.cache.SetByUID(ctx, id, dbTask); err != nil { // кладем в кеш
		log.Printf("Failed to update notification %q in cache: %v", id, err)
	}

	return dbTask, nil
}

// GetPending возвращает все задачи, время отправки которых НАСТУПИЛО - то есть в прошлом и в статусе 'planned','queued'
func (s *NotificationService) GetPending(reqCTX context.Context) ([]*repository.Notification, error) {
	ctx, cancel := withTimeout(reqCTX, 3*time.Second)
	defer cancel()

	notes, err := s.repo.GetPending(ctx, time.Now().UTC(), 500)
	if err != nil {
		log.Printf("Failed to get pending notifications from DB: %v", err)
		return nil, fmt.Errorf("failed to get pending notifications from DB: %w", err)
	}
	return notes, nil
}

func (s *NotificationService) GetAll(reqCTX context.Context) ([]*repository.Notification, error) {
	ctx, cancel := withTimeout(reqCTX, 3*time.Second)
	defer cancel()

	notes, err := s.repo.GetAll(ctx)
	if err != nil {
		log.Printf("Failed to get ALL notifications from DB: %v", err)
		return nil, fmt.Errorf("failed to get ALL notifications from DB: %w", err)
	}
	return notes, nil
}

// Delete отменяет задачу - не удаляет!
func (s *NotificationService) Delete(reqCTX context.Context, id string) error {
	if len(id) == 0 {
		return ErrEmptyID
	}

	ctx, cancel := withTimeout(reqCTX, 2*time.Second)
	defer cancel()

	n, err := s.repo.UpdateStatus(ctx, id, repository.StCancelled)
	if err != nil {
		return fmt.Errorf("failed to cancel notification %q: %s", id, err)
	}
	// обновление записи в редисе
	if err := s.cache.SetByUID(ctx, id, n); err != nil {
		log.Printf("Failed to update notification %q in cache: %v", id, err)
	}
	return nil
}

// MarkSent отмечает нотификацию отправленной и обновляет ее в кеше
func (s *NotificationService) MarkSent(reqCTX context.Context, id string) error {
	if len(id) == 0 {
		return ErrEmptyID
	}

	ctx, cancel := withTimeout(reqCTX, 2*time.Second)
	defer cancel()

	n, err := s.repo.UpdateStatus(ctx, id, repository.StSent)
	if err != nil {
		return fmt.Errorf("failed to cancel notification %q: %s", id, err)
	}
	// обновление записи в редисе
	if err := s.cache.SetByUID(ctx, id, n); err != nil {
		log.Printf("Failed to update notification %q in cache: %v", id, err)
	}
	return nil
}

// MarkDead отмечает нотификацию dead и обновляет ее в кеше
func (s *NotificationService) MarkDead(reqCTX context.Context, id string) error {
	if len(id) == 0 {
		return ErrEmptyID
	}

	ctx, cancel := withTimeout(reqCTX, 2*time.Second)
	defer cancel()

	n, err := s.repo.UpdateStatus(ctx, id, repository.StDead)
	if err != nil {
		return fmt.Errorf("failed to update notification %q: %s", id, err)
	}
	// обновление записи в редисе
	if err := s.cache.SetByUID(ctx, id, n); err != nil {
		log.Printf("Failed to update notification %q in cache: %v", id, err)
	}
	return nil
}

func (s *NotificationService) IncrRetry(reqCTX context.Context, id string) error {
	if len(id) == 0 {
		return ErrEmptyID
	}

	ctx, cancel := withTimeout(reqCTX, 2*time.Second)
	defer cancel()

	n, err := s.repo.IncrementRetry(ctx, id, repository.StSent)
	if err != nil {
		return fmt.Errorf("failed to increment retry count for notification %q: %s", id, err)
	}
	// обновление записи в редисе
	if err := s.cache.SetByUID(ctx, id, n); err != nil {
		log.Printf("Failed to update notification %q in cache: %v", id, err)
	}
	return nil
}

func (s *NotificationService) PublishRetry(ctx context.Context, id string, delay time.Duration) error {
	return s.q.Publish(
		ctx,
		[]byte(id),
		"delayed_notifications",
		rabbitmq.WithExpiration(delay),
	)
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, d)
}
