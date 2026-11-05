-- Recovery codes: the forgot-password mechanism (no email on file at
-- registration). Shown once, hashed at rest, single-use.
CREATE TABLE recovery_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    code_hash  text NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX recovery_codes_account_id_idx ON recovery_codes (account_id);
