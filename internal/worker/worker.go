// Package worker launches worker as a goroutine, which checks
// the list of unsent notifications once in provided period. If
// notification is ready to be sent, it is given to Sender package
package worker

import (
	"context"
	"math"
	"time"

	"github.com/UnendingLoop/delayed-notifier/internal/sender"
	"github.com/UnendingLoop/delayed-notifier/internal/service"
	"github.com/rabbitmq/amqp091-go"
)

type Worker struct {
	storage *service.NotificationService
	sender  sender.Sender
}

func NewWorker(svc *service.NotificationService, sender sender.Sender) *Worker {
	return &Worker{storage: svc, sender: sender}
}

func (w *Worker) HandleMessage(ctx context.Context, msg amqp091.Delivery) error {
	id := string(msg.Body)

	n, err := w.storage.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if n.Status != service.StQueued {
		return msg.Ack(false)
	}

	// Попытка отправки
	if err := w.sender.Send(n); err != nil {

		// считаем количество попыток
		retries := retryCount(msg)

		// ограничение, например 5 попыток
		if retries >= 5 {
			// больше не пробуем → dead-letter
			return msg.Reject(false)
		}

		// вычисляем задержку
		base := 5 * time.Second
		factor := 3.0
		delay := time.Duration(float64(base) * math.Pow(factor, float64(retries)))

		// публикуем повторно в delayed queue
		pubErr := w.storage.PublishRetry(ctx, id, delay)
		if pubErr != nil {
			return pubErr // Nack
		}

		return msg.Reject(false) // исходное убираем
	}

	// Успешная отправка
	if err := w.storage.MarkSent(ctx, id); err != nil {
		return err
	}

	return msg.Ack(false)
}

func retryCount(msg amqp091.Delivery) int {
	if deaths, ok := msg.Headers["x-death"].([]interface{}); ok && len(deaths) > 0 {
		if deathInfo, ok := deaths[0].(amqp091.Table); ok {
			if count, ok := deathInfo["count"].(int64); ok {
				return int(count)
			}
		}
	}
	return 0
}
