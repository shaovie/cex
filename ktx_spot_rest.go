package cex

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (ktx *Ktx) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := ktxSpotEndpoint + "/v1/products?market=spot"
	_, resp, err := ihttp.Get(link, ktxApiDeadline, nil)
	if err != nil {
		return nil, errors.New(ktx.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Result []struct {
			Symbol      string          `json:"symbol"`
			Status      int             `json:"active"`
			TickSz      decimal.Decimal `json:"priceIncrement"`
			LotSz       decimal.Decimal `json:"quantityIncrement"`
			MinOrderQty decimal.Decimal `json:"minOrderSize"`
			MinNotional decimal.Decimal `json:"minOrderValue"`
		} `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ktx.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tktxSpotSymbolMap := make(map[string]string)
	for _, pair := range ret.Result {
		if pair.Status != 1 { // open for trading
			continue
		}
		base, quote, ok := strings.Cut(pair.Symbol, "_")
		if !ok {
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        base + quote,
			Base:          base,
			Quote:         quote,
			Status:        "online",
			PriceTickSize: pair.TickSz,
			QtyStep:       pair.LotSz,
			MinOrderQty:   pair.MinOrderQty,
			MaxOrderQty:   decimal.NewFromFloat(999999999999.99),
			MaxPrice:      decimal.NewFromFloat(999999999999.99),
			MinNotional:   pair.MinNotional,
			Time:          now,
		}
		ep.MinPrice = ep.PriceTickSize
		all[ep.Symbol] = ep
		tktxSpotSymbolMap[ep.Symbol] = pair.Symbol
	}

	ktxSpotSymbolMapMtx.Lock()
	ktxSpotSymbolMap = tktxSpotSymbolMap
	ktxSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (ktx *Ktx) SpotGetBBO(symbol string) (BestBidAsk, error) {
	symbolS := ktx.getSpotSymbol(symbol)
	url := ktxSpotEndpoint + "/v1/order_book?market=spot&level=1&symbol=" + symbolS
	_, resp, err := ihttp.Get(url, ktxApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(ktx.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Result struct {
			Asks [][2]decimal.Decimal `json:"a"`
			Bids [][2]decimal.Decimal `json:"b"`
		} `json:"result"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return BestBidAsk{}, errors.New(ktx.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Result.Asks) > 0 && len(recv.Result.Bids) > 0 {
		return BestBidAsk{
			Symbol:   symbol,
			BidPrice: recv.Result.Bids[0][0],
			BidQty:   recv.Result.Bids[0][1],
			AskPrice: recv.Result.Asks[0][0],
			AskQty:   recv.Result.Asks[0][1],
		}, nil
	}
	return BestBidAsk{}, errors.New(ktx.Name() + " resp empty!")
}
