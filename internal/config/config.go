package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

type Config struct {
	AnthropicAPIKey       string
	DatabaseURL           string
	Neo4jURI              string
	Neo4jUser             string
	Neo4jPassword         string
	WorkerCount           int
	BatchSize             int
	MaxConcurrentAPICalls int
	EmbeddingAPIKey       string
	EmbeddingModel        string
	EmbeddingBaseURL      string
	EmbeddingDimensions   int
	TranslationModel      string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found, using environment variables")
	}

	return &Config{
		AnthropicAPIKey:       getEnv("ANTHROPIC_API_KEY", ""),
		DatabaseURL:           getEnv("DATABASE_URL", "postgres://localhost:5432/rag_translator?sslmode=disable"),
		Neo4jURI:              getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:             getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword:         getEnv("NEO4J_PASSWORD", "password"),
		WorkerCount:           getEnvInt("WORKER_COUNT", 8),
		BatchSize:             getEnvInt("BATCH_SIZE", 10),
		MaxConcurrentAPICalls: getEnvInt("MAX_CONCURRENT_API_CALLS", 5),
		EmbeddingAPIKey:       getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingModel:        getEnv("EMBEDDING_MODEL", "Qwen/Qwen3-Embedding-0.6B"),
		EmbeddingBaseURL:      getEnv("EMBEDDING_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		EmbeddingDimensions:   getEnvInt("EMBEDDING_DIMENSIONS", 1024),
		TranslationModel:      getEnv("TRANSLATION_MODEL", "claude-sonnet-4-20250514"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
