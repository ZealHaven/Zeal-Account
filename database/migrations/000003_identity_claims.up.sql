-- Identity claims + verification (claim-then-verify model).
-- Multiple accounts may hold pending claims of the same identity; the
-- first to verify wins it exclusively. Losing claims are evicted in the
-- same transaction as the verification, and their accounts notified.
CREATE TABLE account_identities (
    identity     text NOT NULL,
    account_id   uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'verified')),
    verified_at  timestamptz,
    otp_hash     text,
    otp_expires_at timestamptz,
    otp_attempts   int NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (identity, account_id)
);

-- At most one verified owner per identity.
CREATE UNIQUE INDEX account_identities_verified_idx
    ON account_identities (identity) WHERE status = 'verified';

CREATE INDEX account_identities_account_id_idx ON account_identities (account_id);
