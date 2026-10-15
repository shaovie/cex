package cex

import (
	"github.com/shopspring/decimal"
)

// BitgetSpotTicker 现货行情, 无报价的交易对这些字段可能是空字符串
type BitgetSpotTicker struct {
	Symbol      string `json:"symbol,omitempty"`
	LastPrice   string `json:"lastPrice"`
	BidPrice    string `json:"bid1Price"`
	AskPrice    string `json:"ask1Price"`
	BidQty      string `json:"bid1Size"`
	AskQty      string `json:"ask1Size"`
	BaseVolume  string `json:"volume24h"`
	QuoteVolume string `json:"turnover24h"`
	Ts          string `json:"ts,omitempty"`
}

// toDecimal 空字符串返回0
func (v *BitgetSpotTicker) toDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// BitgetSpotOrder 现货订单
type BitgetSpotOrder struct {
	Category     string          `json:"category,omitempty"`
	Symbol       string          `json:"symbol,omitempty"`
	OrderId      string          `json:"orderId,omitempty"`
	ClientOid    string          `json:"clientOid,omitempty"`
	Price        decimal.Decimal `json:"price"`
	Qty          decimal.Decimal `json:"qty"`
	Amount       decimal.Decimal `json:"amount"`
	AvgPrice     decimal.Decimal `json:"avgPrice"`
	CumExecQty   decimal.Decimal `json:"cumExecQty"`
	CumExecValue decimal.Decimal `json:"cumExecValue"`
	OrderStatus  string          `json:"orderStatus,omitempty"`
	OrderType    string          `json:"orderType,omitempty"`
	TimeInForce  string          `json:"timeInForce,omitempty"`
	Side         string          `json:"side,omitempty"`
	Fee          decimal.Decimal `json:"fee"`
	FeeCoin      string          `json:"feeCoin,omitempty"`
	CreatedTime  string          `json:"createdTime,omitempty"`
	UpdatedTime  string          `json:"updatedTime,omitempty"`
}

// BitgetAsset 统一账户资产
type BitgetAsset struct {
	Coin      string          `json:"coin"`
	Available decimal.Decimal `json:"available"`
	Balance   decimal.Decimal `json:"balance"`
	Locked    decimal.Decimal `json:"locked"`
	Equity    decimal.Decimal `json:"equity"`
	UsdValue  decimal.Decimal `json:"usdValue"`
}

// BitgetWsArg websocket频道参数
type BitgetWsArg struct {
	InstType string `json:"instType,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
}

// BitgetWsMsg websocket公共/私有频道消息, 如:
// {"action":"snapshot","arg":{"instType":"spot","topic":"books1","symbol":"BTCUSDT"},"data":[...]}
// {"event":"subscribe","arg":{...},"connId":"..."} / {"event":"error","code":"30001","msg":"..."}
type BitgetWsMsg struct {
	Event  string      `json:"event,omitempty"`
	Action string      `json:"action,omitempty"`
	Arg    BitgetWsArg `json:"arg"`
	Data   WsRawJSON   `json:"data,omitempty"`
	Code   string      `json:"code,omitempty"`
	Msg    string      `json:"msg,omitempty"`
	Ts     int64       `json:"ts,omitempty"`
}

func (v *BitgetWsMsg) reset() {
	v.Event = ""
	v.Action = ""
	v.Arg.InstType = ""
	v.Arg.Topic = ""
	v.Arg.Symbol = ""
	v.Data = nil
	v.Code = ""
	v.Msg = ""
	v.Ts = 0
}

// BitgetWsOrderBook 盘口推送(books1/books5), a为卖盘, b为买盘
type BitgetWsOrderBook struct {
	Asks [][2]decimal.Decimal `json:"a,omitempty"`
	Bids [][2]decimal.Decimal `json:"b,omitempty"`
	Ts   string               `json:"ts,omitempty"`
	Seq  int64                `json:"seq,omitempty"`
}

// BitgetWsTrade 成交推送
type BitgetWsTrade struct {
	Price decimal.Decimal `json:"p"`
	Size  decimal.Decimal `json:"v"`
	Side  string          `json:"S,omitempty"`
	Ts    string          `json:"T,omitempty"`
}

// BitgetWsAsset 私有资产推送
type BitgetWsAsset struct {
	Coin      string          `json:"coin"`
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Locked    decimal.Decimal `json:"locked"`
	Equity    decimal.Decimal `json:"equity"`
	UsdValue  decimal.Decimal `json:"usdValue"`
}

// BitgetWsAccount 统一账户资产推送
type BitgetWsAccount struct {
	AccountEquity decimal.Decimal `json:"accountEquity"`
	Coin          []BitgetWsAsset `json:"coin"`
}
