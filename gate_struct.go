package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type GateSpotOrderBook struct {
	Symbol string               `json:"s,omitempty"`
	Time   int64                `json:"t,omitempty"`
	Bids   [][2]decimal.Decimal `json:"bids,omitempty"`
	Asks   [][2]decimal.Decimal `json:"asks,omitempty"`
}

type GateContractOrderBookTick struct {
	Price decimal.Decimal `json:"p,omitempty"`
	Size  decimal.Decimal `json:"s,omitempty"`
}
type GateContractOrderBook struct {
	Symbol string                      `json:"contract,omitempty"`
	Time   int64                       `json:"t,omitempty"`
	Bids   []GateContractOrderBookTick `json:"bids,omitempty"`
	Asks   []GateContractOrderBookTick `json:"asks,omitempty"`
}
type GateWsSpotPubMsg struct {
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"result,omitempty"`
}
type GateWsContractPubMsg struct {
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"result,omitempty"`
}
type GateSpot24hTicker struct {
	Symbol      string          `json:"currency_pair"`
	Last        decimal.Decimal `json:"last"`
	Volume      decimal.Decimal `json:"base_volume"`
	QuoteVolume decimal.Decimal `json:"quote_volume"`
}
type GateContract24hTicker struct {
	Symbol      string          `json:"contract"`
	Last        decimal.Decimal `json:"last"`
	Volume      decimal.Decimal `json:"volume_24h_base"`
	QuoteVolume decimal.Decimal `json:"volume_24h_quote"`
}
type GateFundingRate struct {
	Name     string          `json:"name"`
	NextTime int64           `json:"funding_next_apply,omitempty"`
	Fr       decimal.Decimal `json:"funding_rate,omitempty"`
	Offline  bool            `json:"in_delisting,omitempty"` // 下线过渡期
}
