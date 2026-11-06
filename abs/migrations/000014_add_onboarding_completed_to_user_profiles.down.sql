-- =============================================================================
-- Migration: 000014_add_onboarding_completed_to_user_profiles.down.sql
-- =============================================================================

ALTER TABLE user_profiles 
DROP COLUMN IF EXISTS onboarding_completed;
