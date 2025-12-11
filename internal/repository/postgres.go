package repository

// PostgreSQL storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wb-go/wbf/dbpg"
)

type PostgresRepo struct {
	db *dbpg.DB
}

func NewPostgresRepo(db *dbpg.DB) *PostgresRepo {
	return &PostgresRepo{db: db}
}

func (r *PostgresRepo) Create(ctx context.Context, n *Notification) error {
	query := `INSERT INTO notifications (id, recipient, channel, payload, text, send_at, status)
	VALUES (DEFAULT, $1, $2, $3, $4, $5, $6)
	RETURNING id`
	payload := n.Payload
	if payload == nil {
		payload = []byte("null")
	}
	err := r.db.QueryRowContext(ctx, query,
		n.Recipient, n.Channel, payload, n.Text, n.SendAt, n.Status,
	).Scan(n.ID)
	if err != nil {
		return err
	}
	return nil
}

func (r *PostgresRepo) GetByID(ctx context.Context, id string) (*Notification, error) {
	q := `SELECT id, recipient, channel, payload, text, send_at, status, retry_count, last_error, created_at, updated_at
	      FROM notifications WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	var n Notification
	var payload []byte
	var lastError sql.NullString
	if err := row.Scan(&n.ID, &n.Recipient, &n.Channel, &payload, &n.Text, &n.SendAt, &n.Status, &n.RetryCount, &lastError, &n.CreatedAt, &n.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lastError.Valid {
		s := lastError.String
		n.LastError = &s
	} else {
		n.LastError = nil
	}

	n.Payload = payload
	return &n, nil
}

func (r *PostgresRepo) GetPending(ctx context.Context, until time.Time, limit int) ([]*Notification, error) {
	q := `SELECT id, recipient, channel, payload, text, send_at, status, retry_count, last_error, created_at, updated_at
	      FROM notifications
	      WHERE status IN ('planned','queued') AND send_at <= $1
	      ORDER BY send_at ASC
	      LIMIT $2`
	rows, err := r.db.QueryContext(ctx, q, until, limit)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]*Notification, 0)
	for rows.Next() {
		var n Notification
		var payload []byte
		var lastError sql.NullString
		if err := rows.Scan(&n.ID, &n.Recipient, &n.Channel, &payload, &n.Text, &n.SendAt, &n.Status, &n.RetryCount, &lastError, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		if lastError.Valid {
			n.LastError = &lastError.String
		}
		n.Payload = payload
		res = append(res, &n)
	}
	return res, nil
}

func (r *PostgresRepo) MarkDelivered(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE notifications SET status='delivered', updated_at = now() WHERE id = $1`, id)
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return err
}

func (r *PostgresRepo) IncrementRetry(ctx context.Context, id string, errMsg string) (*Notification, error) {
	q := `UPDATE notifications SET retry_count = retry_count + 1, last_error = $2, updated_at = now()
	      WHERE id = $1
	      RETURNING id, recipient, channel, payload, text, send_at, status, retry_count, last_error, created_at, updated_at`
	var note Notification
	if err := r.db.QueryRowContext(ctx, q, id, errMsg).Scan(&note.ID, &note.Recipient, &note.Channel, &note.Payload, &note.Text, &note.SendAt, &note.Status, &note.RetryCount, &note.LastError, &note.CreatedAt, &note.UpdatedAt); err != nil {
		return nil, err
	}
	return &note, nil
}

func (r *PostgresRepo) UpdateStatus(ctx context.Context, id string, status string) (*Notification, error) {
	res := r.db.QueryRowContext(ctx, `
	UPDATE notifications 
	SET status=$2, updated_at = now() 
	WHERE id = $1
	RETURNING id, recipient, channel, payload, text, send_at, status, retry_count, last_error, created_at, updated_at
	`, id, status)

	if errors.Is(res.Err(), sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	var n Notification
	var payload []byte
	var lastError sql.NullString
	if err := res.Scan(&n.ID, &n.Recipient, &n.Channel, &payload, &n.Text, &n.SendAt, &n.Status, &n.RetryCount, &lastError, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}

	if lastError.Valid {
		s := lastError.String
		n.LastError = &s
	} else {
		n.LastError = nil
	}

	n.Payload = payload
	return &n, nil
}
