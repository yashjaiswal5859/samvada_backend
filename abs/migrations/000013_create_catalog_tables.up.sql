-- =============================================================================
-- Migration: 000013_create_catalog_tables.up.sql
-- Description: AI Catalog (STT, TTS, LLM models) and available resource registry
-- =============================================================================

CREATE TABLE IF NOT EXISTS ai_models (
    id VARCHAR(100) PRIMARY KEY,
    category VARCHAR(50) NOT NULL CHECK (category IN ('stt', 'tts', 'llm')),
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(100) NOT NULL,
    note TEXT,
    is_recommended BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ai_models_category ON ai_models(category);
CREATE INDEX IF NOT EXISTS idx_ai_models_active ON ai_models(is_active);

-- Initial seed data for STT, TTS, and LLM models
INSERT INTO ai_models (id, category, name, provider, note, is_recommended, is_active, metadata)
VALUES
    -- STT Models
    ('deepgram', 'stt', 'Deepgram Nova-2', 'deepgram', 'Recommended · streaming transcription with keyterms support', true, true, '{"latency_ms": 65, "supports_keyterms": true}'),
    ('google-speech', 'stt', 'Google Cloud Speech-to-Text', 'google', 'Enterprise multilingual speech recognition', false, true, '{"latency_ms": 110}'),
    ('azure-speech', 'stt', 'Azure AI Speech', 'azure', 'High accuracy speech-to-text through Microsoft Azure', false, true, '{"latency_ms": 125}'),
    ('whisper-large-v3', 'stt', 'OpenAI Whisper Large v3', 'openai', 'State-of-the-art multilingual transcription', false, true, '{"latency_ms": 250}'),

    -- LLM Models
    ('gpt-4o-mini', 'llm', 'GPT-4o Mini', 'openai', 'Recommended · ultra low latency & optimal cost for voice', true, true, '{"provider": "openai", "ttft_ms": 120}'),
    ('claude-3-5-haiku', 'llm', 'Claude 3.5 Haiku', 'anthropic', 'Fast, concise model with superior reasoning', false, true, '{"provider": "anthropic", "ttft_ms": 140}'),
    ('gemini-1-5-flash', 'llm', 'Gemini 1.5 Flash', 'google', 'Multilingual and high token throughput', false, true, '{"provider": "google", "ttft_ms": 115}'),
    ('gpt-4o', 'llm', 'GPT-4o', 'openai', 'Highest capability model for complex multi-step reasoning', false, true, '{"provider": "openai", "ttft_ms": 220}'),

    -- TTS Models
    ('fish-audio', 'tts', 'Fish Audio', 'fish-audio', 'Recommended · expressive low-latency neural speech generation', true, true, '{"latency_ms": 90, "expressive": true}'),
    ('elevenlabs', 'tts', 'ElevenLabs Multilingual v2', 'elevenlabs', 'Hyper-realistic human voices with broad language support', false, true, '{"latency_ms": 140}'),
    ('cartesia', 'tts', 'Cartesia Sonic', 'cartesia', 'Ultra-low latency conversational voice engine', false, true, '{"latency_ms": 80}'),
    ('playht', 'tts', 'PlayHT 2.0', 'playht', 'Expressive multilingual conversational speech', false, true, '{"latency_ms": 160}'),
    ('azure-neural', 'tts', 'Azure Neural Voice', 'azure', 'Telecom-grade neural voice synthesis', false, true, '{"latency_ms": 130}')
ON CONFLICT (id) DO NOTHING;
