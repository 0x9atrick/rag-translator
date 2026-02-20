-- name: UpsertSeedTranslation :execresult
INSERT INTO seed_translations (hash, source_text, translated_text, file, function_name, entity_type)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (hash) DO UPDATE SET
    translated_text = EXCLUDED.translated_text,
    file = EXCLUDED.file,
    function_name = EXCLUDED.function_name,
    entity_type = EXCLUDED.entity_type,
    updated_at = NOW();

-- name: GetAllSeedTranslations :many
SELECT hash, source_text, translated_text, file, function_name, entity_type
FROM seed_translations
WHERE is_seed = TRUE
ORDER BY created_at;

-- name: GetSeedTranslationsByEntityType :many
SELECT hash, source_text, translated_text, file, function_name, entity_type
FROM seed_translations
WHERE is_seed = TRUE AND entity_type = $1
ORDER BY created_at;
