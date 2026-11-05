# Integrating a Zeal product with Zeal Account

Guide for first-party Zeal products that authenticate through Zeal
Account. It covers registering your client, the redirect setup, token
validation via JWKS, the persona claims, and refresh-token rotation.

**Reference implementation:** [Zeal Connect](../../Connect) — its
authorization-code + PKCE client is the worked example throughout, and
`Connect/backend/connect/internal/integration/phase1_test.go` is a
working end-to-end test you can copy. The gotchas in §8 all come from
that first integration.

## 1. Registering your client

Clients are seeded **by SQL seed files** (dynamic registration is
post-MVP; migrations are schema-only and never INSERT). Add your client
to `database/seeds/required.sql` for master data the app needs, or
`database/seeds/demo.sql` for test/demo-only clients:

```sql
INSERT INTO oauth_clients (id, secret_hash, name, scopes, is_public)
VALUES ('connect', '<BCRYPT HASH>', 'Zeal Connect', '{openid,profile}', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO oauth_client_redirect_uris (client_id, redirect_uri)
VALUES ('connect', 'http://localhost:8190/auth/callback'),
       ('connect', 'https://connect.zealhaven.id/auth/callback')
ON CONFLICT (client_id, redirect_uri) DO NOTHING;
```

| Field | Notes |
|---|---|
| `id` | Your product's client ID (e.g. `connect`). Used as `client_id` and as the token `aud`. |
| `secret_hash` | **bcrypt** hash (fosite's default hasher uses `bcrypt.CompareHashAndPassword`). Not the argon2 hash used for user passwords. `NULL` for public clients. |
| `is_public` | `true` for browser-only SPA clients (PKCE only, no secret). `false` for confidential clients (secret required at `/token`). |
| `scopes` | `{openid,profile}` is enough for first-party products. |
| redirect URIs | Matched **exactly** by fosite — scheme, host, port, path. Register dev and prod origins. |

Generate the secret pair (plaintext goes to the product's `.env`, which
is git-ignored; only the hash goes into the seed — never the
plaintext):

```bash
just client-secret        # prints OIDC_CLIENT_SECRET=<plaintext> + the hash
```

Then apply it: `just seed-required` (idempotent, safe to re-run). Keep
the seed files idempotent with `ON CONFLICT ... DO NOTHING` so they can
run on every deploy. Rotate per environment before a real production
deploy by editing the row (the seed row is the dev/staging value).

## 2. Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/.well-known/openid-configuration` | discovery (`issuer`, endpoints, algs) |
| GET | `/jwks.json` (also `/.well-known/jwks.json`) | RS256 public keys |
| GET | `/authorize` | start flow → login → persona picker |
| POST | `/token` | code → tokens / refresh (form-encoded) |
| GET | `/userinfo` | claims for an access token |
| POST | `/revoke` | token revocation (RFC 7009) |

Supported: response type `code`, grant types `authorization_code` +
`refresh_token`, PKCE **S256 only**, token auth `client_secret_basic`
(confidential) or `none` (public + PKCE). Access tokens live **15 min**,
ID tokens **1 h**, refresh tokens **30 d**.

## 3. Redirect setup

The whole journey happens on the **single front door** origin (Account's
Go binary proxies the portal), so browser cookies stay on one host:

```
your product                     Zeal Account (front door)
────────────                     ────────────────────────
GET /auth/login
  ├─ generate state + nonce     ──302──▶  GET /authorize?client_id=…
  │    + PKCE verifier                     ├─ (no session) 302 /login?next=/authorize?…
  │    store in your session               ├─ login form → POST /api/auth/login
  │                                        ├─ (first pick for this client)
  │                                        │    302 /persona/select → picker UI
  │                                        └─ 303 back to YOUR redirect_uri?code=…
  GET /auth/callback?code&state
  └─ POST /token (Basic + verifier) ──▶  code → tokens
  └─ validate ID token via JWKS, upsert your user row, set your session
```

Rules:

- `redirect_uri` on `/authorize` and at `/token` must **exactly** match a
  registered URI (§1); fosite rejects mismatches.
- `state` and `nonce` are single-use; store them server-side (Redis) and
  verify both on the callback.
- PKCE S256 is mandatory for every client, public or confidential.
- The first authorization for each (account, client) pair shows the
  **persona picker**; the choice becomes the default for that client and
  later logins skip it. `prompt=select_account` forces the picker again.
  A `next` you send to `/login` must point back at `/authorize?…`.

## 4. Token exchange

Confidential client (server-side, e.g. Connect):

```
POST /token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&code=…
&redirect_uri=…            (must match the authorize request)
&code_verifier=…           (PKCE)
Authorization: Basic base64(urlencode(client_id):urlencode(client_secret))
```

Public SPA client: same body without the `Authorization` header
(`client_id` in the body; PKCE is the only credential).

Errors are RFC 6749 JSON (`error`, `error_description`); a replayed code
returns `invalid_grant` — codes are single-use and a replay revokes the
whole token family.

## 5. Validating tokens (JWKS)

Validate ID tokens **locally** — no call to Account per request:

1. Fetch `jwks_uri` from discovery; cache it (re-fetch on unknown `kid`).
2. Verify the signature (**RS256**, `kid` matches a JWKS key).
3. Verify claims: `iss` == the issuer exactly, `aud` contains your
   `client_id`, `exp` in the future, `nonce` equals the one you sent.
4. `sub` is the **Persona ID** — see §6.

Access tokens are JWTs too (same key, 15 min) — for `GET /userinfo` or
API calls. Opaque refresh tokens are not JWTs: never parse them; send
them to `/token` only.

## 6. Persona claims — the contract

| Claim | Example | Meaning |
|---|---|---|
| `sub` | `7fd618f4-…` | **Persona ID.** Your primary key for the user. System-generated UUID, never reused. |
| `zeal_account_id` | `dd7be26a-…` | Login account that owns the persona. Same-account personas share it — ignore unless you truly need account-level grouping. |
| `zeal_persona_name` | `Connect Integration` | Display name (refreshed every login). |
| `zeal_persona_identity` | `ada@example.com` | Freeform identity string, or `null`. |
| `zeal_persona_identity_verified` | `true` | `true` only if the identity was verified via email OTP. **Trust only verified identities**; unverified ones are informational. |

Rules:

- **`sub` is the contract.** Key all authorization and per-product data
  on it. Account authenticates; **your product owns authorization**.
- Claims are refreshed at every login — re-sync your mirror rows on each
  callback (Connect does this in one `SyncClaims` upsert).
- One login account can hold many personas; the persona (not the
  account) is what your product sees. Never assume one human = one `sub`.

## 7. Refresh-token rotation

Refresh tokens are opaque, **30 days**, and **rotated on every use**: the
`/token` response carries a new one and the old becomes invalid. Reusing
a rotated token is treated as theft — the whole family (the refresh
token and every access token derived from it) is revoked and the client
gets `invalid_grant`.

Client obligations:

- Store the refresh token with the session it belongs to; **replace it
  atomically** on each refresh response.
- On `invalid_grant`: drop the stored tokens and send the user through
  `/auth/login` again — do not retry with the same token.
- Refresh only when you need to (access token expired / about to).
  Serial refreshes from many tabs should funnel through one in-flight
  request (mutex/single-flight) or one tab will rotate the family out
  from under the others.

Zeal Connect's MVP never refreshes (its session cookie is the only API
credential); it is documented here for products that keep API sessions.

