-- Demo / test data: never required by the app — safe to skip in
-- production. Idempotent (ON CONFLICT DO NOTHING).
--
-- Local dev + integration tests:  just seed-demo
-- Everything at once:             just seed

\set ON_ERROR_STOP on
BEGIN;

-- Public SPA test client (PKCE-only), moved from migration 000005.
-- Used by the OIDC integration tests (phase 3) and manual experiments
-- against a local callback on :9999.

INSERT INTO oauth_clients (id, name, scopes, is_public)
VALUES ('zeal-test', 'Zeal Test App', '{openid,profile,email}', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO oauth_client_redirect_uris (client_id, redirect_uri)
VALUES ('zeal-test', 'http://127.0.0.1:9999/callback'),
       ('zeal-test', 'http://localhost:9999/callback')
ON CONFLICT (client_id, redirect_uri) DO NOTHING;

-- Demo login account for the portal (no persona yet — create one after
-- logging in). argon2id hash (password.Hash parameters); the password
-- itself is deliberately kept out of version control — recover it by
-- rotating the demo account's password through the portal. DEMO
-- CREDENTIAL ONLY: never seed production with it. On a DB where the
-- username already exists the row is left untouched.
INSERT INTO accounts (username, password_hash)
VALUES ('ridzeal', '$argon2id$v=19$m=65536,t=3,p=4$62h3iH7D02Qsf4X4bTglUQ$nawQ4c17FZBTY+I9esz515J/LCKubnh3Ussy2wJWdDE')
ON CONFLICT ((lower(username))) DO NOTHING;

COMMIT;
