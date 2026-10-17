-- 1. Alter phone_numbers table to allow global unassigned inventory
ALTER TABLE phone_numbers ALTER COLUMN workspace_id DROP NOT NULL;
ALTER TABLE phone_numbers DROP CONSTRAINT IF EXISTS phone_numbers_status_check;
ALTER TABLE phone_numbers ADD CONSTRAINT phone_numbers_status_check 
    CHECK (status IN ('active', 'released', 'pending_verification', 'available', 'unassigned'));

-- 2. Seed Unassigned / Available Phone Numbers pool directly in PostgreSQL
INSERT INTO phone_numbers (id, workspace_id, phone_number, type, provider, status, monthly_cost_usd, created_at)
VALUES
    (gen_random_uuid(), NULL, '+1 (415) 301-4491', 'local', 'twilio', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+1 (415) 792-8820', 'local', 'telnyx', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+1 (800) 240-9911', 'toll_free', 'twilio', 'available', 2.00, NOW()),
    (gen_random_uuid(), NULL, '+1 (800) 650-3344', 'toll_free', 'telnyx', 'available', 2.00, NOW()),
    (gen_random_uuid(), NULL, '+1 (212) 555-8392', 'local', 'bandwidth', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+1 (650) 412-9901', 'local', 'twilio', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+44 20 7183 4590', 'local', 'twilio', 'available', 1.50, NOW()),
    (gen_random_uuid(), NULL, '+44 800 098 8122', 'toll_free', 'telnyx', 'available', 2.50, NOW()),
    (gen_random_uuid(), NULL, '+91 80 6192 4400', 'local', 'bandwidth', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+91 22 4893 2100', 'local', 'bandwidth', 'available', 1.00, NOW()),
    (gen_random_uuid(), NULL, '+91 1800 200 4567', 'toll_free', 'plivo', 'available', 2.00, NOW())
ON CONFLICT (phone_number) DO NOTHING;

-- 3. Also assign a couple of allocated numbers for Clear Trail Technology workspace so the user immediately sees allocated numbers in the active workspace
DO $$
DECLARE
    ct_ws_id UUID := '55433f1b-2cf5-4fe4-a50e-1a0f31dcb4b6';
    maya_agent_id UUID;
    david_agent_id UUID;
BEGIN
    SELECT id INTO maya_agent_id FROM agents WHERE workspace_id = ct_ws_id LIMIT 1;
    IF maya_agent_id IS NULL THEN
        maya_agent_id := gen_random_uuid();
        david_agent_id := gen_random_uuid();
        INSERT INTO agents (id, workspace_id, name, role, description, system_prompt, voice_id, voice_provider, voice_name, language, model, status, active_version_num, assigned_phone, created_at, updated_at)
        VALUES
        (maya_agent_id, ct_ws_id, 'Maya (Luxury Hospitality)', 'Concierge Specialist', 'Handles luxury guest booking and VIP inquiries', 'You are Maya, hospitality concierge.', 'eleven_multilingual_v2', 'elevenlabs', 'Rachel', 'en-US', 'gpt-4o', 'active', 1, '+1 (415) 890-2341', NOW(), NOW()),
        (david_agent_id, ct_ws_id, 'David (Clinical Triage)', 'Clinical Triage Nurse', 'Pre-screens patient symptoms and appointment booking', 'You are David, patient triage specialist.', 'eleven_multilingual_v2', 'elevenlabs', 'Adam', 'en-US', 'gpt-4o', 'active', 1, '+1 (800) 450-8912', NOW(), NOW())
        ON CONFLICT (id) DO NOTHING;
    END IF;

    -- Allocate 2 numbers to Clear Trail Technology
    INSERT INTO phone_numbers (id, workspace_id, phone_number, type, provider, carrier_sid, assigned_agent_id, status, monthly_cost_usd, created_at)
    VALUES
    (gen_random_uuid(), ct_ws_id, '+1 (415) 880-9921', 'local', 'twilio', 'PN_CT_01', maya_agent_id, 'active', 1.00, NOW() - INTERVAL '1 day'),
    (gen_random_uuid(), ct_ws_id, '+1 (800) 330-8811', 'toll_free', 'telnyx', 'PN_CT_02', david_agent_id, 'active', 2.00, NOW() - INTERVAL '2 days'),
    (gen_random_uuid(), ct_ws_id, '+91 80 5500 2211', 'local', 'bandwidth', 'PN_CT_03', NULL, 'unassigned', 1.00, NOW())
    ON CONFLICT (phone_number) DO NOTHING;
END $$;
