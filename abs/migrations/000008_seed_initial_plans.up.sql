-- =============================================================================
-- Migration: 000008_seed_initial_plans.up.sql
-- =============================================================================

INSERT INTO plans (id, tier, name, monthly_price_usd, monthly_price_inr, included_minutes, included_agents, included_workspaces, overage_per_minute_usd, overage_per_minute_inr, features, is_active)
VALUES
    (
        '10000000-0000-0000-0000-000000000001',
        'free',
        'Free Trial',
        0.00,
        0.00,
        100,
        1,
        1,
        0.0600,
        5.0000,
        '{"models": ["gpt-4o-mini", "gemini-1.5-flash"], "telephony": "web_only", "support": "community", "custom_voices": false, "concurrency_limit": 1}'::jsonb,
        true
    ),
    (
        '10000000-0000-0000-0000-000000000002',
        'starter',
        'Starter',
        49.00,
        3999.00,
        1000,
        3,
        2,
        0.0500,
        4.0000,
        '{"models": ["gpt-4o", "gemini-1.5-flash", "claude-3-haiku"], "telephony": "inbound_only", "support": "email", "custom_voices": true, "concurrency_limit": 5}'::jsonb,
        true
    ),
    (
        '10000000-0000-0000-0000-000000000003',
        'pro',
        'Growth / Pro',
        199.00,
        15999.00,
        5000,
        10,
        5,
        0.0400,
        3.2000,
        '{"models": ["gpt-4o", "claude-3-5-sonnet", "elevenlabs_multilingual"], "telephony": "inbound_outbound", "support": "priority", "custom_voices": true, "campaigns": true, "concurrency_limit": 25}'::jsonb,
        true
    ),
    (
        '10000000-0000-0000-0000-000000000004',
        'enterprise',
        'Enterprise Unicorn',
        799.00,
        64999.00,
        25000,
        100,
        50,
        0.0300,
        2.4000,
        '{"models": ["all"], "telephony": "full_sip_trunking", "support": "dedicated_slack", "custom_voices": true, "campaigns": true, "sla": "99.99%", "concurrency_limit": 250}'::jsonb,
        true
    )
ON CONFLICT (tier) DO UPDATE SET
    name = EXCLUDED.name,
    monthly_price_usd = EXCLUDED.monthly_price_usd,
    monthly_price_inr = EXCLUDED.monthly_price_inr,
    included_minutes = EXCLUDED.included_minutes,
    included_agents = EXCLUDED.included_agents,
    included_workspaces = EXCLUDED.included_workspaces,
    overage_per_minute_usd = EXCLUDED.overage_per_minute_usd,
    overage_per_minute_inr = EXCLUDED.overage_per_minute_inr,
    features = EXCLUDED.features,
    is_active = EXCLUDED.is_active;
