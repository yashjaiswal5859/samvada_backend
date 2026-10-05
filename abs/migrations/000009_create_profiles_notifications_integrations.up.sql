-- =============================================================================
-- Migration: 000009_create_profiles_notifications_integrations.up.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id UUID PRIMARY KEY,
    first_name VARCHAR(100) NOT NULL DEFAULT '',
    last_name VARCHAR(100) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    phone VARCHAR(50) NOT NULL DEFAULT '',
    company VARCHAR(255) NOT NULL DEFAULT '',
    job_title VARCHAR(100) NOT NULL DEFAULT '',
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    language VARCHAR(50) NOT NULL DEFAULT 'en',
    avatar_url TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email_enabled BOOLEAN DEFAULT true,
    in_app_enabled BOOLEAN DEFAULT true,
    slack_enabled BOOLEAN DEFAULT false,
    slack_webhook_url TEXT DEFAULT '',
    call_alerts BOOLEAN DEFAULT true,
    billing_alerts BOOLEAN DEFAULT true,
    campaign_alerts BOOLEAN DEFAULT true,
    quiet_hours_start VARCHAR(10) DEFAULT '22:00',
    quiet_hours_end VARCHAR(10) DEFAULT '08:00',
    digest_frequency VARCHAR(50) DEFAULT 'daily',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_notification_pref_user_ws UNIQUE (user_id, workspace_id)
);

CREATE TABLE IF NOT EXISTS integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_connected BOOLEAN DEFAULT false,
    credentials_json JSONB DEFAULT '{}',
    config_json JSONB DEFAULT '{}',
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_integrations_ws_provider UNIQUE (workspace_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_notification_pref_ws ON notification_preferences(workspace_id);
CREATE INDEX IF NOT EXISTS idx_integrations_ws ON integrations(workspace_id);
