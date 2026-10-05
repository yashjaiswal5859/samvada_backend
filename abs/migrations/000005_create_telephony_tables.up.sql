-- =============================================================================
-- Migration: 000005_create_telephony_tables.up.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS phone_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    phone_number VARCHAR(50) UNIQUE NOT NULL,
    type VARCHAR(50) DEFAULT 'toll_free' CHECK (type IN ('toll_free', 'local', 'mobile')),
    provider VARCHAR(50) NOT NULL CHECK (provider IN ('twilio', 'plivo', 'sip_trunk')),
    carrier_sid VARCHAR(255),
    assigned_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    status VARCHAR(50) DEFAULT 'active' CHECK (status IN ('active', 'released', 'pending_verification')),
    monthly_cost_usd NUMERIC(10, 4) DEFAULT 1.00,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_phone_numbers_workspace ON phone_numbers(workspace_id);
CREATE INDEX IF NOT EXISTS idx_phone_numbers_number ON phone_numbers(phone_number);
CREATE INDEX IF NOT EXISTS idx_phone_numbers_assigned_agent ON phone_numbers(assigned_agent_id);
