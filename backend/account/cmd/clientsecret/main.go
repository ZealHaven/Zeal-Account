// Command clientsecret prints a fresh secret pair for seeding a first-party
// OAuth client: the plaintext (goes into the product's .env — git-ignored)
// and its bcrypt hash (goes into the Account migration). Never commit the
// plaintext. Usage:
//
//	go run ./cmd/clientsecret
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fmt.Fprintln(os.Stderr, "generate secret:", err)
		os.Exit(1)
	}
	secret := base64.StdEncoding.EncodeToString(raw)
	// Cost 12 matches fosite's default bcrypt work factor; any cost works
	// since the hash embeds its own parameters.
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash secret:", err)
		os.Exit(1)
	}
	fmt.Printf("OIDC_CLIENT_SECRET=%s\n", secret)
	fmt.Printf("secret_hash=%s\n", hash)
}
