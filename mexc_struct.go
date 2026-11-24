package cex

import (
	"github.com/shopspring/decimal"
)

// MexcSpotOrder 现货订单
type MexcSpotOrder struct {
	Code          int             `json:"code,omitempty"`
	Msg           string          `json:"msg,omitempty"`
	Symbol        string          `json:"symbol,omitempty"`
	OrderId       jsonNumber      `json:"orderId,omitempty"`
	ClientId      string          `json:"clientOrderId,omitempty"`
	Price         decimal.Decimal `json:"price"`
	OrigQty       decimal.Decimal `json:"origQty"`
	ExecutedQty   decimal.Decimal `json:"executedQty"`
	CummQuoteQty  decimal.Decimal `json:"cumulativeQuoteQty"`
	CummQuoteQty2 decimal.Decimal `json:"cummulativeQuoteQty"`
	Status        string          `json:"status,omitempty"`
	Type          string          `json:"type,omitempty"`
	TimeInForce   string          `json:"timeInForce,omitempty"`
	Side          string          `json:"side,omitempty"`
	Time          int64           `json:"time,omitempty"`
	UpdateTime    int64           `json:"updateTime,omitempty"`
}

// FilledAmt 兼容两种拼写
func (v *MexcSpotOrder) filledAmt() decimal.Decimal {
	if v.CummQuoteQty2.IsPositive() {
		return v.CummQuoteQty2
	}
	return v.CummQuoteQty
}
