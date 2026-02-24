package internal

import (
	"context"
	"encoding/json"
	"fmt"

	redis "github.com/redis/go-redis/v9"
)

// InfluencerStore abstracts reading influencer configuration from Redis.
type InfluencerStore struct {
	client *redis.Client
	key    string
}

func NewInfluencerStore(cfg Config) *InfluencerStore {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	return &InfluencerStore{
		client: client,
		key:    cfg.InfluencerSetKey,
	}
}

// ListInfluencers loads all influencers from the Redis set associated with the provided source.

func (s *InfluencerStore) ListInfluencers(ctx context.Context) ([]Influencer, error) {
	if s.key == "" {
		return nil, fmt.Errorf("influencer set key is not configured")
	}
	members, err := s.client.SMembers(ctx, s.key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis SMEMBERS %s: %w", s.key, err)
	}

	res := make([]Influencer, 0, len(members))
	for _, m := range members {
		var inf Influencer
		if err := json.Unmarshal([]byte(m), &inf); err != nil {
			// Skip malformed entries but continue.
			continue
		}
		res = append(res, inf)
	}
	return res, nil
}

func (s *InfluencerStore) Close() error {
	return s.client.Close()
}
