-- 1. Organization & Workspace
INSERT INTO organizations (id, name, slug, billing_currency, billing_email, created_at, updated_at)
VALUES ('33333333-3333-3333-3333-333333333333', 'Acme Corp', 'acme-corp', 'USD', 'billing@acme.com', NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO workspaces (id, org_id, name, slug, is_default, created_at, updated_at)
VALUES ('44444444-4444-4444-4444-444444444444', '33333333-3333-3333-3333-333333333333', 'Default Workspace', 'default-ws', true, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- 2. Workspace Membership
INSERT INTO workspace_members (workspace_id, user_id, workspace_role, created_at)
VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', 'workspace_admin', NOW())
ON CONFLICT (workspace_id, user_id) DO UPDATE SET workspace_role = EXCLUDED.workspace_role;

-- 3. Voice Agents
INSERT INTO agents (id, workspace_id, name, role, description, system_prompt, voice_id, voice_provider, voice_name, language, model, status, active_version_num, assigned_phone, created_at, updated_at)
VALUES 
('aaaaaaaa-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 'Apex Concierge', 'Inbound Support Specialist', 'Handles Tier-1 customer queries and ticket intake', 'You are Apex Concierge, an empathetic customer care agent.', 'eleven_multilingual_v2', 'elevenlabs', 'Rachel', 'en-US', 'gpt-4o', 'active', 1, '+1 (800) 555-0192', NOW(), NOW()),
('aaaaaaaa-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'Swiggy One Pitcher', 'Outbound Sales Pitcher', 'Qualifies enterprise food program leads', 'You are Swiggy One sales specialist calling food managers.', 'eleven_multilingual_v2', 'elevenlabs', 'Aarav', 'hi-IN', 'gpt-4o', 'active', 1, '+1 (800) 555-0481', NOW(), NOW()),
('aaaaaaaa-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', 'Dr. Lal PathLabs', 'Diagnostic Booking Assistant', 'Schedules diagnostic appointments and health checkups', 'You are diagnostic lab assistant assisting patient booking.', 'eleven_multilingual_v2', 'elevenlabs', 'Priya', 'en-IN', 'gpt-4o', 'draft', 1, NULL, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status;

-- 4. Audience
INSERT INTO audiences (id, workspace_id, name, description, contact_count, created_at)
VALUES 
('bbbbbbbb-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 'Enterprise Growth Leads', 'High value leads from SaaS and FinTech verticals', 3, NOW())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- 5. Contacts
INSERT INTO contacts (id, audience_id, name, phone, email, company, tags, status, created_at)
VALUES 
('cccccccc-1111-1111-1111-111111111111', 'bbbbbbbb-1111-1111-1111-111111111111', 'Rahul Sharma', '+91 98765 43210', 'rahul@tata.com', 'Tata Consultancy Services', ARRAY['Hot Lead','High Priority'], 'active', NOW()),
('cccccccc-2222-2222-2222-222222222222', 'bbbbbbbb-1111-1111-1111-111111111111', 'Priya Patel', '+91 98234 56789', 'priya@infosys.com', 'Infosys Ltd', ARRAY['Interested','Follow Up'], 'active', NOW()),
('cccccccc-3333-3333-3333-333333333333', 'bbbbbbbb-1111-1111-1111-111111111111', 'Amit Verma', '+91 91234 56780', 'amit@hcl.com', 'HCL Technologies', ARRAY['Lead'], 'active', NOW())
ON CONFLICT (id) DO NOTHING;

-- 6. Campaigns
INSERT INTO campaigns (id, workspace_id, agent_id, audience_id, name, status, concurrent_calls_limit, total_contacts, completed_calls, successful_calls, failed_calls, created_at, updated_at)
VALUES 
('dddddddd-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 'aaaaaaaa-2222-2222-2222-222222222222', 'bbbbbbbb-1111-1111-1111-111111111111', 'Q3 Enterprise Renewal Drive', 'running', 15, 1250, 940, 712, 38, NOW(), NOW()),
('dddddddd-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', 'aaaaaaaa-1111-1111-1111-111111111111', 'bbbbbbbb-1111-1111-1111-111111111111', 'SaaS Onboarding Welcome Calls', 'scheduled', 10, 420, 0, 0, 0, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status;

-- 7. Telephony Numbers
INSERT INTO phone_numbers (id, workspace_id, phone_number, type, provider, assigned_agent_id, status, monthly_cost_usd, created_at)
VALUES 
('eeeeeeee-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', '+1 (800) 555-0192', 'toll_free', 'twilio', 'aaaaaaaa-1111-1111-1111-111111111111', 'active', 2.00, NOW()),
('eeeeeeee-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444', '+1 (800) 555-0481', 'toll_free', 'twilio', 'aaaaaaaa-2222-2222-2222-222222222222', 'active', 2.00, NOW()),
('eeeeeeee-3333-3333-3333-333333333333', '44444444-4444-4444-4444-444444444444', '+1 (800) 555-0922', 'local', 'plivo', NULL, 'active', 1.00, NOW())
ON CONFLICT (phone_number) DO UPDATE SET status = EXCLUDED.status;

-- 8. Organization Wallet
INSERT INTO organization_wallets (id, org_id, balance_usd, promotional_credit_usd, lifetime_credit_added_usd, lifetime_spend_usd, currency, is_frozen, updated_at)
VALUES 
('ffffffff-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 150.0000, 25.0000, 175.0000, 12.5000, 'USD', false, NOW())
ON CONFLICT (org_id) DO UPDATE SET balance_usd = EXCLUDED.balance_usd;
