package repository

// In-Memory storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type InMemoryRepo struct {
	mu    sync.Mutex
	tasks map[string]*Notification
}

func NewInMemoryRepo() NotificationRepository {
	return &InMemoryRepo{
		tasks: make(map[string]*Notification),
	}
}

// Create новую задачу
func (s *InMemoryRepo) Create(ctx context.Context, n *Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n.ID = uuid.New().String()
	if n.ID == "" {
		return fmt.Errorf("failed to generate new uuid, try again")
	}
	n.CreatedAt = time.Now().UTC()

	s.tasks[n.ID] = n
	return nil
}

// GetByID возвращает задачу по ee id
func (s *InMemoryRepo) GetByID(ctx context.Context, id string) (*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("ID %q not found in map", id)
	}
	return task, nil
}

// GetAll возвращает все задачи
func (s *InMemoryRepo) GetAll(ctx context.Context) ([]*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Notification, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result, nil
}

// GetPending возвращает все задачи, которые уже готовы к отправке
func (s *InMemoryRepo) GetPending(ctx context.Context, until time.Time, limit int) ([]*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Notification, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status != "sent" && t.SendAt.Before(time.Now().UTC()) {
			result = append(result, t)
		}
	}
	return result, nil
}

// MarkDelivered меняет статус нотификации по  предоставленному ID на "sent"
func (s *InMemoryRepo) MarkDelivered(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists := s.tasks[id]
	if !exists {
		return fmt.Errorf("ID %q not found to mark delivery", id)
	}
	task.Status = StSent
	task.UpdatedAt = time.Now().UTC()
	task.Delivered = true
	return nil
}

// UpdateStatus меняет статус нотификации по ее предоставленному ID на входную строку "status"
func (s *InMemoryRepo) UpdateStatus(ctx context.Context, id string, status string) (*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists := s.tasks[id]
	if !exists {
		return nil, ErrNotFound
	}
	task.Status = status
	return task, nil
}

// IncrementRetry увеличивает кол-во совершеных попыток переотправки для определенной нотификации
func (s *InMemoryRepo) IncrementRetry(ctx context.Context, id string, errMsg string) (*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, exists := s.tasks[id]
	if !exists {
		return task, fmt.Errorf("ID %q not found to increment retry attempts", id)
	}
	task.RetryCount++
	return task, nil
}
