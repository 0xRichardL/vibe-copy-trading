package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/libs/go/numbers"
	"golang.org/x/net/websocket"
)

// RawHyperliquidEvent is a generic representation of an incoming Hyperliquid event.
type RawHyperliquidEvent map[string]any

// HyperliquidClient abstracts WebSocket interactions for redundant listeners.
type HyperliquidClient struct {
	wsURL  string
	logger *log.Logger
}

func NewHyperliquidClient(cfg Config, logger *log.Logger) *HyperliquidClient {
	return &HyperliquidClient{
		wsURL:  cfg.HyperWSURL,
		logger: logger,
	}
}

// SubscribeAccountEvents connects to the Hyperliquid WebSocket and streams
// account-level events for a single influencer.
func (c *HyperliquidClient) SubscribeAccountEvents(
	ctx context.Context,
	inf Influencer,
	handler func(RawHyperliquidEvent, time.Time) error,
) error {
	if handler == nil {
		return errors.New("SubscribeAccountEvents: handler is required")
	}

	ws, err := websocket.Dial(c.wsURL, "", "http://localhost/")
	if err != nil {
		return err
	}
	defer func() {
		err := ws.Close()
		if err != nil {
			c.logger.Printf("error closing websocket for influencer %s: %v", inf.ID, err)
		}
	}()

	deadline := 5 * time.Second
	if err := ws.SetDeadline(time.Now().Add(deadline)); err != nil {
		c.logger.Printf("set deadline warning for %s: %v", inf.ID, err)
	}

	subMsg := defaultSubscribePayload(inf)
	c.logger.Printf("subscribing influencer %s (%s)", inf.ID, inf.Address)
	if err := websocket.JSON.Send(ws, subMsg); err != nil {
		return fmt.Errorf("send subscribe payload: %w", err)
	}
	_ = ws.SetDeadline(time.Time{})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var msg RawHyperliquidEvent
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			return err
		}

		received := time.Now().UTC()
		if err := handler(msg, received); err != nil && !errors.Is(err, context.Canceled) {
			c.logger.Printf("handler error for influencer %s: %v", inf.ID, err)
		}
	}
}

// NormalizeEventToSignal converts an event (with listener metadata) into a signal proto.
func NormalizeEventToSignal(inf Influencer, raw RawHyperliquidEvent, receivedAt time.Time) (*busv1.Signal, error) {
	parsed, err := parseHyperliquidUserEvent(raw, receivedAt)
	if err != nil {
		return nil, err
	}

	if parsed.Market == "" {
		return nil, fmt.Errorf("missing market in event for influencer %s", inf.ID)
	}

	sourceID := parsed.SourceEventID
	if sourceID == "" {
		sourceID = fmt.Sprintf("%s:%d", parsed.Kind, parsed.Sequence)
	}
	signalID := buildSignalID(inf.ID, parsed.Market, sourceID)
	action := deriveSignalAction(parsed)
	if parsed.Side == "" {
		parsed.Side = "FLAT"
	}

	timestamp := parsed.TimestampMs
	if timestamp == 0 {
		timestamp = receivedAt.UnixMilli()
	}

	metadata := map[string]string{
		"event_type":      parsed.Kind,
		"source_event_id": sourceID,
	}
	if inf.Address != "" {
		metadata["influencer_address"] = inf.Address
	}
	if parsed.Sequence != 0 {
		metadata["event_sequence"] = strconv.FormatInt(parsed.Sequence, 10)
	}
	if rawJSON, err := json.Marshal(raw); err == nil {
		metadata["raw"] = string(rawJSON)
	}

	return &busv1.Signal{
		SignalId:      signalID,
		InfluencerId:  inf.ID,
		Exchange:      "hyperliquid",
		Market:        parsed.Market,
		Action:        action,
		Side:          parsed.Side,
		Size:          parsed.PositionSize,
		DeltaSize:     parsed.DeltaSize,
		Price:         parsed.Price,
		TimestampMs:   timestamp,
		SourceEventId: sourceID,
		Metadata:      metadata,
	}, nil
}

func defaultSubscribePayload(inf Influencer) map[string]any {
	streams := []string{"fills", "orders", "positions"}
	payload := map[string]any{
		"type":    "subscribe",
		"channel": "userEvents",
		"account": inf.Address,
		"streams": streams,
	}
	if len(inf.Markets) > 0 {
		payload["markets"] = inf.Markets
	}
	return payload
}

type hyperliquidUserEvent struct {
	Kind          string
	Market        string
	Sequence      int64
	SourceEventID string
	Side          string
	PrevSide      string
	PositionSize  float64
	DeltaSize     float64
	Price         float64
	TimestampMs   int64
	Raw           RawHyperliquidEvent
}

