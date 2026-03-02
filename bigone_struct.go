package cex

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

type BigoneSpotBBO struct {
	Ticker struct {
		Symbol string `json:"market"`
		Bid    struct {
			Price decimal.Decimal `json:"price"`
			Qty   decimal.Decimal `json:"amount"`
		} `json:"bid"`
		Ask struct {
			Price decimal.Decimal `json:"price"`
			Qty   decimal.Decimal `json:"amount"`
		} `json:"ask"`
	} `json:"ticker"`
}

func (v *BigoneSpotBBO) reset() {
	v.Ticker.Symbol = ""
	v.Ticker.Bid.Price = decimal.Zero
	v.Ticker.Ask.Price = decimal.Zero
}

type BigoneSpotBBOs struct {
	Tickers []struct {
		Symbol string `json:"market"`
		Bid    struct {
			Price decimal.Decimal `json:"price"`
			Qty   decimal.Decimal `json:"amount"`
		} `json:"bid"`
		Ask struct {
			Price decimal.Decimal `json:"price"`
			Qty   decimal.Decimal `json:"amount"`
		} `json:"ask"`
	} `json:"tickers"`
}

func (v *BigoneSpotBBOs) reset() {
	v.Tickers = v.Tickers[:0]
}

type BigoneSpotWsPubMsg struct {
	Err struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	} `json:"error"`

	DepthSnap    json.RawMessage `json:"depthSnapshot"`
	DepthUpdate  json.RawMessage `json:"depthUpdate"`
	TickerSnap   json.RawMessage `json:"tickersSnapshot"`
	TickerUpdate json.RawMessage `json:"tickerUpdate"`
	TradeSnap    json.RawMessage `json:"tradesSnapshot"`
	TradeUpdate  json.RawMessage `json:"tradeUpdate"`
}

func (v *BigoneSpotWsPubMsg) reset() {
	v.Err.Code = 0
	v.Err.Msg = ""
	v.DepthSnap = nil
	v.DepthUpdate = nil
	v.TickerSnap = nil
	v.TickerUpdate = nil
	v.TradeSnap = nil
	v.TradeUpdate = nil
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
	Depth    BigoneSpotOrderBookDepth `json:"depth"`
	ChangeId string                   `json:"changeId,omitempty"`
	PrevId   string                   `json:"prevId,omitempty"`
}
type BigoneSpotWsPrivMsg struct {
	Err struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	} `json:"error"`

	OrderUpdate   json.RawMessage `json:"orderUpdate"`
	AccountSnap   json.RawMessage `json:"accountsSnapshot"`
	AccountUpdate json.RawMessage `json:"accountUpdate"`
}

func (v *BigoneSpotWsPrivMsg) reset() {
	v.Err.Code = 0
	v.Err.Msg = ""
	v.OrderUpdate = nil
	v.AccountSnap = nil
	v.AccountUpdate = nil
}
