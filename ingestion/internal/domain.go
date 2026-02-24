package internal

import "time"

// RawEvent represents a stored raw Hyperliquid event (WebSocket or REST).
// In this service we primarily stream them to logs or a future sink; the
// formal storage model can live elsewhere.
type RawEvent struct {
	ID                string         `json:"id"`
	InfluencerAddress string         `json:"influencerAddress"`
	EventType         string         `json:"eventType"`
	Market            string         `json:"market"`
	Payload           map[string]any `json:"payload"`
	Timestamp         time.Time      `json:"timestamp"`
	IngestedAt        time.Time      `json:"ingestedAt"`
}

// Influencer represents minimal configuration for an influencer account.
type Influencer struct {
	ID       string   // internal influencer id
	Address  string   // Hyperliquid account/address
	Markets  []string // optional subset of markets to subscribe to
	Priority int      // optional priority hint
}
