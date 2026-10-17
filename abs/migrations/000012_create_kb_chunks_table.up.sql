-- =============================================================================
-- Migration: 000012_create_kb_chunks_table.up.sql
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kb_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    content TEXT NOT NULL,
    token_count INT DEFAULT 0,
    metadata JSONB DEFAULT '{}'::jsonb,
    embedding vector(384),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_workspace ON kb_chunks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_kb ON kb_chunks(kb_id);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_document ON kb_chunks(document_id);

-- HNSW Vector Index for sub-millisecond similarity search
CREATE INDEX IF NOT EXISTS idx_kb_chunks_embedding_hnsw 
ON kb_chunks USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
