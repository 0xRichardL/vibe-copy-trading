package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/domain"
	redis "github.com/redis/go-redis/v9"
)

// InfluencerStore abstracts reading influencer configuration from Redis.
type InfluencerStore struct {
	client *redis.Client
	key    string
}

// ErrNoInfluencers indicates Redis does not currently have any influencers available.
var ErrNoInfluencers = errors.New("no influencers available")

func NewInfluencerStore(client *redis.Client, key string) *InfluencerStore {
	return &InfluencerStore{client: client, key: key}
}

func (s *InfluencerStore) Add(ctx context.Context, inf domain.Influencer) error {
	if s.key == "" {
		return fmt.Errorf("influencer set key is not configured")
	}
	data, err := json.Marshal(inf)
	if err != nil {
		return fmt.Errorf("marshal influencer: %w", err)
	}
	if err := s.client.SAdd(ctx, s.key, string(data)).Err(); err != nil {
		return fmt.Errorf("redis SADD %s: %w", s.key, err)
	}
	return nil
}

// List loads all influencers from the Redis set referenced by the configured key.
func (s *InfluencerStore) List(ctx context.Context) ([]domain.Influencer, error) {
	if s.key == "" {
		return nil, fmt.Errorf("influencer set key is not configured")
	}
	members, err := s.client.SMembers(ctx, s.key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis SMEMBERS %s: %w", s.key, err)
	}

	res := make([]domain.Influencer, 0, len(members))
	for _, m := range members {
		var inf domain.Influencer
		if err := json.Unmarshal([]byte(m), &inf); err != nil {
			// Skip malformed entries but continue.
			continue
		}
		if inf.Address == "" {
			continue
		}
		res = append(res, inf)
	}
	return res, nil
}

type PutBackFunc func() error

// Acquire removes a single influencer from Redis and returns it to the caller.
func (s *InfluencerStore) Acquire(ctx context.Context) (*domain.Influencer, PutBackFunc, error) {
	if s.key == "" {
		return nil, nil, fmt.Errorf("influencer set key is not configured")
	}
	member, err := s.client.SPop(ctx, s.key).Result()
	if err == redis.Nil {
		return nil, nil, ErrNoInfluencers
	}
	if err != nil {
		return nil, nil, fmt.Errorf("redis SPOP %s: %w", s.key, err)
	}
	inf := &domain.Influencer{}
	if err := json.Unmarshal([]byte(member), inf); err != nil {
		return nil, nil, fmt.Errorf("unmarshal influencer: %w", err)
	}
	putBackFunc := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.client.SAdd(ctx, s.key, member).Err(); err != nil {
			return fmt.Errorf("redis SADD %s: %w", s.key, err)
		}
		return nil
	}
	return inf, putBackFunc, nil
}
