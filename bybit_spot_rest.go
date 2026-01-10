package cex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bb *Bybit) SpotSupported() bool {
	return true
}
func (bb *Bybit) SpotServerTime() (int64, error) {
	url := bbUniEndpoint + "/v5/market/time"
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Result struct {
			Time int64 `json:"timeNano,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Result.Time / 1000000, nil
}
func (bb *Bybit) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := bbUniEndpoint + "/v5/market/instruments-info?category=spot"
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol        string `json:"symbol,omitempty"`
				Base          string `json:"baseCoin,omitempty"`
				Quote         string `json:"quoteCoin,omitempty"`
				Status        string `json:"status,omitempty"`
				LotSizeFilter struct {
					MaxQty      decimal.Decimal `json:"maxOrderQty,omitempty"`
					MinQty      decimal.Decimal `json:"minOrderQty,omitempty"`
					MinAmt      decimal.Decimal `json:"minOrderAmt,omitempty"`
					MaxAmt      decimal.Decimal `json:"maxOrderAmt,omitempty"`
					StepSize    decimal.Decimal `json:"basePrecision,omitempty"`
					AmtStepSize decimal.Decimal `json:"quotePrecision,omitempty"`
				} `json:"lotSizeFilter,omitempty"`
				PriceFilter struct {
					TickSize decimal.Decimal `json:"tickSize,omitempty"`
				} `json:"priceFilter,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Result.List {
		if pair.Status != "Trading" {
			continue
		}

		ep := &SpotExchangePairRule{
			Symbol:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
			PriceTickSize: pair.PriceFilter.TickSize,
			MaxOrderQty:   pair.LotSizeFilter.MaxQty,
			MinOrderQty:   pair.LotSizeFilter.MinQty,
			QtyStep:       pair.LotSizeFilter.StepSize,
			MinNotional:   pair.LotSizeFilter.MinAmt,
			Time:          now,
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (bb *Bybit) SpotGetBBO(symbol string) (BestBidAsk, error) {
	url := bbUniEndpoint + "/v5/market/tickers?category=spot&symbol=" + symbol
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Time   int64  `json:"time,omitempty"`
		Result struct {
			List []struct {
				BidPrice decimal.Decimal `json:"bid1Price"`
				BidQty   decimal.Decimal `json:"bid1Size"`
				AskPrice decimal.Decimal `json:"ask1Price"`
				AskQty   decimal.Decimal `json:"ask1Size"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return BestBidAsk{}, errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if len(ret.Result.List) == 0 {
		return BestBidAsk{}, errors.New(bb.Name() + " resp empty")
	}
	bbo := &(ret.Result.List[0])
	return BestBidAsk{
		Symbol:   symbol,
		BidPrice: bbo.BidPrice,
		BidQty:   bbo.BidQty,
		AskPrice: bbo.AskPrice,
		AskQty:   bbo.AskQty,
	}, nil
}
