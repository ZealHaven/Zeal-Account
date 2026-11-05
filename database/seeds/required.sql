-- Required (master) data: rows the app cannot work without.
--
-- Migrations are schema-only; this file owns the master rows. It is
-- idempotent (ON CONFLICT DO NOTHING), so run it after every
-- `migrate-up` and on every deploy:  just seed-required

\set ON_ERROR_STOP on
BEGIN;

-- First-party confidential client for Zeal Connect (moved from
-- migration 000006). secret_hash is bcrypt — fosite's default
-- GetSecretsHasher compares with bcrypt.CompareHashAndPassword — NOT
-- the argon2 hash used for user passwords. The plaintext secret lives
-- only in Connect's .env; never commit it. Redirect URIs are matched
-- exactly by fosite (dev + prod).

INSERT INTO oauth_clients (id, secret_hash, name, scopes, is_public)
VALUES (
    'connect',
    '$2a$12$YiH10WxQ2G0i0vKxZjyjvOfTMNIdCh0o2H2NHIPw50IO0Zq/GbAse',
    'Zeal Connect',
    '{openid,profile}',
    false
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO oauth_client_redirect_uris (client_id, redirect_uri)
VALUES ('connect', 'http://localhost:8190/auth/callback'),
       ('connect', 'https://connect.zealhaven.id/auth/callback')
ON CONFLICT (client_id, redirect_uri) DO NOTHING;

COMMIT;
