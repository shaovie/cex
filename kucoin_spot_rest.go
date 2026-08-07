package cex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (kc *Kucoin) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := kcSpotEndpoint + "/api/ua/v1/market/instrument?tradeType=SPOT"
	_, resp, err := ihttp.Get(link, kcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			List []struct {
				Symbol      string          `json:"symbol"`        // BTC-USDT
				Base        string          `json:"baseCurrency"`  // BTC
				Quote       string          `json:"quoteCurrency"` // USDT
				Status      string          `json:"tradingStatus"`
				TickSz      decimal.Decimal `json:"tickSize"`
				LotSz       decimal.Decimal `json:"baseOrderStep"`
				MinOrderQty decimal.Decimal `json:"minBaseOrderSize"`
				MaxOrderQty decimal.Decimal `json:"maxBaseOrderSize"`
				MinNotional decimal.Decimal `json:"minFunds"`
			} `json:"list"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tkcSpotSymbolMap := make(map[string]string)
	for _, pair := range ret.Data.List {
		if pair.Status != "1" { // open for trading
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        pair.Base + pair.Quote,
			Base:          pair.Base,
			Quote:         pair.Quote,
			Status:        "online",
			PriceTickSize: pair.TickSz,
			QtyStep:       pair.LotSz,
			MinOrderQty:   pair.MinOrderQty,
			MaxOrderQty:   pair.MaxOrderQty,
			MaxPrice:      decimal.NewFromFloat(999999999999.99),
			MinNotional:   pair.MinNotional,
			Time:          now,
		}
		ep.MinPrice = ep.PriceTickSize
		all[ep.Symbol] = ep
		tkcSpotSymbolMap[ep.Symbol] = pair.Symbol
	}

	kcSpotSymbolMapMtx.Lock()
	kcSpotSymbolMap = tkcSpotSymbolMap
	kcSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (kc *Kucoin) SpotGetBBO(symbol string) (BestBidAsk, error) {
	symbolS := kc.getSpotSymbol(symbol)
	url := kcSpotEndpoint + "/api/ua/v1/market/ticker?tradeType=SPOT&symbol=" + symbolS
	_, resp, err := ihttp.Get(url, kcApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(kc.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			List []struct {
				BestBid     decimal.Decimal `json:"bestBidPrice"`
				BestBidSize decimal.Decimal `json:"bestBidSize"`
				BestAsk     decimal.Decimal `json:"bestAskPrice"`
				BestAskSize decimal.Decimal `json:"bestAskSize"`
			} `json:"list"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return BestBidAsk{}, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Data.List) > 0 {
		return BestBidAsk{
			Symbol:   symbol,
			BidPrice: recv.Data.List[0].BestBid,
			BidQty:   recv.Data.List[0].BestBidSize,
			AskPrice: recv.Data.List[0].BestAsk,
			AskQty:   recv.Data.List[0].BestAskSize,
		}, nil
	}
	return BestBidAsk{}, errors.New(kc.Name() + " resp empty!")
}
