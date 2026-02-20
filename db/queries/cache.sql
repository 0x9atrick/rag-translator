-- name: GetCachedTranslation :one
SELECT translated FROM translation_cache WHERE hash = $1;

-- name: UpsertCachedTranslation :exec
INSERT INTO translation_cache (hash, source, translated)
VALUES ($1, $2, $3)
ON CONFLICT (hash) DO UPDATE SET translated = EXCLUDED.translated;

-- name: ListAllCachedTranslations :many
SELECT hash, translated FROM translation_cache;
