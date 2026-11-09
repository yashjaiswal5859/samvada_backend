CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    kratos_id TEXT,
    event_type TEXT NOT NULL,
    ip_address TEXT,
    user_agent TEXT,
    correlation_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_kratos_id ON audit_logs(kratos_id);
