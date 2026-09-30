package cex

import (
	"github.com/mailru/easyjson/jlexer"
	"github.com/shopspring/decimal"
)

// DeespSeek优化
// BnRawJSON 零拷贝取原始JSON值: 直接指向输入缓冲, 不复制.
// json.RawMessage的UnmarshalJSON会append一份拷贝, WS热路径上不需要
type BnRawJSON []byte

func (m *BnRawJSON) UnmarshalEasyJSON(l *jlexer.Lexer) {
	*m = l.Raw()
}

type BinanceSpot24hTicker struct {
	Symbol      string          `json:"s"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}
type BinanceFutures24hTicker struct {
	Symbol      string          `json:"s"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}
type BinanceWsPubMsg struct {
	Code   int       `json:"code,omitempty"`
	Stream string    `json:"stream,omitempty"`
	Data   BnRawJSON `json:"data,omitempty"`
}

func (v *BinanceWsPubMsg) reset() {
	v.Code = 0
	v.Stream = ""
	v.Data = nil
}

type BinanceSpotBBO struct {
	Symbol   string          `json:"s,omitempty"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
}
type BinanceFuturesBBO struct {
	Symbol   string          `json:"s,omitempty"`
	Time     int64           `json:"T,omitempty"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
}
type BinanceSpotOrderBook struct {
	Bids [][2]decimal.Decimal `json:"bids,omitempty"`
	Asks [][2]decimal.Decimal `json:"asks,omitempty"`
}

func (v *BinanceSpotOrderBook) reset() {
	v.Bids = v.Bids[:0]
	v.Asks = v.Asks[:0]
}

type BinanceFuturesOrderBook struct {
	Event  string               `json:"e,omitempty"`
	Time   int64                `json:"E,omitempty"`
	Symbol string               `json:"s,omitempty"`
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}

func (v *BinanceFuturesOrderBook) reset() {
	// v.Event = ""
	// v.Time = 0
	// v.Symbol = ""
	v.Bids = v.Bids[:0]
	v.Asks = v.Asks[:0]
}

type BinanceSpotPublicTrade struct {
	Symbol string `json:"s"`
	//TradeId int64           `json:"t"`
	Time  int64           `json:"T"`
	Price decimal.Decimal `json:"p"`
	Qty   decimal.Decimal `json:"q"`
}
