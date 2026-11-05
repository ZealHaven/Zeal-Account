# Zeal Account

Central account system for the Zeal product family. Every Zeal user pools
here: one login account (username + password) owns one or more **personas**,
and the Persona ID is the stable identity that all other Zeal products use.

Zeal Account replaces the earlier `Zeal/Auth` prototype and is the only
authentication layer for Zeal products. Authorization stays in each product.

## Concepts

- **Account** — the login credential. Username + password, nothing else.
  Registration requires only these two fields.
- **Persona** — the identity an account presents to Zeal products. Each
  account creates an initial persona right after registration, and may add
  more later. A persona has a **system-generated ID**, a name, an optional
  identity string (email, phone, handle, …), and a description.
  Identities are claimable by anyone but become exclusive to the first
  account that **verifies** them via an email code — losing claims are
  removed and their owners are notified in-app.
- **Persona ID = `sub`** — when a product authenticates a user through Zeal
  Account, the OIDC `sub` claim is the Persona ID. Products key all of their
  authorization data off the Persona ID.

## How it works (user POV)

1. Register with username + password.
2. Create the initial persona (name, optional identity, description).
3. Log in anytime to manage personas, change username, or change password.
4. Use the account to access any Zeal product — Account handles login, then
   asks which persona to use the first time; the choice becomes the
   default for that product (a persona switcher comes later).

Locked-out users recover access with **recovery codes** shown once at
registration. **TOTP MFA** is an optional login step-up (post-MVP phase).
The portal has an **in-app notification bell** — identity evictions and
security events (password changed, MFA on/off, recovery codes rotated)
show up there.

## How it works (product POV)

- Zeal Account is an **OIDC provider** (authorization code + PKCE).
- Products integrate with standard OIDC client libraries.
- **Integrating a product?** Follow [`docs/integration.md`](docs/integration.md)
  — client registration, redirect setup, JWKS validation, persona claims,
  refresh rotation (worked example: Zeal Connect).
- `sub` in the ID/access token is the **Persona ID**.
- Products validate JWTs locally via JWKS (`/.well-known/jwks.json`).
- Authorization (roles, permissions, per-product data) is owned by each
  product, keyed by Persona ID.

### OIDC endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/.well-known/openid-configuration` | discovery |
| GET | `/.well-known/jwks.json` (also `/jwks.json`) | RS256 keys |
| GET | `/authorize` | start flow → login → persona picker |
| POST | `/token` | code → tokens / refresh (form-encoded) |
| GET | `/userinfo` | claims for the access token |
| POST | `/revoke` | token revocation (RFC 7009) |

### Portal API

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/auth/register` | create account (username, password) → session |
| POST | `/api/auth/login` | username + password → session |
| POST | `/api/auth/logout` | clear session |
| GET | `/api/me` | current account + personas |
| PATCH | `/api/me` | change username |
| POST | `/api/me/password` | change password |
| POST | `/api/auth/forgot-password` · `/api/auth/reset-password` | recovery-code reset |
| POST | `/api/me/recovery-codes` | regenerate recovery codes |
| GET/POST | `/api/personas` · PATCH `/api/personas/{id}` | persona CRUD |
| POST | `/api/personas/{id}/identity/verify` · `/confirm` | email-OTP identity verification |
| GET | `/api/notifications` · `unread-count` · mark-read | in-app notifications |
| POST | `/api/oidc/persona-select` | SSO persona picker callback |

## Stack

- **Go** + [fosite](https://github.com/ory/fosite) (OAuth2/OIDC)
- **SvelteKit** frontend (account portal: login, register, persona picker,
  settings)
- **Postgres** — accounts, personas, identities, notifications, OAuth
  clients, tokens
- **Redis** — sessions, rate limiting, token revocation
- **Docker Compose** — local infra
- **SMTP** — identity-verification codes only (no marketing/email flows)
- **just** — task runner

## Quick start

```bash
# 1. Infra
cp .env.example .env
just up

# 2. Migrate + seed (schema, then master + demo data)
just migrate-up
just seed

# 3. Dev (backend + Vite dev server)
just dev
```

## Documentation

- [`docs/plans/1-mvp.md`](docs/plans/1-mvp.md) — full requirements, data
  model, OIDC design, and phased implementation plan
- [`docs/integration.md`](docs/integration.md) — guide for Zeal products
  integrating as OIDC clients (the phase 4 deliverable)
- [`AGENTS.md`](AGENTS.md) — working guide for AI agents in this repo

## Project layout

```
backend/account/    Go service (OIDC provider, account/persona API)
frontend/account/   SvelteKit app (portal + login/persona UI)
database/migrations/ SQL schema migrations (forward-only, no data)
database/seeds/     seed SQL — required.sql (master) + demo.sql (demo)
ops/                nginx / systemd / deploy helpers
docs/               plans and design notes
```

## Status

Phases 0–5 implemented and integration-tested. **Staging is live** at
`https://account.zealhaven.id` (issuer `https://account.zealhaven.id`,
deployed to staging, 2026-11-04): systemd user units
(`accountd`, `account-web`) + docker compose (Postgres/Redis on loopback)
behind the existing Cloudflare Tunnel; PKCE (S256) mandatory,
full authorization-code + PKCE loop verified end-to-end against staging.
Issuer: `https://account.zealhaven.id`. See
[`docs/plans/1-mvp.md`](docs/plans/1-mvp.md).

Remaining for phase 6 (hardening): backups, SMTP for identity-OTP email,
TOTP MFA, portal-side persona changer, production deploy.

## License

MIT
