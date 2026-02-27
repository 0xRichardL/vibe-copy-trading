package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/config"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/domain"
	busv1 "github.com/0xRichardL/vibe-copy-trading/libs/go/domain/bus/v1"
	"github.com/0xRichardL/vibe-copy-trading/libs/go/numbers"
	hl "github.com/sonirico/go-hyperliquid"
)

// HyperliquidService abstracts WebSocket interactions for redundant listeners.
type HyperliquidService struct {
	wsURL  string
	logger *log.Logger
}

func NewHyperliquidService(cfg config.Config, logger *log.Logger) *HyperliquidService {
	return &HyperliquidService{
		wsURL:  cfg.HyperWSURL,
		logger: logger,
	}
}

// SignalHandler processes normalized signals prior to downstream distribution.
type SignalHandler func(context.Context, *busv1.Signal) error

// SubscribeAccountEvents connects to the Hyperliquid WebSocket and streams
// account-level events for a single influencer.
func (s *HyperliquidService) SubscribeAccountEvents(
	ctx context.Context,
	inf *domain.Influencer,
	handler SignalHandler,
) error {
	if handler == nil {
		return errors.New("SubscribeAccountEvents: handler is required")
	}

	ws := hl.NewWebsocketClient(s.wsURL)
	if err := ws.Connect(ctx); err != nil {
		return fmt.Errorf("connect websocket: %w", err)
	}
	defer func() {
		if err := ws.Close(); err != nil {
			s.logger.Printf("error closing websocket for influencer %s: %v", inf.Address, err)
		}
	}()

	s.logger.Printf("subscribing influencer %s to Hyperliquid user fills", inf.Address)
	sub, err := ws.OrderFills(
		hl.OrderFillsSubscriptionParams{User: inf.Address},
		func(fills hl.WsOrderFills, err error) {
			if err != nil {
				s.logger.Printf("order fills callback error for influencer %s: %v", inf.Address, err)
				return
			}
			if len(fills.Fills) == 0 {
				return
			}

			received := time.Now().UTC()
			for _, f := range fills.Fills {
				sig, err := NormalizeEventToSignal(inf, f, received)
				if err != nil {
					s.logger.Printf("normalize event error for influencer %s: %v", inf.Address, err)
					continue
				}
				if sig == nil {
					continue
				}
				if err := handler(ctx, sig); err != nil && !errors.Is(err, context.Canceled) {
					s.logger.Printf("handler error for influencer %s: %v", inf.Address, err)
				}
			}
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe to order fills: %w", err)
	}
	defer sub.Close()

	<-ctx.Done()
	return ctx.Err()
}

// NormalizeEventToSignal converts a single Hyperliquid WsOrderFill into a Signal.
func NormalizeEventToSignal(inf *domain.Influencer, fill hl.WsOrderFill, receivedAt time.Time) (*busv1.Signal, error) {
	if fill.Coin == "" {
		return nil, fmt.Errorf("missing market (coin) in fill for influencer %s", inf.Address)
	}

	market := strings.ToUpper(fill.Coin)
	price, _ := numbers.ExtractFloat(fill.Px)
	startPosition, _ := numbers.ExtractFloat(fill.StartPosition)
	size, _ := numbers.ExtractFloat(fill.Sz)
	timestamp := fill.Time
	if timestamp == 0 && !receivedAt.IsZero() {
		timestamp = receivedAt.UnixMilli()
	}

	sideToken := strings.ToUpper(strings.TrimSpace(fill.Side))
	isBuy := sideToken == "B" || sideToken == "BUY"
	var newPosition float64
	if isBuy {
		newPosition = startPosition + size
	} else {
		newPosition = startPosition - size
	}

	prevSideStr := positionSideFromSize(startPosition)
	sideStr := positionSideFromSize(newPosition)
	positionSize := math.Abs(newPosition)
	deltaSize := newPosition - startPosition
	action := deriveSignalAction(prevSideStr, sideStr, positionSize, deltaSize)
	sideEnum := normalizeSignalSide(sideStr)

	sourceID := ""
	if fill.Hash != "" {
		sourceID = fill.Hash
	} else if fill.Tid != 0 {
		sourceID = fmt.Sprintf("tid:%d", fill.Tid)
	} else if fill.Oid != 0 {
		sourceID = fmt.Sprintf("oid:%d", fill.Oid)
	}
	if sourceID == "" {
		sourceID = fmt.Sprintf("fill:%s:%d", market, timestamp)
	}
	signalID := buildSignalID(inf.Address, market, sourceID)

	metadata := map[string]string{
		"event_type":      "fill",
		"source_event_id": sourceID,
	}
	if inf.Address != "" {
		metadata["influencer_address"] = inf.Address
	}
	if rawJSON, err := json.Marshal(fill); err == nil {
		metadata["raw"] = string(rawJSON)
	}

	return &busv1.Signal{
		SignalId:      signalID,
		InfluencerId:  inf.Address,
		Exchange:      "hyperliquid",
		Market:        market,
		Action:        action,
		Side:          sideEnum,
		Size:          positionSize,
		DeltaSize:     deltaSize,
		Price:         price,
		TimestampMs:   timestamp,
		SourceEventId: sourceID,
		Metadata:      metadata,
	}, nil
}

func deriveSignalAction(prevSide, side string, positionSize, deltaSize float64) busv1.SignalAction {
	switch {
	case prevSide != "" && side != "" && !strings.EqualFold(prevSide, side):
		return busv1.SignalAction_SIGNAL_ACTION_FLIP
	case positionSize == 0 && deltaSize < 0:
		return busv1.SignalAction_SIGNAL_ACTION_CLOSE
	case deltaSize > 0:
		if positionSize == deltaSize || prevSide == "" {
			return busv1.SignalAction_SIGNAL_ACTION_OPEN
		}
		return busv1.SignalAction_SIGNAL_ACTION_INCREASE
	case deltaSize < 0:
		if positionSize == 0 {
			return busv1.SignalAction_SIGNAL_ACTION_CLOSE
		}
		return busv1.SignalAction_SIGNAL_ACTION_DECREASE
	default:
		return busv1.SignalAction_SIGNAL_ACTION_OPEN
	}
}

func positionSideFromSize(size float64) string {
	switch {
	case size > 0:
		return "LONG"
	case size < 0:
		return "SHORT"
	default:
		return "FLAT"
	}
}

func normalizeSignalSide(side string) busv1.SignalSide {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "LONG":
		return busv1.SignalSide_SIGNAL_SIDE_LONG
	case "SHORT":
		return busv1.SignalSide_SIGNAL_SIDE_SHORT
	case "FLAT":
		return busv1.SignalSide_SIGNAL_SIDE_FLAT
	default:
		return busv1.SignalSide_SIGNAL_SIDE_UNSPECIFIED
	}
}

func buildSignalID(influencerID, market, sourceID string) string {
	base := fmt.Sprintf("%s|%s|%s", influencerID, market, sourceID)
	hash := sha256.Sum256([]byte(base))
	return hex.EncodeToString(hash[:])
}
