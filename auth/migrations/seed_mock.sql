INSERT INTO users (id, kratos_id, email, display_name, phone_number, is_active, created_at, updated_at) 
VALUES ('11111111-1111-1111-1111-111111111111', 'test-kratos-id-001', 'testuser@samvada.ai', 'Test Admin', '+1234567890', true, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_profiles (id, user_id, first_name, last_name, account_type, industries, onboarding_completed, created_at, updated_at)
VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'Test', 'Admin', 'individual', ARRAY['Technology','Finance'], true, NOW(), NOW())
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO organizations (id, owner_id, name, slug, created_at, updated_at)
VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'Acme Corp', 'acme-corp', NOW(), NOW())
ON CONFLICT (slug) DO NOTHING;

INSERT INTO workspaces (id, owner_id, org_id, name, slug, is_default, created_at, updated_at)
VALUES ('44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', 'Default Workspace', 'default-ws', true, NOW(), NOW())
ON CONFLICT (org_id, slug) DO NOTHING;
