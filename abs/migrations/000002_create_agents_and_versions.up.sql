-- =============================================================================
-- Migration: 000002_create_agents_and_versions.up.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(255) NOT NULL,
    description TEXT,
    avatar_url TEXT,
    system_prompt TEXT NOT NULL,
    voice_id VARCHAR(100) NOT NULL,
    voice_provider VARCHAR(50) DEFAULT 'elevenlabs',
    voice_name VARCHAR(100) NOT NULL,
    language VARCHAR(50) DEFAULT 'en-US',
    model VARCHAR(100) DEFAULT 'gpt-4o',
    status VARCHAR(50) DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    active_version_num INT DEFAULT 1,
    assigned_phone VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version_num INT NOT NULL,
    system_prompt TEXT NOT NULL,
    model VARCHAR(100) NOT NULL,
    voice_id VARCHAR(100) NOT NULL,
    language VARCHAR(50) NOT NULL,
    tools_config JSONB DEFAULT '[]'::jsonb,
    knowledge_base_ids UUID[] DEFAULT '{}',
    published_by UUID NOT NULL,
    change_summary TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(agent_id, version_num)
);

CREATE INDEX IF NOT EXISTS idx_agents_workspace_id ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_versions_agent_id ON agent_versions(agent_id);
