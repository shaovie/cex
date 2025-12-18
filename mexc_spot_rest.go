package cex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (mc *Mexc) SpotSupported() bool {
	return true
}
func (mc *Mexc) SpotServerTime() (int64, error) {
	url := mcUniEndpoint + "/api/v3/time"
	_, resp, err := ihttp.Get(url, mcApiDeadline, nil)
	if err != nil {
		return 0, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"serverTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (mc *Mexc) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := mcUniEndpoint + "/api/v3/exchangeInfo"
	_, resp, err := ihttp.Get(url, mcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Symbols []struct {
			Symbol string `json:"symbol,omitempty"`
			Base   string `json:"baseAsset,omitempty"`
			Quote  string `json:"quoteAsset,omitempty"`
			Status string `json:"status"` // 1 - 开放， 2 - 暂停， 3 - 下线

			StepSize decimal.Decimal `json:"quoteAmountPrecision,omitempty"`
			TickSize decimal.Decimal `json:"baseSizePrecision,omitempty"`
		} `json:"symbols,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Symbols {
		if pair.Status != "1" {
			continue
		}

		ep := &SpotExchangePairRule{
			Symbol: pair.Symbol,
			Base:   pair.Base,
			Quote:  pair.Quote,
			Time:   now,
		}
		ep.MinPrice = decimal.NewFromFloat(0.000000001)
		ep.MaxPrice = decimal.NewFromFloat(999999999)
		ep.MinOrderQty = pair.TickSize
		ep.MaxOrderQty = decimal.NewFromFloat(999999999)
		ep.QtyStep = pair.TickSize
		// ep.PriceTickSize =
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (mc *Mexc) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	url := mcUniEndpoint + "/api/v3/ticker/24hr"
	_, resp, err := ihttp.Get(url, mcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, mc.handleExceptionResp("SpotGetAll24hTicker", resp)
	}
	tickers := []struct {
		Symbol      string          `json:"symbol"`
		Last        decimal.Decimal `json:"lastPrice"`
		Volume      decimal.Decimal `json:"volume"`
		QuoteVolume decimal.Decimal `json:"quoteVolume"`
	}{}
	if err = json.Unmarshal(resp, &tickers); err != nil {
		return nil, errors.New(mc.Name() + " Unmarshal err! " + err.Error())
	}
	if len(tickers) == 0 {
		return nil, errors.New(mc.Name() + " resp empty")
	}
	allTk := make(map[string]Pub24hTicker, len(tickers))
	for _, tk := range tickers {
		v := Pub24hTicker{
			Symbol:      tk.Symbol,
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
