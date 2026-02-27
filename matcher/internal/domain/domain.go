package domain

// Subscription represents a follower's configuration to copy an influencer's signals.
type Subscription struct {
	ID                   string   `json:"subscription_id"`
	InfluencerID         string   `json:"influencer_id"`
	SubscriberID         string   `json:"subscriber_id"`
	Status               string   `json:"status"`
	AllowedMarkets       []string `json:"allowed_markets,omitempty"`
	SizeMode             string   `json:"size_mode"`
	SizeValue            float64  `json:"size_value"`
	MaxNotionalPerSignal float64  `json:"max_notional_per_signal,omitempty"`
	MaxOpenNotional      float64  `json:"max_open_notional,omitempty"`
	Leverage             float64  `json:"leverage,omitempty"`
}
