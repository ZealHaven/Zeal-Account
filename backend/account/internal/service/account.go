// Package service implements account-domain business rules.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"account/backend/account/internal/model"
	"account/backend/account/internal/password"
	"account/backend/account/internal/repository"
)

// Validation failures surfaced to clients as 400s.
var (
	ErrInvalidUsername = errors.New("username must be 3-32 chars: letters, digits, dot, dash, underscore")
	ErrInvalidPassword = errors.New("password must be at least 8 characters")
	ErrInvalidName     = errors.New("persona name is required (1-80 chars)")
	ErrWrongPassword   = errors.New("current password is incorrect")
	ErrNoRecoveryCodes = errors.New("account has no unused recovery codes")
	ErrBadRecoveryCode = errors.New("recovery code is invalid or already used")
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

// ErrIdentityTaken signals an identity claimed by another account (phase 2
// makes this exclusive-after-verification; MVP: blocked outright).
var ErrIdentityTaken = errors.New("identity is already claimed by another account")

const (
	minPasswordLen     = 8
	maxPasswordLen     = 512
	maxPersonaNameLen  = 80
	maxIdentityLen     = 254
	maxDescriptionLen  = 1000
	recoveryCodeCount  = 10
	recoveryCodeLength = 10 // 10 chars, displayed in XXXXX-XXXXX groups
)

// Account wires account operations to their repositories.
type Account struct {
	accounts *repository.Account
	personas *repository.Persona
	recovery *repository.RecoveryCode
}

// NewAccount builds the service.
func NewAccount(accounts *repository.Account, personas *repository.Persona, recovery *repository.RecoveryCode) *Account {
	return &Account{accounts: accounts, personas: personas, recovery: recovery}
}

// RegisterResult is what registration returns to the caller.
type RegisterResult struct {
	Account       model.Account
	RecoveryCodes []string // plaintext — shown once, never stored
}

// Register creates an account + generates recovery codes.
func (s *Account) Register(ctx context.Context, username, pass string) (RegisterResult, error) {
	if !usernameRe.MatchString(username) {
		return RegisterResult{}, ErrInvalidUsername
	}
	if err := checkPassword(pass); err != nil {
		return RegisterResult{}, err
	}
	hash, err := password.Hash(pass)
	if err != nil {
		return RegisterResult{}, err
	}
	acct, err := s.accounts.Create(ctx, strings.ToLower(username), hash)
	if err != nil {
		return RegisterResult{}, err
	}
	codes, hashes, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return RegisterResult{}, err
	}
	if err := s.recovery.ReplaceForAccount(ctx, acct.ID, hashes); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Account: acct, RecoveryCodes: codes}, nil
}

// Authenticate verifies username + password.
func (s *Account) Authenticate(ctx context.Context, username, pass string) (model.Account, error) {
	acct, err := s.accounts.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Uniform failure — don't leak which half was wrong.
			return model.Account{}, password.ErrMismatch
		}
		return model.Account{}, err
	}
	if len(pass) > maxPasswordLen {
		return model.Account{}, password.ErrMismatch
	}
	if err := password.Verify(pass, acct.PasswordHash); err != nil {
		return model.Account{}, err
	}
	return acct, nil
}

// ChangeUsername renames an account.
func (s *Account) ChangeUsername(ctx context.Context, id uuid.UUID, username string) error {
	if !usernameRe.MatchString(username) {
		return ErrInvalidUsername
	}
	return s.accounts.UpdateUsername(ctx, id, strings.ToLower(username))
}

// ChangePassword verifies the current password and sets a new one.
func (s *Account) ChangePassword(ctx context.Context, id uuid.UUID, current, next string) error {
	acct, err := s.accounts.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := password.Verify(current, acct.PasswordHash); err != nil {
		if errors.Is(err, password.ErrMismatch) {
			return ErrWrongPassword
		}
		return err
	}
	if err := checkPassword(next); err != nil {
		return err
	}
	hash, err := password.Hash(next)
	if err != nil {
		return err
	}
	return s.accounts.UpdatePassword(ctx, id, hash)
}

// ResetPassword consumes a recovery code and sets a new password.
func (s *Account) ResetPassword(ctx context.Context, username, code, next string) error {
	acct, err := s.accounts.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrBadRecoveryCode // uniform response; no user enumeration
		}
		return err
	}
	if err := checkPassword(next); err != nil {
		return err
	}
	ok, err := s.recovery.Consume(ctx, acct.ID, normalizeCode(code))
	if err != nil {
		return err
	}
	if !ok {
		return ErrBadRecoveryCode
	}
	hash, err := password.Hash(next)
	if err != nil {
		return err
	}
	return s.accounts.UpdatePassword(ctx, acct.ID, hash)
}

