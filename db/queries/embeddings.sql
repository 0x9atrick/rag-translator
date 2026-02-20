-- name: InsertEmbeddingWithVector :exec
INSERT INTO embeddings (hash, source, context, file_path, embedding)
VALUES ($1, $2, $3, $4, $5::vector)
ON CONFLICT (hash) DO NOTHING;

-- name: SearchSimilarEmbeddings :many
SELECT source, context, (1 - (embedding <=> $1::vector))::float8 AS similarity
FROM embeddings
WHERE embedding IS NOT NULL
ORDER BY embedding <=> $1::vector
LIMIT $2;

-- name: GetEmbeddingByHash :one
SELECT id, hash, source, context, file_path, created_at
FROM embeddings
WHERE hash = $1;
