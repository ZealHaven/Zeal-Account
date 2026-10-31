# Zeal Account — MVP Plan

**Status:** planning (2026-10-31)
**Goal:** Zeal Account becomes the single authentication layer for all Zeal
products. Users register with username + password, manage personas, and use
their account to sign in to any Zeal product via standard OIDC — with the
**Persona ID** as the identity every product keys off.

## 0. Decisions (locked)

| Decision | Choice |
|---|---|
| Relationship to `Zeal/Auth` | **Account replaces Auth.** Auth is legacy prior-art only. |
| SSO mechanism | **Standard OIDC** — authorization code + PKCE, RS256 JWTs, JWKS. |
| Stack | **Go + fosite** backend, **SvelteKit** frontend, Postgres, Redis. |
| Identity claim | `sub` = **Persona ID** (system-generated UUID). Authorization stays per-product. |
| Registration fields | Username + password **only**. |
| Forgot password | **Recovery codes** (generated at registration, hashed, single-use) — there is no email on file. |
| Identity verification | **Email OTP via SMTP.** Claims are exclusive only once verified: first account to verify an identity keeps it; every other account's claim on it is removed and that account gets an in-app notification. Phone/SMS verification deferred (phone-shaped identities stay unverified claims). |
| Notifications | **In-app only** (portal bell: list, unread count, mark read). Events: identity-conflict evictions + security notices (password changed, MFA on/off, recovery codes regenerated). |
| MFA | **Optional TOTP** (phase 6). A login step-up, not a recovery mechanism. |
| Persona selection | First authorization shows the picker; the chosen persona becomes the **default** for that client. Portal-side persona changer comes later. |
| Issuer domain | `https://account.zealhaven.id`. First deployment target: **zeal-staging**. |

## 1. Requirements

### User POV

1. Register a new account with **username & password only**.
2. Upon registration, create the **initial persona**: name, identity
   (optional — email, phone, handle, anything), description. The Persona ID
   is system-generated; the user never chooses it.
3. Log in to check and modify personas, change username, or change password.
4. Use the account to access other Zeal products that require
   authentication.
5. When accessing a Zeal product, **choose which persona** to use.

### Product (client) POV

1. Zeal Account is the authentication layer. The ID carried into the
   product is the **Persona ID**.
2. Each product owns its own authorization, keyed by Persona ID.

## 2. Domain model

```
Account (login credential)          Persona (identity presented to products)
┌──────────────────────────┐        ┌──────────────────────────────────────┐
│ id          uuid  (sys)  │ 1    * │ id          uuid  (sys)  ← the ID   │
│ username    text  (uk)   │───────▶│ account_id  uuid  FK                 │
│ password_hash text       │        │ name        text                     │
│ created_at / updated_at  │        │ identity    text  (optional)         │
└──────────────────────────┘        │ description text  (optional)         │
                                    │ created_at / updated_at              │
                                    └──────────────────────────────────────┘
```

Rules:

- An account must have **at least one persona** before it can be used for
  SSO. Registration lands the user in an onboarding flow that creates it.
- A persona belongs to exactly one account. Persona IDs are never reused.
- `identity` is a freeform optional string (email, phone, handle…).
  **Claim-then-verify**: any number of accounts may hold the same identity
  as an *unverified* claim. The first account to **verify** it (email OTP)
  wins it exclusively; every other account's claim of that identity is
  removed and those accounts receive an **in-app notification**. Same-account
  personas may always share an identity. See `account_identities` (§3).
- Deleting a persona is deferred: products may hold data keyed by its ID.
  MVP allows edit only; soft-disable may come later.

## 3. Database schema (Postgres)

### accounts
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | system-generated |
| username | text UK | case-insensitive uniqueness (citext or lowered index) |
| password_hash | text | argon2id |
| created_at / updated_at | timestamptz | |

### personas
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | system-generated — this is the `sub` |
| account_id | uuid FK → accounts | ON DELETE RESTRICT (MVP: no hard deletes) |
| name | text | required |
| identity | text | optional, freeform; claim-then-verify via `account_identities` (§3) |
| description | text | optional |
| created_at / updated_at | timestamptz | |

Index: `(account_id)`.

