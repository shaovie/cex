package cex

import (
	"github.com/shopspring/decimal"
)

// KucoinCurrency 账户资产
type KucoinCurrency struct {
	Currency  string          `json:"currency"`
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Hold      decimal.Decimal `json:"hold"`
	Equity    decimal.Decimal `json:"equity"`
}

// KucoinSpotOrder 现货订单
type KucoinSpotOrder struct {
	OrderId     string          `json:"orderId,omitempty"`
	ClientOid   string          `json:"clientOid,omitempty"`
	Symbol      string          `json:"symbol,omitempty"`
	TradeType   string          `json:"tradeType,omitempty"`
	Side        string          `json:"side,omitempty"`
	OrderType   string          `json:"orderType,omitempty"`
	Price       decimal.Decimal `json:"price"`
	Size        decimal.Decimal `json:"size"`
	SizeUnit    string          `json:"sizeUnit,omitempty"`
	FilledSize  decimal.Decimal `json:"filledSize"`
	AvgPrice    decimal.Decimal `json:"avgPrice"`
	Fee         decimal.Decimal `json:"fee"`
	FeeCurrency string          `json:"feeCurrency,omitempty"`
	Tax         decimal.Decimal `json:"tax"`
	TimeInForce string          `json:"timeInForce,omitempty"`
	Status      int             `json:"status"` // 0未触发 1已触发 2未成交 3已成交 4部分成交 5已撤销 6部分成交后撤销
	CreatedAt   string          `json:"createdAt,omitempty"`
	UpdatedAt   string          `json:"updatedAt,omitempty"`
}

// KucoinWsMsg websocket消息, 如:
// {"T":"obu.SPOT","dp":"1","t":"snapshot","P":... ,"d":{...}}
// {"T":"ticker.SPOT","P":...,"d":{...}} / {"T":"orderAll.UNIFIED",...} / {"T":"balance.UNIFIED",...}
// {"id":"1","result":true} / {"type":"pong"} / {"sessionId":"..","message":"welcome","pingInterval":18000}
type KucoinWsMsg struct {
	Type         string    `json:"type,omitempty"`
	Channel      string    `json:"T,omitempty"`
	DataType     string    `json:"t,omitempty"`
	Depth        string    `json:"dp,omitempty"`
	Data         WsRawJSON `json:"d,omitempty"`
	Id           string    `json:"id,omitempty"`
	Result       bool      `json:"result,omitempty"`
	SessionId    string    `json:"sessionId,omitempty"`
	Message      string    `json:"message,omitempty"`
	PingInterval int       `json:"pingInterval,omitempty"`
}

func (v *KucoinWsMsg) reset() {
	v.Type = ""
	v.Channel = ""
	v.DataType = ""
	v.Depth = ""
	v.Data = nil
	v.Id = ""
	v.Result = false
	v.SessionId = ""
	v.Message = ""
	v.PingInterval = 0
}

// KucoinTicker 盘口推送(obu)
type KucoinTicker struct {
	Symbol string      `json:"s"`
	Bids   [][2]string `json:"b"`
	Asks   [][2]string `json:"a"`
	Time   int64       `json:"M,omitempty"`
}

// KucoinWsTicker 行情推送(ticker)
type KucoinWsTicker struct {
	Symbol   string          `json:"s"`
	AskPrice decimal.Decimal `json:"a"`
	AskQty   decimal.Decimal `json:"A"`
	BidPrice decimal.Decimal `json:"b"`
	BidQty   decimal.Decimal `json:"B"`
	Last     decimal.Decimal `json:"l"`
	LastQty  decimal.Decimal `json:"q"`
	Side     string          `json:"S,omitempty"`
	Time     int64           `json:"M,omitempty"`
}

// KucoinWsBalance 私有资产推送
type KucoinWsBalance struct {
	Currency  string          `json:"c"`
	Available decimal.Decimal `json:"a"`
	Balance   decimal.Decimal `json:"b"`
	Hold      decimal.Decimal `json:"h"`
	Equity    decimal.Decimal `json:"e"`
	Liability decimal.Decimal `json:"l"`
	UTime     int64           `json:"U,omitempty"`
}

// KucoinWsOrder 私有订单推送, 部分字段可能为空字符串
type KucoinWsOrder struct {
	OrderId     int64  `json:"O,omitempty"` // 推送里是数字
	ClientOid   string `json:"ci,omitempty"`
	Symbol      string `json:"s,omitempty"`
	TradeType   string `json:"tT,omitempty"`
	Side        string `json:"S,omitempty"`
	OrderType   string `json:"oT,omitempty"`
	Price       string `json:"p,omitempty"`
	Size        string `json:"q,omitempty"`
	SizeUnit    string `json:"qU,omitempty"`
	FilledSize  string `json:"f,omitempty"`
	AvgPrice    string `json:"aP,omitempty"`
	Fee         string `json:"fS,omitempty"`
	FeeCurrency string `json:"fC,omitempty"`
	RemainSize  string `json:"rS,omitempty"`
	CancelSize  string `json:"cS,omitempty"`
	TimeInForce string `json:"tIF,omitempty"`
	Status      int    `json:"os"`
	UTime       int64  `json:"U,omitempty"`
}

// toDecimal 空字符串返回0
func (v *KucoinWsOrder) toDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
