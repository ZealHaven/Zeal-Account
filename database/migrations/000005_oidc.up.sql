-- OIDC provider storage (phase 3). Schema only — OAuth client rows live
-- in database/seeds/ (migrations never INSERT); dynamic registration is
-- post-MVP.

CREATE TABLE oauth_clients (
    id            text PRIMARY KEY,
    -- bcrypt/argon hash; NULL for public (SPA) clients using PKCE only
    secret_hash   text,
    name          text NOT NULL,
    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'disabled')),
    scopes        text[] NOT NULL DEFAULT '{openid,profile}',
    is_public     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_client_redirect_uris (
    client_id   text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    PRIMARY KEY (client_id, redirect_uri)
);

-- Fosite request sessions: authorize codes, access/refresh token sessions,
-- PKCE and OIDC request state, keyed JSONB blobs (proven in Zeal/Auth).
CREATE TABLE fosite_requests (
    key        text PRIMARY KEY,
    data       jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX fosite_requests_created_idx ON fosite_requests (created_at);

-- Access token bookkeeping for revocation.
CREATE TABLE oauth_access_tokens (
    signature  text PRIMARY KEY,
    request_id text NOT NULL,
    expires_at timestamptz,
    revoked_at timestamptz
);

CREATE INDEX oauth_access_tokens_request_idx ON oauth_access_tokens (request_id);

-- Replay guards for JWT client assertions (unused in MVP, required by
-- the fosite storage interface).
CREATE TABLE oauth_client_assertions (
    jti        text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);

-- RS256 signing keys. Private keys are AES-encrypted at rest; the
-- encryption key comes from the environment.
CREATE TABLE signing_keys (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kid                   text NOT NULL UNIQUE,
    algorithm             text NOT NULL DEFAULT 'RS256',
    public_key            text NOT NULL,
    private_key_encrypted text NOT NULL,
    status                text NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active', 'rotated')),
    created_at            timestamptz NOT NULL DEFAULT now()
);

-- Default persona per (account, client): the picker runs on the first
-- authorize for a pair, then the choice is remembered.
CREATE TABLE persona_defaults (
    account_id  uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    client_id   text NOT NULL,
    persona_id  uuid NOT NULL REFERENCES personas (id) ON DELETE CASCADE,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, client_id)
);

-- The `zeal-test` client (public SPA, PKCE-only) that used to be seeded
-- here moved to database/seeds/demo.sql.
