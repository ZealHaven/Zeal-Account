// Package repository persists account-domain entities in Postgres.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"account/backend/account/internal/model"
	"account/backend/account/internal/password"
)

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned on unique-constraint violations (e.g. username).
var ErrConflict = errors.New("conflict")

// Account persists accounts.
type Account struct {
	pool *pgxpool.Pool
}

func NewAccount(pool *pgxpool.Pool) *Account { return &Account{pool: pool} }

// Create inserts an account and returns it with generated fields.
func (r *Account) Create(ctx context.Context, username, passwordHash string) (model.Account, error) {
	var a model.Account
	err := r.pool.QueryRow(ctx, `
		INSERT INTO accounts (username, password_hash)
		VALUES ($1, $2)
		RETURNING id, username, password_hash, created_at, updated_at
	`, username, passwordHash).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Account{}, ErrConflict
		}
		return model.Account{}, fmt.Errorf("account create: %w", err)
	}
	return a, nil
}

// GetByUsername returns the account with the given username (case-insensitive).
func (r *Account) GetByUsername(ctx context.Context, username string) (model.Account, error) {
	var a model.Account
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, created_at, updated_at
		FROM accounts WHERE lower(username) = lower($1)
	`, username).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Account{}, ErrNotFound
	}
	if err != nil {
		return model.Account{}, fmt.Errorf("account get by username: %w", err)
	}
	return a, nil
}

// Get returns an account by id.
func (r *Account) Get(ctx context.Context, id uuid.UUID) (model.Account, error) {
	var a model.Account
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, created_at, updated_at
		FROM accounts WHERE id = $1
	`, id).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Account{}, ErrNotFound
	}
	if err != nil {
		return model.Account{}, fmt.Errorf("account get: %w", err)
	}
	return a, nil
}

