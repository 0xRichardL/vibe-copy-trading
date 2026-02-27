package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/0xRichardL/vibe-copy-trading/matcher/internal/domain"
	redis "github.com/redis/go-redis/v9"
)

// SubscriptionStore abstracts reading subscription configuration from Redis.
type SubscriptionStore struct {
	client *redis.Client
	key    string
}

// NewSubscriptionStore creates a new SubscriptionStore backed by Redis.
func NewSubscriptionStore(client *redis.Client, key string) *SubscriptionStore {
	return &SubscriptionStore{client: client, key: key}
}

// Add stores a subscription in the configured Redis set.
func (s *SubscriptionStore) Add(ctx context.Context, sub domain.Subscription) error {
	if s.key == "" {
		return fmt.Errorf("subscription set key is not configured")
	}
	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}
	if err := s.client.SAdd(ctx, s.key, string(data)).Err(); err != nil {
		return fmt.Errorf("redis SADD %s: %w", s.key, err)
	}
	return nil
}

// ListByInfluencer loads subscriptions for a given influencer by scanning the Redis set
// and filtering client-side. This is a simple implementation suitable for MVP.
func (s *SubscriptionStore) ListByInfluencer(ctx context.Context, influencerID string) ([]domain.Subscription, error) {
	if s.key == "" {
		return nil, fmt.Errorf("subscription set key is not configured")
	}
	members, err := s.client.SMembers(ctx, s.key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis SMEMBERS %s: %w", s.key, err)
	}

	res := make([]domain.Subscription, 0, len(members))
	for _, m := range members {
		var sub domain.Subscription
		if err := json.Unmarshal([]byte(m), &sub); err != nil {
			// Skip malformed entries but continue.
			continue
		}
		if sub.InfluencerID != influencerID {
			continue
		}
		res = append(res, sub)
	}
	return res, nil
}
