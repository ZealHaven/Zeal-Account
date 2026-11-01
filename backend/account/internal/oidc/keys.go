package oidc

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// KeyService manages the RS256 signing keys. The active private key is
// AES-GCM encrypted at rest (KEY_ENC_HEX); a key is generated at bootstrap
// when none exists.
type KeyService struct {
	db  *pgxpool.Pool
	enc []byte // 32-byte AES key
}

// NewKeyService builds the service from a hex-encoded 32-byte key.
func NewKeyService(db *pgxpool.Pool, encKeyHex string) (*KeyService, error) {
	enc, err := hex.DecodeString(encKeyHex)
	if err != nil || len(enc) != 32 {
		return nil, errors.New("oidc: KEY_ENC_HEX must be 32 bytes hex-encoded")
	}
	return &KeyService{db: db, enc: enc}, nil
}

// EnsureActiveKey returns the active key's kid, generating one when none
// exists. If the stored active key can't be decrypted with the configured
// secret (e.g. dev DB mixed with test keys), it is rotated automatically —
// tokens signed with it are already unusable by this server anyway.
func (k *KeyService) EnsureActiveKey(ctx context.Context) (string, error) {
	var kid string
	err := k.db.QueryRow(ctx, `
		SELECT kid FROM signing_keys WHERE status = 'active' LIMIT 1
	`).Scan(&kid)
	if err != nil {
		return k.GenerateKey(ctx)
	}
	if _, derr := k.ActivePrivateKey(ctx); derr != nil {
		// Undecryptable with this KEY_ENC_HEX: rotate.
		return k.GenerateKey(ctx)
	}
	return kid, nil
}

// GenerateKey creates a fresh RSA-2048 key, stores it, and marks it active.
func (k *KeyService) GenerateKey(ctx context.Context) (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("oidc: generate rsa: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", fmt.Errorf("oidc: marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	enc, err := encryptPEM(privPEM, k.enc)
	if err != nil {
		return "", err
	}
	kid := fmt.Sprintf("key-%d", time.Now().Unix())

	tx, err := k.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		UPDATE signing_keys SET status = 'rotated' WHERE status = 'active'
	`); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO signing_keys (kid, public_key, private_key_encrypted, status)
		VALUES ($1, $2, $3, 'active')
	`, kid, string(pubPEM), base64.StdEncoding.EncodeToString(enc)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return kid, nil
}

// ActivePrivateKey decrypts and returns the active signing key.
func (k *KeyService) ActivePrivateKey(ctx context.Context) (*rsa.PrivateKey, error) {
	var encB64 string
	if err := k.db.QueryRow(ctx, `
		SELECT private_key_encrypted FROM signing_keys WHERE status = 'active' LIMIT 1
	`).Scan(&encB64); err != nil {
		return nil, fmt.Errorf("oidc: active key: %w", err)
	}
	enc, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return nil, fmt.Errorf("oidc: key decode: %w", err)
	}
	privPEM, err := decryptPEM(enc, k.enc)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, errors.New("oidc: key PEM malformed")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// PublicJWKS returns the JWK Set for the active key (served at /jwks.json).
func (k *KeyService) PublicJWKS(ctx context.Context) (map[string]any, error) {
	var kid, pubPEM string
	if err := k.db.QueryRow(ctx, `
		SELECT kid, public_key FROM signing_keys WHERE status = 'active' LIMIT 1
	`).Scan(&kid, &pubPEM); err != nil {
		return nil, fmt.Errorf("oidc: jwks: %w", err)
	}
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return nil, errors.New("oidc: public key PEM malformed")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oidc: parse public key: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("oidc: signing key is not RSA")
	}
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigEndianBytes(pub.E)),
		}},
	}, nil
}

func bigEndianBytes(e int) []byte {
	if e == 0 {
		return []byte{0}
	}
	var out []byte
	for e > 0 {
		out = append([]byte{byte(e & 0xff)}, out...)
		e >>= 8
	}
	return out
}

func encryptPEM(plain, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("oidc: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oidc: gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("oidc: nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decryptPEM(enc, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("oidc: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oidc: gcm: %w", err)
	}
	if len(enc) < gcm.NonceSize() {
		return nil, errors.New("oidc: ciphertext too short")
	}
	plain, err := gcm.Open(nil, enc[:gcm.NonceSize()], enc[gcm.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("oidc: decrypt: %w", err)
	}
	return plain, nil
}
