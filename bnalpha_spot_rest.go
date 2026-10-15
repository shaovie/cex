package cex

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (ba *BnAlpha) SpotSupported() bool {
	return true
}

// SpotLoadAllPairRule 交易对规则, symbol用标准格式BASEQUOTE(比如ZKJUSDT)
// alpha的交易对是ALPHA_<tokenId><quote>, 需要先用Token列表把 ALPHA_<tokenId> 还原成代币符号
func (ba *BnAlpha) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := baEndpoint + "/bapi/defi/v1/public/wallet-direct/buw/wallet/cex/alpha/all/token/list"
	_, resp, err := ba.Get(link, baRuleDeadline, nil)
	if err != nil {
		return nil, errors.New(ba.Name() + " net error! " + err.Error())
	}
	tokenRecv := struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    []struct {
			AlphaId string `json:"alphaId,omitempty"` // ALPHA_173
			Symbol  string `json:"symbol,omitempty"`  // ZKJ
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &tokenRecv); err != nil {
		return nil, errors.New(ba.Name() + " token unmarshal fail! " + err.Error())
	}
	if tokenRecv.Code != "000000" {
		return nil, ba.handleExceptionResp("SpotLoadAllPairRule", resp)
	}
	tokenMap := make(map[string]string, len(tokenRecv.Data))
	for _, v := range tokenRecv.Data {
		if len(v.AlphaId) == 0 || len(v.Symbol) == 0 {
			continue
		}
		tokenMap[v.AlphaId] = strings.ToUpper(v.Symbol)
	}

	link = baEndpoint + "/bapi/defi/v1/public/alpha-trade/get-exchange-info"
	_, resp, err = ba.Get(link, baRuleDeadline, nil)
	if err != nil {
		return nil, errors.New(ba.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    struct {
			Symbols []struct {
				Symbol  string `json:"symbol,omitempty"`     // ALPHA_173USDT
				Base    string `json:"baseAsset,omitempty"`  // ALPHA_173
				Quote   string `json:"quoteAsset,omitempty"` // USDT
				Status  string `json:"status,omitempty"`
				Filters []struct {
					FilterType string          `json:"filterType"`
					MaxPrice   decimal.Decimal `json:"maxPrice"`
					MinPrice   decimal.Decimal `json:"minPrice"`
					TickSize   decimal.Decimal `json:"tickSize"`

					MaxQty   decimal.Decimal `json:"maxQty"`
					MinQty   decimal.Decimal `json:"minQty"`
					StepSize decimal.Decimal `json:"stepSize"`

					MinNotional decimal.Decimal `json:"minNotional"`
				} `json:"filters,omitempty"`
			} `json:"symbols,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(ba.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != "000000" || len(recv.Data.Symbols) == 0 {
		return nil, ba.handleExceptionResp("SpotLoadAllPairRule", resp)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	symbolMap := make(map[string]string)
	symbolStdMap := make(map[string]string)
	for _, pair := range recv.Data.Symbols {
		if pair.Status != "TRADING" {
			continue
		}
		base := tokenMap[pair.Base]
		if len(base) == 0 { // 没查到代币符号, 拼不出标准symbol
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:   base + pair.Quote,
			Base:     base,
			Quote:    pair.Quote,
			Status:   "online",
			MaxPrice: decimal.NewFromFloat(999999999999.99),
			Time:     now,
		}
		for _, flt := range pair.Filters {
			if flt.FilterType == "PRICE_FILTER" {
				ep.MaxPrice = flt.MaxPrice
				ep.MinPrice = flt.MinPrice
				ep.PriceTickSize = flt.TickSize
			} else if flt.FilterType == "LOT_SIZE" {
				ep.MinOrderQty = flt.MinQty
				ep.MaxOrderQty = flt.MaxQty
				ep.QtyStep = flt.StepSize
			} else if flt.FilterType == "NOTIONAL" || flt.FilterType == "MIN_NOTIONAL" {
				ep.MinNotional = flt.MinNotional
			}
		}
		all[ep.Symbol] = ep
		symbolMap[ep.Symbol] = pair.Symbol    // XMNUSDT -> ALPHA_888USDT
		symbolStdMap[pair.Symbol] = ep.Symbol // ALPHA_888USDT -> XMNUSDT
	}
	baSpotSymbolMapMtx.Lock()
	baSpotSymbolMap = symbolMap
	baSpotSymbolStdMap = symbolStdMap
	baSpotSymbolMapMtx.Unlock()
	return all, nil
}

// SpotGetBBO 最优买卖价
func (ba *BnAlpha) SpotGetBBO(symbol string) (BestBidAsk, error) {
	symbolS := ba.getSpotSymbol(symbol)
	if len(symbolS) == 0 { // 规则没加载时按原样试一次
		symbolS = symbol
	}
	link := baEndpoint + "/bapi/defi/v1/public/alpha-trade/book-ticker?symbol=" + symbolS
	_, resp, err := ba.Get(link, baApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(ba.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    struct {
			Symbol   string          `json:"symbol,omitempty"`
			BidPrice decimal.Decimal `json:"bidPrice"`
			BidQty   decimal.Decimal `json:"bidQty"`
			AskPrice decimal.Decimal `json:"askPrice"`
			AskQty   decimal.Decimal `json:"askQty"`
			Time     int64           `json:"time,omitempty"` // msec
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return BestBidAsk{}, errors.New(ba.Name() + " unmarshal error! " + err.Error())
	}
	if ret.Code != "000000" || len(ret.Data.Symbol) == 0 {
		return BestBidAsk{}, ba.handleExceptionResp("SpotGetBBO", resp)
	}
	return BestBidAsk{
		Symbol:   symbol,
		Time:     ret.Data.Time,
		BidPrice: ret.Data.BidPrice,
		BidQty:   ret.Data.BidQty,
		AskPrice: ret.Data.AskPrice,
		AskQty:   ret.Data.AskQty,
	}, nil
}
