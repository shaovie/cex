package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type OkxOrderBook struct {
	Time string               `json:"ts,omitempty"`
	Bids [][2]decimal.Decimal `json:"bids,omitempty"` // 只解析前2个成员
	Asks [][2]decimal.Decimal `json:"asks,omitempty"`
}
type OkxOrderBooks struct {
	Data []OkxOrderBook `json:"data,omitempty"`
}
type OkxWsPubMsg struct {
	Arg struct {
		Channel string `json:"channel,omitempty"`
		Symbol  string `json:"instId,omitempty"`
	} `json:"arg,omitempty"`
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (v *OkxWsPubMsg) reset() {
	v.Data = nil
	v.Arg.Channel = ""
	v.Event = ""
}

type Okx24hTicker struct {
	Symbol      string          `json:"instId"`
	Last        decimal.Decimal `json:"last"`
	Volume      decimal.Decimal `json:"vol24h"`
	QuoteVolume decimal.Decimal `json:"volCcy24h"`
}
type Okx24hTickers struct {
	Code string         `json:"code,omitempty"`
	Msg  string         `json:"msg,omitempty"`
	Data []Okx24hTicker `json:"data,omitempty"`
}
type OkxFundingRates struct {
	Code string `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
	Data []struct {
		Symbol   string          `json:"instId"`
		InstType string          `json:"instType"`
		NextTime string          `json:"nextFundingTime,omitempty"` // msec
		Fr       decimal.Decimal `json:"fundingRate,omitempty"`
	} `json:"data,omitempty"`
}
type OkxKLine struct {
	Code string      `json:"code,omitempty"`
	Msg  string      `json:"msg,omitempty"`
	Data [][9]string `json:"data,omitempty"`
}
