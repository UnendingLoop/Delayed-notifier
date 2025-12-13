// Package sender provides methods for sending notifications - Telegram, email etc.
package sender

import (
	"log"

	"github.com/UnendingLoop/delayed-notifier/internal/repository"
)

type Sender interface {
	Send(n *repository.Notification) error
}

type LogSender struct{}

func NewLogSender() *LogSender {
	return &LogSender{}
}

func (ls *LogSender) Send(n *repository.Notification) error {
	log.Printf("Notification %q IS SENT: %d time", n.ID, n.RetryCount)
	return nil
}
