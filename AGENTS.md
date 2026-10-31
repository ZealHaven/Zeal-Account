# AGENTS.md

Guidance for AI agents working in the Zeal Account repository.

## What this project is

The single authentication layer for the Zeal product family. It owns login
accounts (username + password) and **personas** (system-generated ID, name,
optional identity string, description). It is a standard **OIDC provider**:
other Zeal products redirect here for login, the user picks a persona, and
the product receives JWTs whose `sub` claim is the **Persona ID**.

It supersedes the older `~/Zeal/Auth` prototype. Do not build on or reference
that codebase unless explicitly asked; it may be used as prior art (it proved
the fosite integration).

## Hard rules

- **Persona ID is the contract.** `sub` in every issued token MUST be a
  persona UUID. Products key their authorization data off it. Never put the
  account ID in `sub`.
- **Registration takes username + password only.** No email, no phone at
  signup. Do not add required fields to the registration flow.
- **Authorization is not this service's job.** Roles, permissions, and
  per-product access live in each product. Account only authenticates.
- **Never log or store plaintext credentials.** Passwords are argon2id
  hashed; tokens are stored hashed/rotated per the plan.
- **PKCE (S256) is mandatory** for all OAuth clients. No exceptions.

## Stack and layout

- Go backend in `backend/account/` (module `account/backend/account`),
  [fosite](https://github.com/ory/fosite) for OAuth2/OIDC, Postgres,
  Redis.
- SvelteKit frontend in `frontend/account/` — serves the portal (persona
  management, username/password settings) and the OIDC UI (login, persona
  picker, consent).
- In dev the Go process serves the API and proxies the Vite dev server; in
  production it proxies the built SvelteKit server (single front door,
  same pattern as `~/Zeal/Monitor`).
- SQL migrations in `migrations/`, plain SQL, numbered, forward-only.
- Task runner: `just`. See the `justfile` for canonical commands — prefer
  adding a recipe over inventing ad-hoc commands.

## Conventions

- Follow the file/module structure in the plan (`docs/plans/1-mvp.md`):
  `cmd/`, `internal/{config,db,handler,repository,service,oidc}`.
- Frontend routes mirror intent: `/register`, `/login`, `/onboarding`
  (first persona), `/persona/select` (SSO picker), `/settings`.
- Keep the Go binary the single public entry point; don't expose the
  frontend or Postgres/Redis directly.
- Tests: unit tests next to the code; the OIDC flows get an integration
  test that runs the full authorization-code + PKCE loop against a real
  Postgres + Redis (see `just test-integration`).

## Docs

- Requirements, data model, OIDC design, and phase plan:
  `docs/plans/1-mvp.md` — keep it updated when decisions change; record
  deviations there, don't silently diverge.
- README.md is the human-facing overview; keep the two in sync at a high
  level.
- When a phase ships, mark it in the plan (✅/⚠️/❌) like the sibling
  projects do.

## Product decisions (locked)

Resolved in `docs/plans/1-mvp.md` §9 — follow these, don't re-litigate:

- **Forgot password = recovery codes** (generated at registration, hashed,
  single-use, regenerable). No email exists, so no email reset. Optional
  **TOTP MFA** (phase 6) is a login step-up, not a recovery mechanism.
- **Identity = claim-then-verify:** pending claims may collide freely; the
  first account to verify via **email OTP (SMTP)** owns the identity
  exclusively. Losing claims are removed, their personas' `identity`
  nulled, and the affected accounts get an in-app notification. Same-account
  personas may share an identity. Enforced by `account_identities`
  (PK `(identity, account_id)`, partial unique index on verified rows).
- **Notifications are in-app only** (portal bell + `/notifications`):
  identity evictions and security events. Don't add email digests.
- **Persona picker:** first pick per client becomes the default; the
  picker does not re-run. `prompt=select_account` forces it. Portal-side
  persona changer is a later feature.
- **Issuer:** `https://account.zealhaven.id`; first deploy target
  **zeal-staging**.
