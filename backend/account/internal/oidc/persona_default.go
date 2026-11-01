package oidc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersonaDefault remembers which persona an account picked for a client:
// the picker runs on the first authorize of the pair, then the choice
// becomes the default.
type PersonaDefault struct {
	pool *pgxpool.Pool
}

// NewPersonaDefault wires the repo.
func NewPersonaDefault(pool *pgxpool.Pool) *PersonaDefault {
	return &PersonaDefault{pool: pool}
}

// ErrNoDefault signals no saved persona for the (account, client) pair.
var ErrNoDefault = errors.New("oidc: no default persona")

// Get returns the saved persona for the pair.
func (r *PersonaDefault) Get(ctx context.Context, accountID uuid.UUID, clientID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT persona_id FROM persona_defaults
		WHERE account_id = $1 AND client_id = $2
	`, accountID, clientID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNoDefault
	}
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// Set upserts the default persona for the pair.
func (r *PersonaDefault) Set(ctx context.Context, accountID uuid.UUID, clientID string, personaID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO persona_defaults (account_id, client_id, persona_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, client_id)
		DO UPDATE SET persona_id = $3, updated_at = now()
	`, accountID, clientID, personaID)
	return err
}
