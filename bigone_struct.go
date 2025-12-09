package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type BigoneSpot24hTicker struct {
	Code int    `json:"code,omitempty"`
	Msg  string `json:"message,omitempty"`
	Data []struct {
		Symbol string `json:"asset_pair_name"`
		Bid    struct {
			Price decimal.Decimal `json:"price"`
			//Qty        decimal.Decimal `json:"quantity"`
		} `json:"bid"`
		Ask struct {
			Price decimal.Decimal `json:"price"`
			//Qty        decimal.Decimal `json:"quantity"`
		} `json:"ask"`
		Volume      decimal.Decimal `json:"base_volume"`
		QuoteVolume decimal.Decimal `json:"quote_volume"`
	} `json:"data,omitempty"`
}
type BigoneSpotWsPubMsg struct {
	Err struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
	} `json:"error,omitempty"`

	DepthSnap    json.RawMessage `json:"depthSnapshot,omitempty"`
	DepthUpdate  json.RawMessage `json:"depthUpdate,omitempty"`
	TickerSnap   json.RawMessage `json:"tickersSnapshot,omitempty"`
	TickerUpdate json.RawMessage `json:"tickerUpdate,omitempty"`
}

func (v *BigoneSpotWsPubMsg) reset() {
	v.Err.Code = 0
	v.Err.Msg = ""
	v.DepthSnap = nil
	v.DepthUpdate = nil
	v.TickerSnap = nil
	v.TickerUpdate = nil
}

type BigoneSpotOrderBookItem struct {
	Price decimal.Decimal `json:"price"`
	Qty   string          `json:"amount"`
}
type BigoneSpotOrderBookDepth struct {
	Symbol string                    `json:"market"`
	Bids   []BigoneSpotOrderBookItem `json:"bids"`
	Asks   []BigoneSpotOrderBookItem `json:"asks"`
}
type BigoneSpotOrderBook struct {
	Depth    BigoneSpotOrderBookDepth `json:"depth,omitempty"`
	ChangeId string                   `json:changeId,omitempty`
	PrevId   string                   `json:prevId,omitempty`
}
type BigoneSpotWsPrivMsg struct {
	Err struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
	} `json:"error,omitempty"`

	OrderUpdate   json.RawMessage `json:"orderUpdate,omitempty"`
	AccountSnap   json.RawMessage `json:"accountsSnapshot,omitempty"`
	AccountUpdate json.RawMessage `json:"accountUpdate,omitempty"`
}

func (v *BigoneSpotWsPrivMsg) reset() {
	v.Err.Code = 0
	v.Err.Msg = ""
	v.OrderUpdate = nil
	v.AccountSnap = nil
	v.AccountUpdate = nil
}
