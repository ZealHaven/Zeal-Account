// Package password hashes and verifies account passwords with argon2id.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameters tuned for interactive login (~50-100ms on a modern laptop).
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // KiB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// ErrMismatch is returned when a hash does not verify.
var ErrMismatch = errors.New("password: mismatch")

// Hash derives an PHC-format string: $argon2id$v=19$m=...,t=...,p=...$salt$hash.
func Hash(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: salt: %w", err)
	}
	sum := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

// Verify checks plain against an encoded hash in constant time.
func Verify(plain, encoded string) error {
	salt, sum, err := decode(encoded)
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	if subtle.ConstantTimeCompare(got, sum) != 1 {
		return ErrMismatch
	}
	return nil
}

func decode(encoded string) (salt, sum []byte, err error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, errors.New("password: malformed hash")
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, fmt.Errorf("password: salt: %w", err)
	}
	sum, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, fmt.Errorf("password: hash: %w", err)
	}
	return salt, sum, nil
}
