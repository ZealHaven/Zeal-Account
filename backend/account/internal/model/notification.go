package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Notification is an in-app message for an account: identity evictions
// and security events. Never emailed.
type Notification struct {
	ID        uuid.UUID       `json:"id"`
	AccountID uuid.UUID       `json:"-"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"read_at"`
	CreatedAt time.Time       `json:"created_at"`
}

// Notification type constants — the closed set the portal renders.
const (
	NotifyIdentityClaimRemoved = "identity_claim_removed"
	NotifyPasswordChanged      = "password_changed"
	NotifyPasswordReset        = "password_reset"
	NotifyRecoveryCodesRegen   = "recovery_codes_regenerated"
	NotifyMFAEnabled           = "mfa_enabled"
	NotifyMFADisabled          = "mfa_disabled"
)