// UpdateUsername renames an account.
func (r *Account) UpdateUsername(ctx context.Context, id uuid.UUID, username string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE accounts SET username = $2, updated_at = now() WHERE id = $1
	`, id, username)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("account update username: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePassword sets a new password hash.
func (r *Account) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE accounts SET password_hash = $2, updated_at = now() WHERE id = $1
	`, id, passwordHash)
	if err != nil {
		return fmt.Errorf("account update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Persona persists personas.
type Persona struct {
	pool *pgxpool.Pool
}

func NewPersona(pool *pgxpool.Pool) *Persona { return &Persona{pool: pool} }

const personaColumns = `id, account_id, name, identity, description, created_at, updated_at`

// Create inserts a persona for an account.
func (r *Persona) Create(ctx context.Context, accountID uuid.UUID, name string, identity, description *string) (model.Persona, error) {
	var p model.Persona
	err := r.pool.QueryRow(ctx, `
		INSERT INTO personas (account_id, name, identity, description)
		VALUES ($1, $2, $3, $4)
		RETURNING `+personaColumns,
		accountID, name, identity, description,
	).Scan(&p.ID, &p.AccountID, &p.Name, &p.Identity, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return model.Persona{}, fmt.Errorf("persona create: %w", err)
	}
	return p, nil
}

// ListByAccount returns all personas of an account, oldest first.
func (r *Persona) ListByAccount(ctx context.Context, accountID uuid.UUID) ([]model.Persona, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+personaColumns+` FROM personas
		WHERE account_id = $1 ORDER BY created_at
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("persona list: %w", err)
	}
	defer rows.Close()
	var out []model.Persona
	for rows.Next() {
		var p model.Persona
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Identity, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("persona scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one persona, verifying it belongs to the account.
func (r *Persona) Get(ctx context.Context, id, accountID uuid.UUID) (model.Persona, error) {
	var p model.Persona
	err := r.pool.QueryRow(ctx, `
		SELECT `+personaColumns+` FROM personas
		WHERE id = $1 AND account_id = $2
	`, id, accountID).Scan(&p.ID, &p.AccountID, &p.Name, &p.Identity, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Persona{}, ErrNotFound
	}
	if err != nil {
		return model.Persona{}, fmt.Errorf("persona get: %w", err)
	}
	return p, nil
}

// Update patches mutable persona fields. Nil name/description leave those
// fields unchanged; a non-nil identity writes it, and an empty/whitespace
// identity clears it (writeIdentity=true, identity=nil).
func (r *Persona) Update(ctx context.Context, id, accountID uuid.UUID, name, description, identity *string, writeIdentity bool) (model.Persona, error) {
	var p model.Persona
	err := r.pool.QueryRow(ctx, `
		UPDATE personas SET
			name        = COALESCE($3, name),
			identity    = CASE WHEN $4::boolean THEN $5 ELSE identity END,
			description = COALESCE($6, description),
			updated_at  = now()
		WHERE id = $1 AND account_id = $2
		RETURNING `+personaColumns,
		id, accountID, name, writeIdentity, identity, description,
	).Scan(&p.ID, &p.AccountID, &p.Name, &p.Identity, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Persona{}, ErrNotFound
	}
	if err != nil {
		return model.Persona{}, fmt.Errorf("persona update: %w", err)
	}
	return p, nil
}

// RecoveryCode persists recovery codes.
type RecoveryCode struct {
	pool *pgxpool.Pool
}

func NewRecoveryCode(pool *pgxpool.Pool) *RecoveryCode { return &RecoveryCode{pool: pool} }

// ReplaceForAccount wipes a account's codes and inserts fresh ones
// (hashes only — plaintext is returned to the caller once, never stored).
func (r *RecoveryCode) ReplaceForAccount(ctx context.Context, accountID uuid.UUID, codeHashes []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recovery replace begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(ctx, `DELETE FROM recovery_codes WHERE account_id = $1`, accountID); err != nil {
		return fmt.Errorf("recovery replace delete: %w", err)
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recovery_codes (account_id, code_hash) VALUES ($1, $2)
		`, accountID, h); err != nil {
			return fmt.Errorf("recovery replace insert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("recovery replace commit: %w", err)
	}
	return nil
}

// CountUnused returns how many unused codes an account has.
func (r *RecoveryCode) CountUnused(ctx context.Context, accountID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM recovery_codes
		WHERE account_id = $1 AND used_at IS NULL
	`, accountID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("recovery count: %w", err)
	}
	return n, nil
}

// Consume atomically marks one unused code of the account as used.
// Codes are stored with fresh argon2 salts, so matching tries each unused
// row (≤10 per account) until one verifies — then flips used_at in the
// same statement to prevent double-spend under concurrency.
func (r *RecoveryCode) Consume(ctx context.Context, accountID uuid.UUID, normalizedPlain string) (bool, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, code_hash FROM recovery_codes
		WHERE account_id = $1 AND used_at IS NULL
		ORDER BY created_at
	`, accountID)
	if err != nil {
		return false, fmt.Errorf("recovery consume list: %w", err)
	}
	type candidate struct {
		id   uuid.UUID
		hash string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.hash); err != nil {
			rows.Close()
			return false, fmt.Errorf("recovery consume scan: %w", err)
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("recovery consume rows: %w", err)
	}
	if len(candidates) == 0 {
		return false, nil
	}
	for _, c := range candidates {
		if password.Verify(normalizedPlain, c.hash) == nil {
			tag, err := r.pool.Exec(ctx, `
				UPDATE recovery_codes SET used_at = now()
				WHERE id = $1 AND used_at IS NULL
			`, c.id)
			if err != nil {
				return false, fmt.Errorf("recovery consume update: %w", err)
			}
			return tag.RowsAffected() == 1, nil
		}
	}
	return false, nil
}

// IdentityOwnedByOther reports whether the identity is already used by at
// least one persona of a different account.
func (r *Persona) IdentityOwnedByOther(ctx context.Context, identity string, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM personas
			WHERE identity = $1 AND account_id <> $2
		)
	`, identity, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("identity lookup: %w", err)
	}
	return exists, nil
}
