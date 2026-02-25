package internal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration for the ingestion service.
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	KafkaBrokers []string
	KafkaTopic   string

	HyperWSURL string

	InfluencerSetKey string
}

// envOrDefault returns the value of an env var or a default.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) (int, error) {
	if raw := os.Getenv(key); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		return val, nil
	}

	return def, nil
}

func envCSVOrDefault(key, def string) []string {
	raw := envOrDefault(key, def)
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// LoadConfig loads configuration from environment variables.
// This matches the TECHNICAL_SPECS at a coarse level and can be
// refined as shared config libraries are introduced.
func LoadConfig() (Config, error) {
	redisDB, err := envIntOrDefault("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		RedisAddr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,

		KafkaBrokers: envCSVOrDefault("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:   envOrDefault("KAFKA_TOPIC_INFLUENCER_SIGNALS", "influencer_signals"),

		HyperWSURL: envOrDefault("HYPERLIQUID_WS_URL", "wss://api.hyperliquid.xyz/ws"),

		InfluencerSetKey: envOrDefault("INFLUENCER_SET_KEY", "ingestion:influencers:primary"),
	}

	return cfg, nil
}
