package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type BinanceSpot24hTicker struct {
	Symbol      string          `json:"s"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}
type BinanceWsSpotPubMsg struct {
	Code   int             `json:"code,omitempty"`
	Stream string          `json:"stream,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

func (v *BinanceWsSpotPubMsg) reset() {
	v.Code = 0
	v.Stream = ""
	v.Data = nil
}

type BinanceWsContractPubMsg struct {
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"result,omitempty"`
}
type BinanceSpotOrderBook struct {
	Bids [][2]decimal.Decimal `json:"bids,omitempty"`
	Asks [][2]decimal.Decimal `json:"asks,omitempty"`
}
type BinanceContractOrderBook struct {
	Event  string               `json:"e,omitempty"`
	Time   int64                `json:"E,omitempty"`
	Symbol string               `json:"s,omitempty"`
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}
