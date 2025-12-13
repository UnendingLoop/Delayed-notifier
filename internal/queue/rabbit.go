// Package queue returns Rabbit client, publisher and CFG for creating consumer
package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func NewRabbitInit(amqpURL string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if err := setupQueues(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, err
	}

	log.Println("RabbitMQ topology initialized")
	return conn, ch, nil
}

func setupQueues(ch *amqp.Channel) error {
	// 1) Exchange
	if err := ch.ExchangeDeclare(
		"notifications_exchange",
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	// ---------------------------
	// 2) MAIN QUEUE (Queue2) – для воркера
	// ---------------------------
	mainArgs := amqp.Table{
		"x-dead-letter-exchange":    "notifications_exchange",
		"x-dead-letter-routing-key": "retry",
	}

	if _, err := ch.QueueDeclare(
		"notifications_queue",
		true,
		false,
		false,
		false,
		mainArgs,
	); err != nil {
		return err
	}
	if err := ch.QueueBind(
		"notifications_queue",
		"send",
		"notifications_exchange",
		false,
		nil,
	); err != nil {
		return err
	}

	// ---------------------------
	// 3) RETRY QUEUE (Queue3)
	// ---------------------------
	retryArgs := amqp.Table{
		"x-dead-letter-exchange":    "notifications_exchange",
		"x-dead-letter-routing-key": "send",
	}
	if _, err := ch.QueueDeclare(
		"notifications_retry_queue",
		true,
		false,
		false,
		false,
		retryArgs,
	); err != nil {
		return err
	}
	if err := ch.QueueBind(
		"notifications_retry_queue",
		"retry",
		"notifications_exchange",
		false,
		nil,
	); err != nil {
		return err
	}

	// ---------------------------
	// 4) DELAYED QUEUE (Queue1)
	// ---------------------------
	delayedArgs := amqp.Table{
		"x-dead-letter-exchange":    "notifications_exchange",
		"x-dead-letter-routing-key": "send",
	}
	if _, err := ch.QueueDeclare(
		"delayed_notifications",
		true,
		false,
		false,
		false,
		delayedArgs,
	); err != nil {
		return err
	}

	return nil
}

// PublishDelayed публикует сообщение в Queue1 с TTL (в миллисекундах)
func PublishDelayed(ctx context.Context, ch *amqp.Channel, notificationID string, ttl time.Duration) error {
	return ch.PublishWithContext(ctx,
		"",
		"delayed_notifications", // пустой routingKey, т.к. DLX
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(notificationID),
			Expiration:  fmt.Sprintf("%d", int(ttl.Milliseconds())),
		},
	)
}

// PublishRetry публикует сообщение в RetryQueue с экспоненциальной задержкой
func PublishRetry(ch *amqp.Channel, notificationID string, attempt int) error {
	baseDelay := 10 * time.Second
	delay := baseDelay * time.Duration(1<<attempt) // 10s, 20s, 40s, 80s ...

	headers := amqp.Table{
		"x-attempts": attempt,
	}

	return ch.Publish(
		"",
		"notifications_retry_queue",
		false,
		false,
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(notificationID),
			Expiration:  fmt.Sprintf("%d", int(delay.Milliseconds())),
			Headers:     headers,
		},
	)
}
