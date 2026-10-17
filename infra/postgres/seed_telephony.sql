-- Ensure provider check constraint allows all modern carriers
ALTER TABLE phone_numbers DROP CONSTRAINT IF EXISTS phone_numbers_provider_check;
ALTER TABLE phone_numbers ADD CONSTRAINT phone_numbers_provider_check CHECK (provider IN ('twilio', 'plivo', 'sip_trunk', 'telnyx', 'bandwidth'));

DO $$
DECLARE
    ws RECORD;
    agent1_id UUID;
    agent2_id UUID;
    agent3_id UUID;
    num_count INT;
BEGIN
    FOR ws IN SELECT id FROM workspaces LOOP
        -- Check if agents exist for this workspace
        SELECT id INTO agent1_id FROM agents WHERE workspace_id = ws.id AND name LIKE 'Maya%' LIMIT 1;
        IF agent1_id IS NULL THEN
            agent1_id := gen_random_uuid();
            agent2_id := gen_random_uuid();
            agent3_id := gen_random_uuid();

            INSERT INTO agents (id, workspace_id, name, role, description, system_prompt, voice_id, voice_provider, voice_name, language, model, status, active_version_num, assigned_phone, created_at, updated_at)
            VALUES
            (agent1_id, ws.id, 'Maya (Luxury Hospitality)', 'Concierge Specialist', 'Handles luxury guest booking and VIP inquiries', 'You are Maya, hospitality concierge.', 'eleven_multilingual_v2', 'elevenlabs', 'Rachel', 'en-US', 'gpt-4o', 'active', 1, '+1 (415) 890-2341', NOW(), NOW()),
            (agent2_id, ws.id, 'David (Clinical Triage)', 'Clinical Triage Nurse', 'Pre-screens patient symptoms and appointment booking', 'You are David, patient triage specialist.', 'eleven_multilingual_v2', 'elevenlabs', 'Adam', 'en-US', 'gpt-4o', 'active', 1, '+1 (800) 450-8912', NOW(), NOW()),
            (agent3_id, ws.id, 'Liam (Enterprise SDR)', 'B2B Inbound Qualifier', 'Qualifies enterprise inbound sales leads', 'You are Liam, B2B sales rep.', 'eleven_multilingual_v2', 'elevenlabs', 'Liam', 'en-US', 'gpt-4o', 'active', 1, '+44 20 7946 0192', NOW(), NOW())
            ON CONFLICT (id) DO NOTHING;
        ELSE
            SELECT id INTO agent2_id FROM agents WHERE workspace_id = ws.id AND name LIKE 'David%' LIMIT 1;
            SELECT id INTO agent3_id FROM agents WHERE workspace_id = ws.id AND name LIKE 'Liam%' LIMIT 1;
        END IF;

        -- Check if phone numbers exist for this workspace
        SELECT count(*) INTO num_count FROM phone_numbers WHERE workspace_id = ws.id;
        IF num_count = 0 THEN
            INSERT INTO phone_numbers (id, workspace_id, phone_number, type, provider, carrier_sid, assigned_agent_id, status, monthly_cost_usd, created_at)
            VALUES
            (gen_random_uuid(), ws.id, '+1 (415) 890-2341', 'local', 'twilio', 'PN_TWILIO_415_01', agent1_id, 'active', 1.00, NOW() - INTERVAL '3 days'),
            (gen_random_uuid(), ws.id, '+1 (800) 450-8912', 'toll_free', 'telnyx', 'PN_TELNYX_800_02', agent2_id, 'active', 2.00, NOW() - INTERVAL '2 days'),
            (gen_random_uuid(), ws.id, '+44 20 7946 0192', 'local', 'twilio', 'PN_TWILIO_LON_03', agent3_id, 'active', 1.50, NOW() - INTERVAL '1 day'),
            (gen_random_uuid(), ws.id, '+91 80 4000 1111', 'local', 'bandwidth', 'PN_BANDWIDTH_BLR_04', NULL, 'active', 1.00, NOW())
            ON CONFLICT (phone_number) DO NOTHING;
        END IF;
    END LOOP;
END $$;
