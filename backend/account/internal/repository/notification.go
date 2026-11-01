package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"account/backend/account/internal/model"
)

// Notification persists in-app notifications.
type Notification struct {
	pool *pgxpool.Pool
}

func NewNotification(pool *pgxpool.Pool) *Notification { return &Notification{pool: pool} }

const notificationColumns = `id, account_id, type, title, body, data, read_at, created_at`

// InsertNotification adds a notification. q may be the pool or a pgx
// transaction, so identity eviction can notify atomically.
func InsertNotification(ctx context.Context, q querier, accountID uuid.UUID, typ, title, body string, data any) error {
	raw := []byte("{}")
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("notification data: %w", err)
		}
		raw = b
	}
	_, err := q.Exec(ctx, `
		INSERT INTO notifications (account_id, type, title, body, data)
		VALUES ($1, $2, $3, $4, $5)
	`, accountID, typ, title, body, raw)
	if err != nil {
		return fmt.Errorf("notification insert: %w", err)
	}
	return nil
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Insert adds a notification for the account (pool-backed).
func (r *Notification) Insert(ctx context.Context, accountID uuid.UUID, typ, title, body string, data any) error {
	return InsertNotification(ctx, r.pool, accountID, typ, title, body, data)
}

// List returns an account's notifications, newest first.
func (r *Notification) List(ctx context.Context, accountID uuid.UUID, unreadOnly bool, limit int) ([]model.Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT ` + notificationColumns + ` FROM notifications
		WHERE account_id = $1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, q, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("notification list: %w", err)
	}
	defer rows.Close()
	out := []model.Notification{}
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.AccountID, &n.Type, &n.Title, &n.Body, &n.Data, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("notification scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount returns the badge count for an account.
func (r *Notification) UnreadCount(ctx context.Context, accountID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE account_id = $1 AND read_at IS NULL
	`, accountID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("notification unread count: %w", err)
	}
	return n, nil
}

// MarkRead marks one notification read if the account owns it.
// Returns ErrNotFound when no such notification exists for the account;
// an already-read notification is an idempotent no-op.
func (r *Notification) MarkRead(ctx context.Context, id, accountID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE id = $1 AND account_id = $2 AND read_at IS NULL
	`, id, accountID)
	if err != nil {
		return fmt.Errorf("notification mark read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM notifications WHERE id = $1 AND account_id = $2)
		`, id, accountID).Scan(&exists); err != nil {
			return fmt.Errorf("notification exists check: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAllRead marks every unread notification of the account.
func (r *Notification) MarkAllRead(ctx context.Context, accountID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE account_id = $1 AND read_at IS NULL
	`, accountID)
	if err != nil {
		return fmt.Errorf("notification mark all read: %w", err)
	}
	return nil
}
