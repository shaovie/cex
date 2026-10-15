package cex

import (
	"github.com/shopspring/decimal"
)

// AsterSpotWsPubMsg 公共频道推送(combined stream)
type AsterSpotWsPubMsg struct {
	Id     int64     `json:"id,omitempty"`
	Code   int       `json:"code,omitempty"`
	Msg    string    `json:"msg,omitempty"`
	Result WsRawJSON `json:"result,omitempty"`
	Stream string    `json:"stream,omitempty"`
	Data   WsRawJSON `json:"data,omitempty"`
}

func (v *AsterSpotWsPubMsg) reset() {
	v.Id = 0
	v.Code = 0
	v.Msg = ""
	v.Result = nil
	v.Stream = ""
	v.Data = nil
}

// AsterSpotBookTicker 最优买卖价 <symbol>@bookTicker
type AsterSpotBookTicker struct {
	Symbol   string          `json:"s,omitempty"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
	Time     int64           `json:"E,omitempty"` // msec
}

// AsterSpotOrderBook 有限档位 <symbol>@depth5
type AsterSpotOrderBook struct {
	Symbol string               `json:"s,omitempty"`
	Time   int64                `json:"E,omitempty"` // msec
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}

func (v *AsterSpotOrderBook) reset() {
	v.Symbol = ""
	v.Time = 0
	v.Bids = v.Bids[:0]
	v.Asks = v.Asks[:0]
}

// AsterSpot24hTicker <symbol>@miniTicker
type AsterSpot24hTicker struct {
	Symbol      string          `json:"s,omitempty"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}

// AsterSpotPublicTrade <symbol>@aggTrade
type AsterSpotPublicTrade struct {
	Symbol string          `json:"s,omitempty"`
	Price  decimal.Decimal `json:"p"`
	Qty    decimal.Decimal `json:"q"`
	Time   int64           `json:"T,omitempty"` // msec
}

// AsterSpotWsPrivMsg 私有推送, 带listenKey的raw连接直接推事件本身
type AsterSpotWsPrivMsg struct {
	Event string `json:"e,omitempty"`
}

func (v *AsterSpotWsPrivMsg) reset() {
	v.Event = ""
}

// AsterSpotWsOrder 私有推送的订单 executionReport
type AsterSpotWsOrder struct {
	Symbol       string          `json:"s,omitempty"`
	ClientId     string          `json:"c,omitempty"`
	ClientIdOrig string          `json:"C,omitempty"`
	Side         string          `json:"S,omitempty"`
	OrderType    string          `json:"o,omitempty"`
	TimeInForce  string          `json:"f,omitempty"`
	Qty          decimal.Decimal `json:"q"`
	Price        decimal.Decimal `json:"p"`
	AvgPrice     decimal.Decimal `json:"ap"`
	StopPrice    decimal.Decimal `json:"P"`
	EventType    string          `json:"x,omitempty"`
	Status       string          `json:"X,omitempty"`
	OrderId      int64           `json:"i,omitempty"`
	LaseExecQty  decimal.Decimal `json:"l"`
	LaseExecPrc  decimal.Decimal `json:"L"`
	ExecutedQty  decimal.Decimal `json:"z"`
	FeeQty       decimal.Decimal `json:"n"`
	FeeAsset     string          `json:"N,omitempty"`
	TradeTime    int64           `json:"T,omitempty"` // msec
	CreateTime   int64           `json:"O,omitempty"` // msec
	CummQuoteQty decimal.Decimal `json:"Z"`
}

// AsterSpotWsBalance 私有推送的资产 outboundAccountPosition
type AsterSpotWsBalance struct {
	Balances []AsterSpotWsBalanceItem `json:"B,omitempty"`
	Time     int64                    `json:"E,omitempty"` // msec
}

type AsterSpotWsBalanceItem struct {
	Symbol string          `json:"a,omitempty"`
	Free   decimal.Decimal `json:"f"`
	Locked decimal.Decimal `json:"l"`
}

// AsterFuturesWsPubMsg 合约公共频道推送(combined stream)
type AsterFuturesWsPubMsg struct {
	Id     int64     `json:"id,omitempty"`
	Code   int       `json:"code,omitempty"`
	Msg    string    `json:"msg,omitempty"`
	Result WsRawJSON `json:"result,omitempty"`
	Stream string    `json:"stream,omitempty"`
	Data   WsRawJSON `json:"data,omitempty"`
}

func (v *AsterFuturesWsPubMsg) reset() {
	v.Id = 0
	v.Code = 0
	v.Msg = ""
	v.Result = nil
	v.Stream = ""
	v.Data = nil
}

// AsterFuturesBBO 合约最优买卖价 <symbol>@bookTicker
type AsterFuturesBBO struct {
	Symbol   string          `json:"s,omitempty"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
	Time     int64           `json:"E,omitempty"` // msec
}

// AsterFuturesOrderBook 合约有限档位 <symbol>@depth5@100ms
type AsterFuturesOrderBook struct {
	Symbol string               `json:"s,omitempty"`
	Time   int64                `json:"E,omitempty"` // msec
	Bids   [][2]decimal.Decimal `json:"b,omitempty"`
	Asks   [][2]decimal.Decimal `json:"a,omitempty"`
}

func (v *AsterFuturesOrderBook) reset() {
	v.Symbol = ""
	v.Time = 0
	v.Bids = v.Bids[:0]
	v.Asks = v.Asks[:0]
}

// AsterFutures24hTicker 合约 <symbol>@miniTicker
type AsterFutures24hTicker struct {
	Symbol      string          `json:"s,omitempty"`
	Last        decimal.Decimal `json:"c"`
	Volume      decimal.Decimal `json:"v"`
	QuoteVolume decimal.Decimal `json:"q"`
}

// AsterFuturesWsPrivMsg 合约私有推送, 带listenKey的raw连接直接推事件本身
type AsterFuturesWsPrivMsg struct {
	Event   string    `json:"e,omitempty"`
	Time    int64     `json:"E,omitempty"` // msec
	Order   WsRawJSON `json:"o,omitempty"`
	Account WsRawJSON `json:"a,omitempty"`
}

func (v *AsterFuturesWsPrivMsg) reset() {
	v.Event = ""
	v.Time = 0
	v.Order = nil
	v.Account = nil
}

// AsterFuturesWsOrder 合约私有推送的订单 ORDER_TRADE_UPDATE
type AsterFuturesWsOrder struct {
	Symbol      string          `json:"s,omitempty"`
	ClientId    string          `json:"c,omitempty"`
	Side        string          `json:"S,omitempty"`
	OrderType   string          `json:"o,omitempty"`
	TimeInForce string          `json:"f,omitempty"`
	Qty         decimal.Decimal `json:"q"`
	Price       decimal.Decimal `json:"p"`
	AvgPrice    decimal.Decimal `json:"ap"`
	StopPrice   decimal.Decimal `json:"sp"`
	EventType   string          `json:"x,omitempty"`
	Status      string          `json:"X,omitempty"`
	OrderId     int64           `json:"i,omitempty"`
	LaseExecQty decimal.Decimal `json:"l"`
	LaseExecPrc decimal.Decimal `json:"L"`
	ExecutedQty decimal.Decimal `json:"z"`
	FeeQty      decimal.Decimal `json:"n"`
	FeeAsset    string          `json:"N,omitempty"`
	TradeTime   int64           `json:"T,omitempty"` // msec
	Rpnl        decimal.Decimal `json:"rp"`
}
