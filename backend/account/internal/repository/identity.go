package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"account/backend/account/internal/model"
	"account/backend/account/internal/password"
)

// OTP TTL + attempt budget for identity verification codes.
const (
	OTPTimeToLive  = 15 * time.Minute
	OTPMaxAttempts = 5
)

// OTP verification failures (distinct messages so the UI can hint).
var (
	ErrOTPInvalid = errors.New("verification code is incorrect")
	ErrOTPExpired = errors.New("verification code has expired, request a new one")
)

// Eviction describes what a verification removed, for the caller to log.
type Eviction struct {
	Identity         string
	AccountsNotified int
	PersonasCleared  int
}

// IdentityClaim persists claim-then-verify identity rows.
type IdentityClaim struct {
	pool *pgxpool.Pool
}

func NewIdentityClaim(pool *pgxpool.Pool) *IdentityClaim {
	return &IdentityClaim{pool: pool}
}

// Claim ensures a pending (or existing verified-by-this-account) claim row
// exists for the identity, without touching any OTP state. Called on every
// persona create/update that carries an identity.
func (r *IdentityClaim) Claim(ctx context.Context, identity string, accountID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO account_identities (identity, account_id)
		VALUES ($1, $2)
		ON CONFLICT (identity, account_id) DO NOTHING
	`, identity, accountID)
	if err != nil {
		return fmt.Errorf("identity claim: %w", err)
	}
	return nil
}

// SetOTP stores a fresh verification code for the claim.
func (r *IdentityClaim) SetOTP(ctx context.Context, identity string, accountID uuid.UUID, codeHash string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE account_identities
		SET otp_hash = $3, otp_expires_at = $4, otp_attempts = 0
		WHERE identity = $1 AND account_id = $2
	`, identity, accountID, codeHash, time.Now().Add(OTPTimeToLive))
	if err != nil {
		return fmt.Errorf("identity set otp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimStatus reports the claim's verification state.
type ClaimStatus int

const (
	ClaimNone ClaimStatus = iota
	ClaimPending
	ClaimVerifiedByAccount
)

// Status returns whether this account holds the identity, and how.
func (r *IdentityClaim) Status(ctx context.Context, identity string, accountID uuid.UUID) (ClaimStatus, error) {
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT status FROM account_identities
		WHERE identity = $1 AND account_id = $2
	`, identity, accountID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaimNone, nil
	}
	if err != nil {
		return ClaimNone, fmt.Errorf("identity status: %w", err)
	}
	if status == "verified" {
		return ClaimVerifiedByAccount, nil
	}
	return ClaimPending, nil
}

// VerifiedByOther reports whether the identity is VERIFIED by a different
// account (pending claims by others don't count).
func (r *IdentityClaim) VerifiedByOther(ctx context.Context, identity string, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_identities
			WHERE identity = $1 AND account_id <> $2 AND status = 'verified'
		)
	`, identity, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("identity verified lookup: %w", err)
	}
	return exists, nil
}

// Verify atomically promotes a claim to verified: it checks the OTP
// (argon2 hash, TTL, attempt cap), then in the SAME transaction deletes
// every other account's claim on the identity, clears their personas'
// identity field, and inserts one identity_claim_removed notification per
// affected account. An eviction without its notification is impossible.
//
// Returns ErrOTP* sentinels on bad codes, ErrNotFound when the claim row
// is missing, and an Eviction summary describing what was removed.
func (r *IdentityClaim) Verify(ctx context.Context, identity string, accountID uuid.UUID, plainCode string) (*Eviction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity verify begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	// Lock this claim row — concurrent confirms serialize here.
	var storedHash *string
	var expiresAt *time.Time
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT otp_hash, otp_expires_at, otp_attempts
		FROM account_identities
		WHERE identity = $1 AND account_id = $2
		FOR UPDATE
	`, identity, accountID).Scan(&storedHash, &expiresAt, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identity verify lock: %w", err)
	}
	if storedHash == nil || password.Verify(plainCode, *storedHash) != nil {
		// Count the miss; burn the code after too many tries.
		attempts++
		if attempts >= OTPMaxAttempts {
			_, _ = tx.Exec(ctx, `
				UPDATE account_identities
				SET otp_hash = NULL, otp_expires_at = NULL, otp_attempts = 0
				WHERE identity = $1 AND account_id = $2
			`, identity, accountID)
		} else {
			_, _ = tx.Exec(ctx, `
				UPDATE account_identities SET otp_attempts = $3
				WHERE identity = $1 AND account_id = $2
			`, identity, accountID, attempts)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("identity verify commit miss: %w", err)
		}
		return nil, ErrOTPInvalid
	}
	if expiresAt == nil || time.Now().After(*expiresAt) {
		return nil, ErrOTPExpired
	}

	// Collect losing accounts and their affected personas BEFORE deleting,
	// so the notifications can name what was taken.
	rows, err := tx.Query(ctx, `
		SELECT p.account_id, p.id
		FROM personas p
		JOIN account_identities ai
		  ON ai.identity = $1 AND ai.account_id = p.account_id
		WHERE p.identity = $1 AND p.account_id <> $2
	`, identity, accountID)
	if err != nil {
		return nil, fmt.Errorf("identity verify losers: %w", err)
	}
	type loser struct {
		accountID uuid.UUID
		personaID uuid.UUID
	}
	var losers []loser
	for rows.Next() {
		var l loser
		if err := rows.Scan(&l.accountID, &l.personaID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("identity verify loser scan: %w", err)
		}
		losers = append(losers, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity verify losers rows: %w", err)
	}

	// 1. Claim rows of other accounts for this identity are gone.
	if _, err := tx.Exec(ctx, `
		DELETE FROM account_identities WHERE identity = $1 AND account_id <> $2
	`, identity, accountID); err != nil {
		return nil, fmt.Errorf("identity verify delete claims: %w", err)
	}
	// 2. Their personas' identity field is cleared.
	if _, err := tx.Exec(ctx, `
		UPDATE personas SET identity = NULL, updated_at = now()
		WHERE identity = $1 AND account_id <> $2
	`, identity, accountID); err != nil {
		return nil, fmt.Errorf("identity verify clear personas: %w", err)
	}
	// 3. One notification per losing account, naming their personas.
	notified := map[uuid.UUID][]uuid.UUID{}
	for _, l := range losers {
		notified[l.accountID] = append(notified[l.accountID], l.personaID)
	}
	for acctID, personaIDs := range notified {
		body := fmt.Sprintf("The identity %q was verified by another account and removed from %d of your personas.", identity, len(personaIDs))
		data := map[string]any{"identity": identity, "persona_ids": personaIDs}
		if err := InsertNotification(ctx, tx, acctID, model.NotifyIdentityClaimRemoved,
			"Identity removed", body, data); err != nil {
			return nil, err
		}
	}
	// 4. Promote this claim.
	if _, err := tx.Exec(ctx, `
		UPDATE account_identities
		SET status = 'verified', verified_at = now(),
		    otp_hash = NULL, otp_expires_at = NULL, otp_attempts = 0
		WHERE identity = $1 AND account_id = $2
	`, identity, accountID); err != nil {
		return nil, fmt.Errorf("identity verify promote: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("identity verify commit: %w", err)
	}
	return &Eviction{
		Identity:         identity,
		AccountsNotified: len(notified),
		PersonasCleared:  len(losers),
	}, nil
}

// randomOTP returns a 6-digit numeric code.
func randomOTP() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("otp: %w", err)
	}
	// Rejection-free modulo bias is irrelevant for 6 digits + rate limits.
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1_000_000), nil
}

// HashOTP is a helper for handlers: generates and hashes a code, returning
// the plaintext for delivery and the hash for storage.
func HashOTP() (plain, hash string, err error) {
	plain, err = randomOTP()
	if err != nil {
		return "", "", err
	}
	hash, err = password.Hash(plain)
	if err != nil {
		return "", "", fmt.Errorf("otp hash: %w", err)
	}
	return plain, hash, nil
}
