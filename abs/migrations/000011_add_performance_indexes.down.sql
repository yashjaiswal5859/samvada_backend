-- =============================================================================
-- Migration: 000011_add_performance_indexes.down.sql
-- Description: Drop performance indexes
-- =============================================================================

DROP INDEX IF EXISTS idx_kb_workspace_created;
DROP INDEX IF EXISTS idx_webhooks_ws_active;
DROP INDEX IF EXISTS idx_audit_logs_org_action;
DROP INDEX IF EXISTS idx_audit_logs_org_created;
DROP INDEX IF EXISTS idx_phone_numbers_ws_status;
DROP INDEX IF EXISTS idx_invoices_org_status_created;
DROP INDEX IF EXISTS idx_wallet_tx_wallet_created;
DROP INDEX IF EXISTS idx_contacts_audience_status;
DROP INDEX IF EXISTS idx_campaigns_ws_status_scheduled;
DROP INDEX IF EXISTS idx_agents_workspace_status;
