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

// LoadConfig loads configuration from environment variables.
// This matches the TECHNICAL_SPECS at a coarse level and can be
// refined as shared config libraries are introduced.
func LoadConfig() (Config, error) {
	redisDBStr := envOrDefault("REDIS_DB", "0")
	redisDB, err := strconv.Atoi(redisDBStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	brokersStr := envOrDefault("KAFKA_BROKERS", "localhost:9092")
	brokers := strings.Split(brokersStr, ",")

	cfg := Config{
		RedisAddr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,

		KafkaBrokers: brokers,
		KafkaTopic:   envOrDefault("KAFKA_TOPIC_INFLUENCER_SIGNALS", "influencer_signals"),

		HyperWSURL: envOrDefault("HYPERLIQUID_WS_URL", "wss://api.hyperliquid.xyz/ws"),

		InfluencerSetKey: envOrDefault("INFLUENCER_SET_KEY", "ingestion:influencers:primary"),
	}

	return cfg, nil
}
