-- =============================================================================
-- Migration: 000011_add_performance_indexes.up.sql
-- Description: Composite and covering indexes for hot query paths (Phase 6.3)
-- =============================================================================

-- 1. Agents: Fast workspace listing and status filtering
CREATE INDEX IF NOT EXISTS idx_agents_workspace_status ON agents (workspace_id, status);

-- 2. Campaigns: High-frequency scheduler queries for active/scheduled runs
CREATE INDEX IF NOT EXISTS idx_campaigns_ws_status_scheduled ON campaigns (workspace_id, status, scheduled_at);

-- 3. Campaign Contacts: Outbound dialer queue pop operations (pending contacts)
CREATE INDEX IF NOT EXISTS idx_campaign_contacts_queue ON campaign_contacts (campaign_id, status, attempts);

-- 4. Billing Ledger: Reverse chronological transaction history pagination
CREATE INDEX IF NOT EXISTS idx_wallet_tx_wallet_created ON wallet_transactions (wallet_id, created_at DESC);

-- 5. Invoices: Org invoice history and status filtering
CREATE INDEX IF NOT EXISTS idx_invoices_org_status_created ON invoices (org_id, status, created_at DESC);

-- 6. Telephony: Fast phone number allocation and status check
CREATE INDEX IF NOT EXISTS idx_phone_numbers_org_assigned ON phone_numbers (org_id, is_assigned);

-- 7. Audit Logs: Enterprise compliance search and timeline queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_created ON audit_logs (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_action ON audit_logs (org_id, action, created_at DESC);

-- 8. Webhooks: Fast dispatch lookup for active webhook subscriptions
CREATE INDEX IF NOT EXISTS idx_webhooks_org_active ON webhooks (org_id, is_active);

-- 9. Knowledge Base Documents: Fast retrieval by workspace
CREATE INDEX IF NOT EXISTS idx_kb_workspace_created ON knowledge_bases (workspace_id, created_at DESC);