func parseHyperliquidUserEvent(raw RawHyperliquidEvent, receivedAt time.Time) (*hyperliquidUserEvent, error) {
	if raw == nil {
		return nil, errors.New("empty raw event")
	}

	evt := &hyperliquidUserEvent{Raw: raw}
	evt.Kind = strings.ToLower(firstString(raw, "type", "eventType", "kind"))
	evt.Market = strings.ToUpper(firstString(raw, "market", "symbol", "pair"))
	evt.Sequence = firstInt(raw, "sequence", "seq", "nonce")
	evt.SourceEventID = firstString(raw, "sourceEventId", "eventId", "txHash", "txid", "id")
	evt.Side = strings.ToUpper(firstString(raw, "side", "positionSide", "direction"))
	evt.PrevSide = strings.ToUpper(firstString(raw, "prevSide", "previousSide", "priorSide"))
	evt.Price = firstFloat(raw, "price", "fillPrice", "avgPrice")
	evt.DeltaSize = firstFloat(raw, "deltaSize", "sizeDelta", "delta", "fillSize", "quantity", "size")
	evt.PositionSize = firstFloat(raw, "positionSize", "sizeAfter", "currentSize", "positionQty")
	if evt.PositionSize == 0 {
		if posMap, ok := nestedMap(raw, "position"); ok {
			if v, err := numbers.ExtractFloat(posMap["size"]); err == nil {
				evt.PositionSize = v
			}
			posRaw := RawHyperliquidEvent(posMap)
			if side := firstString(posRaw, "side", "direction"); side != "" && evt.Side == "" {
				evt.Side = strings.ToUpper(side)
			}
		}
	}
	if evt.PositionSize == 0 && evt.DeltaSize != 0 {
		evt.PositionSize = evt.DeltaSize
	}
	if evt.Kind == "" {
		evt.Kind = "event"
	}
	if evt.SourceEventID == "" && evt.Sequence != 0 {
		evt.SourceEventID = fmt.Sprintf("%s:%d", evt.Kind, evt.Sequence)
	}
	evt.TimestampMs = firstTimestampMillis(raw, receivedAt)
	return evt, nil
}

func deriveSignalAction(evt *hyperliquidUserEvent) string {
	switch {
	case evt == nil:
		return "OPEN"
	case evt.PrevSide != "" && evt.Side != "" && !strings.EqualFold(evt.PrevSide, evt.Side):
		return "FLIP"
	case evt.PositionSize == 0 && evt.DeltaSize < 0:
		return "CLOSE"
	case evt.DeltaSize > 0:
		if evt.PositionSize == evt.DeltaSize || evt.PrevSide == "" {
			return "OPEN"
		}
		return "INCREASE"
	case evt.DeltaSize < 0:
		if evt.PositionSize == 0 {
			return "CLOSE"
		}
		return "DECREASE"
	default:
		return "OPEN"
	}
}

func buildSignalID(influencerID, market, sourceID string) string {
	base := fmt.Sprintf("%s|%s|%s", influencerID, market, sourceID)
	hash := sha256.Sum256([]byte(base))
	return hex.EncodeToString(hash[:])
}

func firstString(raw RawHyperliquidEvent, keys ...string) string {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			switch v := val.(type) {
			case string:
				return strings.TrimSpace(v)
			case json.Number:
				return v.String()
			case fmt.Stringer:
				return strings.TrimSpace(v.String())
			}
		}
	}
	return ""
}

func firstFloat(raw RawHyperliquidEvent, keys ...string) float64 {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			if f, err := numbers.ExtractFloat(val); err == nil {
				return f
			}
		}
	}
	return 0
}

func firstInt(raw RawHyperliquidEvent, keys ...string) int64 {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			if i, err := numbers.ExtractInt(val); err == nil {
				return i
			}
		}
	}
	return 0
}

func firstTimestampMillis(raw RawHyperliquidEvent, fallback time.Time) int64 {
	if ms := firstInt(raw, "timestamp", "ts", "time", "eventTime"); ms != 0 {
		if ms > 1_000_000_000_000 {
			return ms
		}
		return ms * 1000
	}
	if !fallback.IsZero() {
		return fallback.UnixMilli()
	}
	return 0
}

func nestedMap(raw RawHyperliquidEvent, key string) (map[string]any, bool) {
	if val, ok := raw[key]; ok {
		if m, ok := val.(map[string]any); ok {
			return m, true
		}
	}
	return nil, false
}
