-- =============================================================================
-- Migration: 000014_add_onboarding_completed_to_user_profiles.up.sql
-- =============================================================================

ALTER TABLE user_profiles 
ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN NOT NULL DEFAULT false;

-- Update starter plan included_minutes to 250 to match production plan spec
UPDATE plans 
SET included_minutes = 250 
WHERE tier = 'starter';
