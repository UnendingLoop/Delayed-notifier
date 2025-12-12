// Package queue returns Rabbit client, publisher and CFG for creating consumer
package queue

import (
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/wb-go/wbf/rabbitmq"
	"github.com/wb-go/wbf/retry"
)

func NewRabbitInit(rabbitAddr, rabbitName string) (*rabbitmq.RabbitClient, *rabbitmq.Publisher, rabbitmq.ConsumerConfig) {
	if err := setupRabbitTopology(rabbitAddr); err != nil {
		log.Fatalln("Failed to preconfig RabbitMQ:", err)
	}

	rabbitConfig := rabbitmq.ClientConfig{
		URL:            rabbitAddr,
		ConnectionName: rabbitName, // для идентификации в RabbitMQ UI
		ConnectTimeout: 3 * time.Minute,
		Heartbeat:      1 * time.Minute,
		PublishRetry: retry.Strategy{
			Attempts: 0,           // Количество попыток.
			Delay:    time.Minute, // Начальная задержка между попытками.
			Backoff:  1.23,        // Множитель для увеличения задержки.
		},
		ConsumeRetry: retry.Strategy{
			Attempts: 0,           // Количество попыток.
			Delay:    time.Minute, // Начальная задержка между попытками.
			Backoff:  1.23,        // Множитель для увеличения задержки.
		},
	}
	clientRabbit, err := rabbitmq.NewClient(rabbitConfig)
	if err != nil {
		log.Fatalln("Failed to create RabbitMQ-client:", err)
	}

	producerRabbit := rabbitmq.NewPublisher(
		clientRabbit,
		"",
		"text/plain",
	)

	consumerCFG := rabbitmq.ConsumerConfig{
		Queue:         "notifications_queue",
		ConsumerTag:   "",
		AutoAck:       false,
		Ask:           rabbitmq.AskConfig{},
		Nack:          rabbitmq.NackConfig{},
		Args:          nil,
		Workers:       1,
		PrefetchCount: 1,
	}
	log.Println("Rabbit Init done")
	return clientRabbit, producerRabbit, consumerCFG
}

// call once during StartApp()
func setupRabbitTopology(amqpURL string) error {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

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
	// 2) MAIN QUEUE (worker)
	// ---------------------------
	mainArgs := amqp.Table{
		// ошибки воркера → в retry_queue
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
	// 3) RETRY QUEUE
	// ---------------------------
	// сообщения сюда попадают при Nack
	// задержка фиксированная, напр. 60 секунд
	retryArgs := amqp.Table{
		"x-message-ttl":             int32(60_000), // 60 сек
		"x-dead-letter-exchange":    "notifications_exchange",
		"x-dead-letter-routing-key": "send", // после TTL → обратно в основной воркер
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
	// 4) DEAD QUEUE
	// ---------------------------
	// сюда попадут сообщения после превышения TTL или слишком большого количества попыток
	if _, err := ch.QueueDeclare(
		"notifications_dead_queue",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	if err := ch.QueueBind(
		"notifications_dead_queue",
		"dead",
		"notifications_exchange",
		false,
		nil,
	); err != nil {
		return err
	}

	// ---------------------------
	// 5) DELAYED QUEUE (плановые sendAt)
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

	log.Println("Rabbit topology with retry/dlx/delayed setup done")
	return nil
}
