CREATE TABLE IF NOT EXISTS pending_auth (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    otp_hash TEXT NOT NULL,
    purpose TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    metadata TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_pending_auth_email_purpose UNIQUE (email, purpose)
);

CREATE INDEX IF NOT EXISTS idx_pending_auth_email ON pending_auth(email);
CREATE INDEX IF NOT EXISTS idx_pending_auth_expires_at ON pending_auth(expires_at);
