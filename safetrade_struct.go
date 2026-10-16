package cex

import (
	"github.com/shopspring/decimal"
)

// SafetradeDepth 公有depth推送的body, 如:
// {"asks":[],"bids":[["1.31","12449.2664"]],"sequence":3592202}
type SafetradeDepth struct {
	Asks [][2]decimal.Decimal `json:"asks"`
	Bids [][2]decimal.Decimal `json:"bids"`
}

// SafetradeWsOrder 私有order推送, 字段和rest订单一致
type SafetradeWsOrder struct {
	Id           int64           `json:"id,omitempty"`
	Market       string          `json:"market,omitempty"`
	Side         string          `json:"side,omitempty"`
	Type         string          `json:"type,omitempty"`
	OrdType      string          `json:"ord_type,omitempty"` // 文档未列出, 实际返回ord_type
	State        string          `json:"state,omitempty"`
	Price        decimal.Decimal `json:"price"`
	AvgPrice     decimal.Decimal `json:"avg_price"`
	OriginAmount decimal.Decimal `json:"origin_amount"`
	FilledAmount decimal.Decimal `json:"filled_amount"`
	CreatedAt    string          `json:"created_at,omitempty"`
	UpdatedAt    string          `json:"updated_at,omitempty"`
}

// SafetradeWsBalance 私有balance推送
type SafetradeWsBalance struct {
	Currency string          `json:"currency,omitempty"`
	Balance  decimal.Decimal `json:"balance"`
	Locked   decimal.Decimal `json:"locked"`
}
