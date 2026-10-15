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

// BnAlphaDepthUpdate 增量盘口 <symbol>@fulldepth@100ms
// 数量为0表示删掉该档位
type BnAlphaDepthUpdate struct {
	Symbol string               `json:"s,omitempty"`
	Time   int64                `json:"E,omitempty"`  // 事件时间 msec
	LastId int64                `json:"u,omitempty"`  // 本次更新最后一个updateId
	PrevId int64                `json:"pu,omitempty"` // 上次推送的updateId
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}

func (v *BnAlphaDepthUpdate) reset() {
	v.Symbol = ""
	v.Time = 0
	v.LastId = 0
	v.PrevId = 0
	v.Bids = v.Bids[:0]
	v.Asks = v.Asks[:0]
}
