-- Accounts: the login credential (username + password). Nothing else.
CREATE TABLE accounts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Case-insensitive unique usernames.
CREATE UNIQUE INDEX accounts_username_lower_idx ON accounts (lower(username));

-- Personas: identities presented to Zeal products. The id is the OIDC `sub`.
CREATE TABLE personas (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  uuid NOT NULL REFERENCES accounts (id) ON DELETE RESTRICT,
    name        text NOT NULL,
    identity    text,
    description text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX personas_account_id_idx ON personas (account_id);
