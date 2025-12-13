// Package worker launches worker as a goroutine to process notifications.
// It handles delayed sending, retries with exponential backoff, and dead-lettering.
package worker

import (
	"context"
	"log"
	"math"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/UnendingLoop/delayed-notifier/internal/repository"
	"github.com/UnendingLoop/delayed-notifier/internal/sender"
	"github.com/UnendingLoop/delayed-notifier/internal/service"
)

// Worker обрабатывает нотификации из RabbitMQ
type Worker struct {
	storage *service.NotificationService
	sender  sender.Sender
	channel *amqp.Channel
}

// NewWorker создает нового воркера
func NewWorker(svc *service.NotificationService, s sender.Sender, ch *amqp.Channel) *Worker {
	return &Worker{
		storage: svc,
		sender:  s,
		channel: ch,
	}
}

// StartConsuming запускает потребление из notifications_queue
func (w *Worker) StartConsuming(ctx context.Context) error {
	msgs, err := w.channel.Consume(
		"notifications_queue",
		"",
		false, // AutoAck = false
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			if err := w.HandleMessage(ctx, msg); err != nil {
				log.Println("Error handling message:", err)
			}
		}
	}()
	return nil
}

// HandleMessage основной обработчик сообщений
func (w *Worker) HandleMessage(ctx context.Context, msg amqp.Delivery) error {
	id := string(msg.Body)

	n, err := w.storage.GetByID(ctx, id)
	if err != nil {
		log.Println("failed to fetch notification:", err)
		return msg.Nack(false, true)
	}

	// Если статус уже не "queued", удаляем сообщение
	if n.Status != repository.StQueued {
		return msg.Ack(false)
	}

	now := time.Now().UTC()

	// 1) Если еще рано отправлять → откладываем в delayed_notifications с TTL
	if now.Before(n.SendAt) {
		delay := time.Until(n.SendAt)
		return w.publishDelayed(id, delay, msg)
	}

	// 2) Пытаемся отправить
	if err := w.sender.Send(n); err == nil {
		if err := w.storage.MarkSent(ctx, id); err != nil {
			log.Println("failed to mark sent:", err)
			return msg.Nack(false, true)
		}
		return msg.Ack(false)
	}

	// 3) Ошибка отправки → переотправка в retry_queue
	retries := getRetries(msg)
	if retries >= 5 {
		log.Println("notification dead after retries:", id)
		if err := w.storage.MarkDead(ctx, id); err != nil {
			log.Println("failed to mark dead:", err)
		}
		return msg.Ack(false)
	}

	// 4) Экспоненциальная задержка
	baseDelay := 2 * time.Second
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(retries)))

	return w.publishRetry(id, retries+1, delay, msg)
}

// publishDelayed кладет сообщение в delayed_notifications с TTL
func (w *Worker) publishDelayed(id string, ttl time.Duration, msg amqp.Delivery) error {
	err := w.channel.Publish(
		"", // default exchange
		"delayed_notifications",
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(id),
			Expiration:  formatTTL(ttl),
			Headers:     msg.Headers, // можно перенести заголовки
		},
	)
	if err != nil {
		log.Println("failed to publish to delayed_notifications:", err)
		return msg.Nack(false, true)
	}
	return msg.Ack(false)
}

// publishRetry кладет сообщение в retry_queue с TTL
func (w *Worker) publishRetry(id string, retries int, ttl time.Duration, msg amqp.Delivery) error {
	if msg.Headers == nil {
		msg.Headers = amqp.Table{}
	}
	msg.Headers["x-retries"] = int32(retries)

	err := w.channel.Publish(
		"notifications_exchange",
		"retry",
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(id),
			Expiration:  formatTTL(ttl),
			Headers:     msg.Headers,
		},
	)
	if err != nil {
		log.Println("failed to publish to retry queue:", err)
		return msg.Nack(false, true)
	}
	return msg.Ack(false)
}

// formatTTL возвращает TTL в миллисекундах
func formatTTL(d time.Duration) string {
	return strconv.Itoa(int(d / time.Millisecond))
}

// getRetries извлекает количество попыток из x-retries
func getRetries(msg amqp.Delivery) int {
	if msg.Headers == nil {
		return 0
	}
	if v, ok := msg.Headers["x-retries"]; ok {
		switch t := v.(type) {
		case int32:
			return int(t)
		case int64:
			return int(t)
		case int:
			return t
		}
	}
	return 0
}
