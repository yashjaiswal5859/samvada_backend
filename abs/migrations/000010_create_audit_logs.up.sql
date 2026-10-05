-- =============================================================================
-- Migration: 000010_create_audit_logs.up.sql
-- Description: Audit log table for all administrative actions (Phase 6.4)
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID,
    org_id UUID,
    user_id UUID NOT NULL,
    action VARCHAR(64) NOT NULL, -- e.g. "agent.publish", "agent.rollback", "apikey.create", "apikey.revoke", "webhook.create", "webhook.delete", "member.invite", "member.role_change"
    resource_type VARCHAR(64) NOT NULL, -- e.g. "agent", "api_key", "webhook", "member", "billing"
    resource_id VARCHAR(128) NOT NULL DEFAULT '',
    ip_address VARCHAR(45) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_workspace_id ON audit_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
