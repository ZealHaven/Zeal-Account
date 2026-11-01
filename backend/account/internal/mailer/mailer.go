// Package mailer sends identity-verification codes. It is deliberately
// narrow: this service has NO marketing, welcome, or digest email — the
// only mail that leaves here is a 6-digit OTP.
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"time"
)

// Config is the SMTP connection info.
type Config struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// Enabled reports whether SMTP is configured.
func (c Config) Enabled() bool { return c.Host != "" && c.From != "" }

// Mailer sends verification codes over SMTP.
type Mailer struct {
	cfg Config
	log *slog.Logger
}

// New builds a Mailer.
func New(cfg Config, log *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, log: log}
}

// SendVerificationCode emails a 6-digit code to the given address.
// When SMTP isn't configured, it logs the code instead (dev-only
// convenience — the flow still works locally).
func (m *Mailer) SendVerificationCode(ctx context.Context, to, code string) error {
	subject := "Your Zeal Account verification code"
	body := fmt.Sprintf(
		"Your verification code is: %s\n\nIt expires in 15 minutes. If you didn't request it, ignore this email.",
		code,
	)
	if !m.cfg.Enabled() {
		m.log.Warn("smtp not configured; logging verification code instead",
			"to", to, "code", code)
		return nil
	}

	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.cfg.From, to, subject, body,
	))

	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}

	// smtp.SendMail has no context; bound it with a goroutine + timeout.
	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, m.cfg.From, []string{to}, msg)
	}()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("smtp send: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(20 * time.Second):
		return fmt.Errorf("smtp send: timeout")
	}
}
