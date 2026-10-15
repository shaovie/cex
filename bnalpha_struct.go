package cex

import (
	"github.com/shopspring/decimal"
)

// BnAlphaWsPubMsg 公共频道推送, stream名是symbol的小写(alpha_xxxusdt@bookTicker)
type BnAlphaWsPubMsg struct {
	Id     int64     `json:"id,omitempty"`
	Code   int       `json:"code,omitempty"`
	Msg    string    `json:"msg,omitempty"`
	Result WsRawJSON `json:"result,omitempty"`
	Stream string    `json:"stream,omitempty"`
	Data   WsRawJSON `json:"data,omitempty"`
}

func (v *BnAlphaWsPubMsg) reset() {
	v.Id = 0
	v.Code = 0
	v.Msg = ""
	v.Result = nil
	v.Stream = ""
	v.Data = nil
}

// BnAlphaBookTicker 最优买卖价 <symbol>@bookTicker
type BnAlphaBookTicker struct {
	Symbol   string          `json:"s,omitempty"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
	Time     int64           `json:"E,omitempty"` // msec
}
