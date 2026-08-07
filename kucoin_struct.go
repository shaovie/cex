package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type KucoinWsPubMsg struct {
	Type    string          `json:"type"`
	Channel string          `json:"T"`
	T       string          `json:"t"`
	Depth   string          `json:"dp"`
	Data    json.RawMessage `json:"d"`
}

func (v *KucoinWsPubMsg) reset() {
	v.Type = ""
	v.T = ""
	v.Channel = ""
	v.Depth = ""
	v.Data = nil
}

type KucoinTicker struct {
	Symbol string               `json:"s"`
	Bids   [][2]decimal.Decimal `json:"b"`
	Asks   [][2]decimal.Decimal `json:"a"`
}
