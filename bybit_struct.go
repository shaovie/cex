package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type BybitWsPubMsg struct {
	Time  int64           `json:"ts,omitempty"`
	Op    string          `json:"op,omitempty"`
	Topic string          `json:"topic,omitempty"`
	Type  string          `json:"type,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (v *BybitWsPubMsg) reset() {
	v.Time = 0
	v.Op = ""
	v.Topic = ""
	v.Type = ""
	v.Data = nil
}

type BybitOrderBook struct {
	Symbol string               `json:"s,omitempty"`
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}
type BybitSpotBBO struct {
	Symbol string               `json:"s,omitempty"`
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}
type BybitSpot24hTicker struct {
	Symbol      string          `json:"s"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}
