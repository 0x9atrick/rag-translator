CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS translation_cache (
    hash       TEXT PRIMARY KEY,
    source     TEXT NOT NULL,
    translated TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_translation_cache_hash ON translation_cache (hash);

CREATE TABLE IF NOT EXISTS seed_translations (
    hash            TEXT PRIMARY KEY,
    source_text     TEXT NOT NULL,
    translated_text TEXT NOT NULL,
    file            TEXT NOT NULL DEFAULT '',
    function_name   TEXT NOT NULL DEFAULT '',
    entity_type     TEXT NOT NULL DEFAULT 'general',
    is_seed         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_seed_translations_entity ON seed_translations (entity_type);
CREATE INDEX IF NOT EXISTS idx_seed_translations_file ON seed_translations (file);

CREATE TABLE IF NOT EXISTS embeddings (
    id         SERIAL PRIMARY KEY,
    hash       TEXT UNIQUE NOT NULL,
    source     TEXT NOT NULL,
    context    TEXT NOT NULL DEFAULT '',
    file_path  TEXT NOT NULL DEFAULT '',
    embedding  vector(1024),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_embeddings_hash ON embeddings (hash);