### account_identities
| Column | Type | Notes |
|---|---|---|
| identity | text | part of PK |
| account_id | uuid FK → accounts | part of PK — a claim by this account |
| status | text | `pending` → `verified` |
| verified_at | timestamptz nullable | set when OTP confirmed |
| otp_hash | text nullable | pending email-OTP code (hashed) |
| otp_expires_at | timestamptz nullable | OTP TTL |
| otp_attempts | int default 0 | brute-force guard on OTP entry |
| created_at | timestamptz | |

- PK `(identity, account_id)` — multiple accounts may hold `pending`
  claims of the same identity; same-account personas share claims freely.
- Every persona create/edit with an identity upserts its `pending` row
  here (an already-`verified` identity can't be re-entered elsewhere).
- **Partial unique index:** `UNIQUE (identity) WHERE status = 'verified'`
  — at most one verified owner per identity.
- **Verification flow:** request OTP → 6-digit code emailed via SMTP
  (hashed at rest, TTL, attempt cap) → confirm sets `verified`, then in
  one transaction: delete all other rows for that identity, set
  `personas.identity = NULL` for personas of other accounts, and insert
  one notification per affected account (`identity_claim_removed`).
  **The notification is part of the same transaction** — an eviction
  without a notification must never be possible (no silent loss).

### notifications
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| account_id | uuid FK → accounts | recipient |
| type | text | `identity_claim_removed`, `password_changed`, `password_reset`, `mfa_enabled`, `mfa_disabled`, `recovery_codes_regenerated` |
| title / body | text | human-readable |
| data | jsonb | structured extras: evicted `identity` value + affected `persona_ids` |
| read_at | timestamptz nullable | unread until set |
| created_at | timestamptz | |

Index: `(account_id, created_at DESC)`.

### recovery_codes
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| account_id | uuid FK → accounts | |
| code_hash | text | hashed code (never plaintext at rest) |
| used_at | timestamptz nullable | single-use |
| created_at | timestamptz | |

Generated at registration (e.g., 10 codes, shown once); regenerable from
settings. Used by the forgot-password flow (§5).

### oauth_clients + fosite storage
Fosite-backed tables (clients, authorization codes, access/refresh tokens,
handled via a fosite store adapter — key-JSONB session storage like the Auth
prototype, which was verified working). Clients are seeded by migration for
first-party Zeal products; dynamic registration is out of MVP scope.

### sessions / revocation
Redis: login sessions (httpOnly cookie), refresh-token and access-token
blacklists for logout propagation.

## 4. OIDC design

- **Flow:** authorization code + PKCE (S256 required). No implicit, no
  client credentials in MVP.
- **Endpoints:**

  | Method | Path | Purpose |
  |---|---|---|
  | GET | `/.well-known/openid-configuration` | discovery |
  | GET | `/.well-known/jwks.json` | RS256 public keys |
  | GET | `/authorize` | start flow → login → persona picker |
  | POST | `/token` | code → tokens / refresh |
  | GET | `/userinfo` | claims for access token |
  | POST | `/revoke` | token revocation |
  | GET | `/logout` | end session (+ RP-initiated logout, phase 5) |

- **Claims:** `sub` = persona UUID. Extra first-party claims:
  `zeal_account_id`, `zeal_persona_name`, `zeal_persona_identity`,
  `zeal_persona_identity_verified`. Access tokens: JWT RS256, ~15 min.
  ID tokens: JWT RS256. Refresh tokens: opaque, hashed, rotated on use.
- **Persona selection:** when the authorize endpoint has no saved default
  persona for this (account, client) pair, it renders the picker (one entry
  per persona of the logged-in account). The chosen persona becomes the
  **default for that client**; subsequent logins skip the picker.
  `prompt=select_account` still forces it. A portal-side persona changer
  is a later feature. An account with zero personas is redirected to
  onboarding.
- **Clients:** first-party Zeal products registered in the clients table
  via migration (client_id, redirect URIs, no secret for public SPA
  clients; confidential clients get a hashed secret).

## 5. HTTP API (Account portal)

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/auth/register` | create account (username, password) → session |
| POST | `/api/auth/login` | username + password → session |
| POST | `/api/auth/logout` | clear session |
| GET | `/api/me` | current account + personas |
| PATCH | `/api/me` | change username |
| POST | `/api/me/password` | change password (requires current password) |
| POST | `/api/auth/forgot-password` | start reset: username → consume a recovery code |
| POST | `/api/auth/reset-password` | recovery code + new password |
| POST | `/api/me/recovery-codes` | regenerate recovery codes (invalidates old) |
| GET | `/api/personas` | list own personas |
| POST | `/api/personas` | create persona |
| PATCH | `/api/personas/{id}` | edit persona |
| POST | `/api/personas/{id}/identity/verify` | request email OTP for the persona's identity |
| POST | `/api/personas/{id}/identity/confirm` | submit OTP → verified (evicts losing claims) |
| GET | `/api/notifications` | list notifications (`?unread=true`) |
| GET | `/api/notifications/unread-count` | badge count |
| POST | `/api/notifications/{id}/read` | mark one read |
| POST | `/api/notifications/read-all` | mark all read |
| GET | `/healthz` | liveness |

Security-relevant changes (password change/reset, MFA on/off, recovery-code
regeneration) fire a notification to the account.

Session: httpOnly, SameSite=Lax, Secure cookie backed by Redis. Rate limits
(Redis) on register/login/password-change.

## 6. Frontend (SvelteKit)

| Route | Purpose |
|---|---|
| `/register` | username + password sign-up |
| `/login` | username + password sign-in |
| `/onboarding` | create first persona (blocking until done) |
| `/persona/select` | SSO persona picker (rendered during `/authorize`) |
| `/settings` | change username / password |
| `/personas` (+ `/personas/[id]`) | list / edit / create personas; identity verified badge + OTP entry |
| `/notifications` | notification list (bell with unread count in the portal header) |
| `/` | portal home: persona overview + product links |

The Go binary is the single front door: API under `/api/*` and
`/authorize` etc., everything else proxied to the SvelteKit server (dev:
Vite; prod: built Node server) — same pattern as `~/Zeal/Monitor`.

## 7. Phases

### Phase 0 — Foundation
- [ ] Repo scaffold: `backend/account` (Go module, chi router, config,
      db pool, healthz), `frontend/account` (SvelteKit + TS), `go.work`
- [ ] `justfile` (up/down/migrate/dev/build/test), `.env.example`,
      docker-compose for Postgres + Redis
- [ ] CI-less quality gate: `just check` = go vet + svelte-check + tsc
- **Done when:** `just dev` serves a healthz from Go and the SvelteKit
  placeholder through the proxy.

### Phase 1 — Accounts & personas
- [ ] Migrations: `accounts`, `personas`
- [ ] Register / login / logout / session cookie (argon2id, rate limits)
- [ ] Persona CRUD; onboarding flow enforcing ≥1 persona
- [ ] Settings: change username, change password
- [ ] Recovery codes generated at registration (hashed, shown once) +
      forgot-password flow (username → recovery code → new password)
- **Done when:** integration test covers register → onboard → login →
  create/edit persona → change password → reset password via recovery
  code; UI works end to end.

### Phase 2 — Identity verification & notifications
- [ ] SMTP mailer config (`.env.example`: host, port, user, pass, from)
      + OTP send/confirm endpoints with TTL + attempt cap
- [ ] Verify transaction: promote claim → evict losing claims + null their
      personas' identity → insert `identity_claim_removed` notifications
- [ ] Notifications: table, list/unread-count/read endpoints, portal bell
      + `/notifications` page; security-event notifications (password
      change/reset, recovery-code regen)
- [ ] Persona detail UI: verify button, OTP entry, verified badge;
      notification when your identity got evicted
- **Done when:** integration test covers two accounts claiming one email →
  first to confirm OTP keeps it, second loses the claim, and the
  `identity_claim_removed` notification exists atomically with the
  eviction (verifiable per persona_id); security changes produce
  notifications.

### Phase 3 — OIDC provider
- [ ] Fosite store adapter + migrations for clients/tokens; seed a test
      first-party client
- [ ] Endpoints: discovery, JWKS, authorize, token, userinfo, revoke
- [ ] Login + persona-picker screens integrated into `/authorize`
      (`prompt=select_account`, first-pick-becomes-default)
- [ ] RS256 key management (generate at bootstrap, JWKS served)
- [ ] E2E test: full authorization-code + PKCE loop, assert `sub` = persona
      ID
- **Done when:** a test client completes the flow and gets a verifiable
  JWT with the persona `sub`.

### Phase 4 — First product integration
- [ ] Integration guide in `docs/` (redirect setup, token validation via
      JWKS, handling persona claims, refresh rotation)
- [ ] Bootstrap script/migration to register a real Zeal product client
- **Done when:** one real Zeal product authenticates against Account in a
  staging environment.

### Phase 5 — Hardening & ops
- [ ] Refresh-token rotation + reuse detection, logout propagation
      (revocation blacklist), RP-initiated logout
- [ ] Brute-force protection, security headers, CSRF on portal forms
- [ ] Deployment: Dockerfile, systemd/nginx/Caddy config, backups for
      Postgres; issuer URL `https://account.zealhaven.id`, first deploy
      to **zeal-staging**
- **Done when:** deployable single-command prod install; attack-surface
  checklist reviewed.

### Phase 6 — Optional MFA (TOTP)
- [ ] TOTP enrollment in portal (re-enter password to enable); backup
      codes generated at enable time (hashed, single-use)
- [ ] Step-up screen in the login/authorize flow when MFA is enabled
- [ ] Disable/recovery: TOTP removal via password + backup code
- **Done when:** an MFA-enabled account requires TOTP at login and at
  `/authorize`; backup-code recovery works; accounts without MFA are
  unaffected.

## 8. Assumptions

- No email/phone at registration ⇒ **no email verification** and no
  email-based password recovery. Password change works while logged in;
  locked-out users recover via **recovery codes** (§3, §5).
- `identity` uses a **claim-then-verify** model: pending claims may collide,
  verification is by **email OTP via SMTP** (no SMS in MVP — phone-shaped
  identities stay unverified), first verified account wins and losers are
  evicted + notified. Losing an identity sets the persona's `identity` to
  NULL; the persona itself is untouched.
- "Identity" without verification is informational text — products should
  trust verified identities only; the OIDC persona claims include
  verification status (`zeal_persona_identity_verified`).
- Notifications are in-app only (bell in the portal); no email/digest for
  them.
- The persona picker runs **once per client**: the first pick becomes the
  default. Switching personas from the portal is a later feature.
- One active login session per browser; multiple concurrent sessions
  allowed (listable/revocable later).
- Persona deletion is out of MVP scope (products may reference the ID);
  edit only.
- All Zeal products are first-party and trusted; dynamic client
  registration and third-party consent screens are post-MVP.
- `Zeal/Auth` remains untouched as a separate repo until Account reaches
  phase 4.

## 9. Product decisions (resolved 2026-10-31)

1. **Forgot password:** recovery codes generated at registration (hashed,
   single-use, regenerable). Chosen because identity is optional and no
   email exists.
2. **Identity exclusivity:** claims are free until verified. An identity may
   be claimed by multiple accounts; the first account to **verify** it
   (email OTP via SMTP) owns it exclusively. Every other account's claim
   on that identity is removed and that account gets an in-app
   notification. Same-account personas may always share an identity.
3. **Persona selection:** first pick becomes the default (per client);
   the picker does not run again by default. A persona changer is a
   later portal feature; `prompt=select_account` can force the picker.
4. **Domain:** issuer is `https://account.zealhaven.id`; first deployment
   target is **zeal-staging**.
5. **Verification channel:** email OTP via SMTP. SMS/phone verification is
   deferred — no SMS provider in MVP.
6. **Notifications:** in-app only (bell + `/notifications`). Scope:
   identity-conflict evictions and security events (password change/reset,
   MFA on/off, recovery codes regenerated). No email digests, no
   preferences UI, no admin broadcasts in MVP.

## 10. Reference

- Fosite: <https://github.com/ory/fosite>
- OIDC Core: <https://openid.net/specs/openid-connect-core-1_0.html>
- Prior art: `~/Zeal/Auth` (working fosite integration, verified
  authorization-code E2E), `~/Zeal/Monitor` (Go single-front-door +
  SvelteKit pattern)
