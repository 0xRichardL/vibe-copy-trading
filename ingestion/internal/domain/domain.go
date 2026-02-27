package domain

// Influencer represents minimal configuration for an influencer account.
// For now, we only track the Hyperliquid address.
type Influencer struct {
	Address string `json:"address"`
}
