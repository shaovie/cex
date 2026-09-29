package cex

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (sa *Safetrade) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := stEndpoint + "/trade/public/markets"
	_, resp, err := sa.Get(link, stApiDeadline, stHttpHeaders)
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}

	ret := []struct {
		Id              string          `json:"id"` // 交易所内部symbol, 如 btcusdt
		Base            string          `json:"base_unit"`
		Quote           string          `json:"quote_unit"`
		State           string          `json:"state"`
		AmountPrecision int32           `json:"amount_precision"`
		PricePrecision  int32           `json:"price_precision"`
		MinPrice        decimal.Decimal `json:"min_price"`
		MaxPrice        decimal.Decimal `json:"max_price"`
		MinAmount       decimal.Decimal `json:"min_amount"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tSymbolMap := make(map[string]string)
	for _, m := range ret {
		if m.State != "enabled" { // open for trading
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        strings.ToUpper(m.Id),
			Base:          strings.ToUpper(m.Base),
			Quote:         strings.ToUpper(m.Quote),
			Status:        "online",
			PriceTickSize: PowOneTenth(int(m.PricePrecision)),
			QtyStep:       PowOneTenth(int(m.AmountPrecision)),
			MinPrice:      m.MinPrice,
			MaxPrice:      m.MaxPrice,
			MinOrderQty:   m.MinAmount,
			MaxOrderQty:   decimal.NewFromFloat(999999999999.99),
			Time:          now,
		}
		all[ep.Symbol] = ep
		tSymbolMap[ep.Symbol] = m.Id
	}

	stSpotSymbolMapMtx.Lock()
	stSpotSymbolMap = tSymbolMap
	stSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (sa *Safetrade) SpotGetBBO(symbol string) (BestBidAsk, error) {
	id := sa.getSpotSymbol(symbol)
	if id == "" {
		return BestBidAsk{}, errors.New(sa.Name() + " unknown symbol! " + symbol)
	}
	link := stEndpoint + "/trade/public/markets/" + id + "/depth?limit=1"
	_, resp, err := sa.Get(link, stApiDeadline, stHttpHeaders)
	if err != nil {
		return BestBidAsk{}, errors.New(sa.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Asks [][2]decimal.Decimal `json:"asks"`
		Bids [][2]decimal.Decimal `json:"bids"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return BestBidAsk{}, errors.New(sa.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Bids) > 0 && len(recv.Asks) > 0 {
		return BestBidAsk{
			Symbol:   symbol,
			BidPrice: recv.Bids[0][0],
			BidQty:   recv.Bids[0][1],
			AskPrice: recv.Asks[0][0],
			AskQty:   recv.Asks[0][1],
		}, nil
	}
	return BestBidAsk{}, errors.New(sa.Name() + " resp empty!")
}