// RegenerateRecoveryCodes replaces an account's codes.
func (s *Account) RegenerateRecoveryCodes(ctx context.Context, id uuid.UUID) ([]string, error) {
	codes, hashes, err := generateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.recovery.ReplaceForAccount(ctx, id, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// Get returns an account by id.
func (s *Account) Get(ctx context.Context, id uuid.UUID) (model.Account, error) {
	return s.accounts.Get(ctx, id)
}

// --- personas ---------------------------------------------------------------

// CreatePersona creates a persona, rejecting identities already bound to
// another account.
func (s *Account) CreatePersona(ctx context.Context, accountID uuid.UUID, name string, identity, description *string) (model.Persona, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxPersonaNameLen {
		return model.Persona{}, ErrInvalidName
	}
	identity, err := cleanIdentity(identity)
	if err != nil {
		return model.Persona{}, err
	}
	description, err = cleanDescription(description)
	if err != nil {
		return model.Persona{}, err
	}
	if identity != nil {
		taken, err := s.personas.IdentityOwnedByOther(ctx, *identity, accountID)
		if err != nil {
			return model.Persona{}, err
		}
		if taken {
			return model.Persona{}, ErrIdentityTaken
		}
	}
	return s.personas.Create(ctx, accountID, name, identity, description)
}

// ListPersonas returns the account's personas.
func (s *Account) ListPersonas(ctx context.Context, accountID uuid.UUID) ([]model.Persona, error) {
	return s.personas.ListByAccount(ctx, accountID)
}

// UpdatePersona patches one persona the account owns. A present-but-empty
// identity clears the field; an absent (nil) identity leaves it unchanged.
func (s *Account) UpdatePersona(ctx context.Context, id, accountID uuid.UUID, name, identity, description *string) (model.Persona, error) {
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" || len(trimmed) > maxPersonaNameLen {
			return model.Persona{}, ErrInvalidName
		}
		name = &trimmed
	}
	var err error
	writeIdentity := identity != nil
	if identity != nil {
		identity, err = cleanIdentity(identity)
		if err != nil {
			return model.Persona{}, err
		}
		if identity != nil {
			taken, err := s.personas.IdentityOwnedByOther(ctx, *identity, accountID)
			if err != nil {
				return model.Persona{}, err
			}
			if taken {
				return model.Persona{}, ErrIdentityTaken
			}
		}
	}
	if description != nil {
		description, err = cleanDescription(description)
		if err != nil {
			return model.Persona{}, err
		}
	}
	return s.personas.Update(ctx, id, accountID, name, description, identity, writeIdentity)
}

// --- helpers ----------------------------------------------------------------

func checkPassword(pass string) error {
	if len(pass) < minPasswordLen || len(pass) > maxPasswordLen {
		return ErrInvalidPassword
	}
	return nil
}

// cleanIdentity trims and validates an optional identity; "" becomes nil.
func cleanIdentity(identity *string) (*string, error) {
	if identity == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*identity)
	if v == "" {
		return nil, nil
	}
	if len(v) > maxIdentityLen {
		return nil, fmt.Errorf("identity must be at most %d characters", maxIdentityLen)
	}
	return &v, nil
}

// cleanDescription trims an optional description; "" becomes nil.
func cleanDescription(description *string) (*string, error) {
	if description == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*description)
	if v == "" {
		return nil, nil
	}
	if len(v) > maxDescriptionLen {
		return nil, fmt.Errorf("description must be at most %d characters", maxDescriptionLen)
	}
	return &v, nil
}

// generateRecoveryCodes returns plaintext codes (dashed for display) and
// their hashes. Hashes are computed over the NORMALIZED (dash-free) form —
// Consume verifies against normalizeCode() output, so the two must match.
func generateRecoveryCodes(n int) (plain, hashes []string, err error) {
	plain = make([]string, 0, n)
	hashes = make([]string, 0, n)
	for i := 0; i < n; i++ {
		code, err := randomCode(recoveryCodeLength)
		if err != nil {
			return nil, nil, err
		}
		h, err := password.Hash(normalizeCode(code))
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, code)
		hashes = append(hashes, h)
	}
	return plain, hashes, nil
}

func randomCode(n int) (string, error) {
	// Unambiguous alphabet: no 0/O, 1/I/L.
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("recovery code: %w", err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	s := string(b)
	return s[:n/2] + "-" + s[n/2:], nil
}

// normalizeCode strips formatting so users can paste "abc12-def34" or
// "abc12def34" alike.
func normalizeCode(code string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, code))
}
