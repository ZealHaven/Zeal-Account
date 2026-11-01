// Package model holds account-domain data types shared across layers.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Account is the login credential: username + password, nothing else.
type Account struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Persona is the identity an account presents to Zeal products.
// Its ID is the OIDC `sub` issued to products.
type Persona struct {
	ID          uuid.UUID `json:"id"`
	AccountID   uuid.UUID `json:"-"`
	Name        string    `json:"name"`
	Identity    *string   `json:"identity"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RecoveryCode is a single-use forgot-password code, stored hashed.
type RecoveryCode struct {
	ID        uuid.UUID  `json:"id"`
	AccountID uuid.UUID  `json:"-"`
	CodeHash  string     `json:"-"`
	UsedAt    *time.Time `json:"-"`
	CreatedAt time.Time  `json:"created_at"`
}