## 8. Gotchas from the first integration (Connect)

- **Redirect status is 303, not 302.** Account's own redirects
  (`/login`, `/persona/select`) are `302 Found`; the final
  code-carrying redirect from `/authorize` is issued by fosite and is
  `303 See Other`. Accept any 3xx when following the flow programmatically.
- **SvelteKit `goto()` cannot target `/authorize`.** It's a Go
  front-door route, not a SvelteKit route — `goto('/authorize?…')` throws
  `navigation_route_missing` and silently strands the user on `/login`.
  Use `window.location.assign(next)` for post-login redirects into OIDC
  routes (Account's own login page had exactly this bug; fixed in
  phase 4). Same applies in your product if its shell is an SPA.
- **Login rate limit is 10 / 15 min per IP.** Integration tests should
  treat `429` as "retry later", not as failure.
- **Stay on one browser host.** The redirect chain must land back on the
  exact origin your cookies were set on (dev: `localhost` vs `127.0.0.1`
  are different origins — pick one and use it everywhere).
- **Wait for SPA hydration** before clicking submit in browser tests, or
  the form falls back to a native GET (in Account's case, with the
  password in the query string).

## 9. Verifying your integration

1. **Automated (required):** an integration test that drives the full
   loop against a running Account — login → authorize → picker → code →
   `/token` (Basic + PKCE) → JWKS-validated ID token with `sub` = persona
   ID → session → logout. Copy the structure of Connect's
   `phase1_test.go` (`TestPhase1LoginKeyPublish`); keep a fresh-cookie
   jar per journey and treat `429` as a skip.
2. **Manual smoke:** run both front doors locally (Account `just dev`,
   your product `just dev`), open your product in a **fresh browser
   profile**, and click through: sign-in → Account login → persona picker
   → back in your product as the chosen persona.
3. Run it again with an account that already picked a persona — the
   picker must **not** appear (default per client), and
   `prompt=select_account` must force it.

## 10. Local topology (dev)

| Service | Address |
|---|---|
| Account front door (Go, proxies portal) | `http://localhost:8082` |
| Account Vite dev server (proxied) | `http://127.0.0.1:5273` |
| Account issuer (`OIDC_ISSUER`) | `http://localhost:8082` |
| Your product's issuer env | `OIDC_ISSUER=http://localhost:8082` |

Point your product's `OIDC_ISSUER` at Account's front door, register your
redirect URI (§1), put the secret in your `.env`, and start with §9.
