-- migrations/001_initial.sql
-- Initial schema for IP-SAKTI backend.
-- Run with: psql $DATABASE_URL -f migrations/001_initial.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS sessions (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    raw_description       TEXT        NOT NULL,
    clarification_answers JSONB       NOT NULL DEFAULT '{}',
    classification        JSONB,
    classification_done   BOOLEAN     NOT NULL DEFAULT FALSE,
    roadmap               JSONB,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast session lookups by ID (UUID PK already indexed, but explicit
-- for documentation clarity).
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions (created_at DESC);
