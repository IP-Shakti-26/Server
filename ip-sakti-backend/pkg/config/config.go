package config

import (
	"errors"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// It is loaded exactly once at startup and passed via dependency injection.
type Config struct {
	Port            string
	DatabaseURL     string
	GeminiAPIKey    string   // GEMINI_API_KEY, required
	AnthropicAPIKey string   // ANTHROPIC_API_KEY, optional (future)
	QdrantHost      string
	QdrantPort      string   // HTTP REST port, default 6333
	QdrantGRPCPort  string   // gRPC port, default 6334
	QdrantAPIKey    string
	QdrantCollection string  // QDRANT_COLLECTION, default "ipsakti_chunks"
	OpenAIAPIKey    string   // OPENAI_API_KEY, optional (for embeddings, future)
	Env             string
	AllowedOrigins  []string
}

// Load reads the .env file (if present) and then reads environment variables.
// Returns an error if any required field is missing.
func Load() (*Config, error) {
	// Attempt to load .env — ignore error if file does not exist
	_ = godotenv.Load()

	cfg := &Config{
		Port:            getEnvOrDefault("SERVER_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		QdrantHost:      getEnvOrDefault("QDRANT_HOST", "localhost"),
		QdrantPort:      getEnvOrDefault("QDRANT_PORT", "6333"),
		QdrantGRPCPort:  getEnvOrDefault("QDRANT_GRPC_PORT", "6334"),
		QdrantAPIKey:    os.Getenv("QDRANT_API_KEY"),
		QdrantCollection: getEnvOrDefault("QDRANT_COLLECTION", "ipsakti_chunks"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		Env:             getEnvOrDefault("APP_ENV", "development"),
		AllowedOrigins:  parseOrigins(getEnvOrDefault("ALLOWED_ORIGINS", "*")),
	}

	var errs []string
	if cfg.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if cfg.GeminiAPIKey == "" {
		errs = append(errs, "GEMINI_API_KEY is required")
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
