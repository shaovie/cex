package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type MexcSpot24hTicker struct {
	Symbol      string          `json:"s"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}
type MexcWsSpotPubMsg struct {
	Code   int             `json:"code,omitempty"`
	Stream string          `json:"stream,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

func (v *MexcWsSpotPubMsg) reset() {
	v.Code = 0
	v.Stream = ""
	v.Data = nil
}

type MexcSpotOrderBook struct {
	Bids [][2]decimal.Decimal `json:"bids,omitempty"`
	Asks [][2]decimal.Decimal `json:"asks,omitempty"`
}
