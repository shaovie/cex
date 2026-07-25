package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type KtxWsPubMsg struct {
	Pong   float64         `json:"pong"`
	Op     string          `json:"op"`
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data,omitempty"`
}

func (v *KtxWsPubMsg) reset() {
	v.Pong = 0.0
	v.Op = ""
	v.Stream = ""
	v.Data = nil
}

type KtxTicker struct {
	Symbol   string          `json:"product"`
	BidPrice decimal.Decimal `json:"bidPrice"`
	BidQty   decimal.Decimal `json:"bidQty"`
	AskPrice decimal.Decimal `json:"askPrice"`
	AskQty   decimal.Decimal `json:"askQty"`
}
