package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

func (bn *Binance) FuturesSupported(typ string) bool {
	if typ == "UM" || typ == "CM" {
		return true
	}
	return false
}
func (bn *Binance) FuturesServerTime(typ string) (int64, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/time"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/time"
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bn.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"serverTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (bn *Binance) FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal {
	return size
}
func (bn *Binance) FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal {
	return qty
}
func (bn *Binance) FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/exchangeInfo"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/exchangeInfo"
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " " + typ + " net error! " + err.Error())
	}

	recv := struct {
		Code    int    `json:"code,omitempty"`
		Msg     string `json:"msg,omitempty"`
		Symbols []struct {
			Symbol         string          `json:"symbol,omitempty"`
			Pair           string          `json:"pair,omitempty"`
			Quote          string          `json:"quoteAsset,omitempty"`
			Base           string          `json:"baseAsset,omitempty"`
			Status         string          `json:"status,omitempty"`
			ContractType   string          `json:"contractType,omitempty"`
			ContractStatus string          `json:"contractStatus,omitempty"` // for CM
			ContractSize   decimal.Decimal `json:"contractSize,omitempty"`   // for CM
			Filters        []struct {
				FilterType  string          `json:"filterType"`
				MaxPrice    decimal.Decimal `json:"maxPrice,omitempty"`
				MinPrice    decimal.Decimal `json:"minPrice,omitempty"`
				MaxQty      decimal.Decimal `json:"maxQty,omitempty"`
				MinQty      decimal.Decimal `json:"minQty,omitempty"`
				TickSize    decimal.Decimal `json:"tickSize,omitempty"`
				StepSize    decimal.Decimal `json:"stepSize,omitempty"`
				MinNotional decimal.Decimal `json:"notional,omitempty"` // for UM
			}
		} `json:"symbols"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bn.Name() + " api err! " + recv.Msg)
	}
	all := make(map[string]*FuturesExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Symbols {
		if (typ == "UM" && pair.Status != "TRADING") ||
			(typ == "CM" && pair.ContractStatus != "TRADING") {
			continue
		}

		ep := &FuturesExchangePairRule{
			Typ:          typ,
			Symbol:       pair.Pair,
			Quote:        pair.Quote,
			Base:         pair.Base,
			ContractSize: pair.ContractSize,
			Time:         now,
		}
		if pair.ContractType != "PERPETUAL" { // e.g. BTCUSD_240925
			ep.Symbol = pair.Symbol
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
			} else if flt.FilterType == "MIN_NOTIONAL" { // for UM
				ep.MinNotional = flt.MinNotional
			}
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (bn *Binance) FuturesGetAll24hTicker(typ string) (map[string]Pub24hTicker, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/ticker/24hr"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/ticker/24hr"
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("FuturesGetAll24hTicker", resp)
	}

	tks := []struct {
		Symbol      string          `json:"symbol"`
		Last        decimal.Decimal `json:"lastPrice"`
		Volume      decimal.Decimal `json:"volume"`
		QuoteVolume decimal.Decimal `json:"quoteVolume,omitempty"`
		BaseVolume  decimal.Decimal `json:"baseVolume,omitempty"`
	}{}

	err = json.Unmarshal(resp, &tks)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	allTk := make(map[string]Pub24hTicker, len(tks))
	for _, tk := range tks {
		if typ == "CM" {
			tk.Symbol = strings.ReplaceAll(tk.Symbol, "_PERP", "")
		}
		v := Pub24hTicker{
			Symbol:      tk.Symbol,
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
			BaseVolume:  tk.BaseVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (bn *Binance) FuturesGetBBO(typ, symbol string) (BestBidAsk, error) {
	if typ == "UM" {
		url := bnUMFuturesEndpoint + "/fapi/v1/ticker/bookTicker?symbol=" + symbol
		_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
		if err != nil {
			return BestBidAsk{}, errors.New(bn.Name() + " net error! " + err.Error())
		}
		bbo := struct {
			Symbol   string          `json:"symbol,omitempty"`
			BidPrice decimal.Decimal `json:"bidPrice,omitempty"`
			BidQty   decimal.Decimal `json:"bidQty,omitempty"`
			AskPrice decimal.Decimal `json:"askPrice,omitempty"`
			AskQty   decimal.Decimal `json:"askQty,omitempty"`
		}{}
		if err = json.Unmarshal(resp, &bbo); err != nil {
			return BestBidAsk{}, errors.New(bn.Name() + " Unmarshal err! " + err.Error())
		}
		return BestBidAsk{
			Symbol:   bbo.Symbol,
			BidPrice: bbo.BidPrice,
			BidQty:   bbo.BidQty,
			AskPrice: bbo.AskPrice,
			AskQty:   bbo.AskQty,
		}, nil
	}
	if typ == "CM" {
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
		url := bnCMFuturesEndpoint + "/dapi/v1/ticker/bookTicker?symbol=" + symbol
		_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
		if err != nil {
			return BestBidAsk{}, errors.New(bn.Name() + " net error! " + err.Error())
		}
		bboL := []struct {
			Symbol   string          `json:"symbol,omitempty"`
			BidPrice decimal.Decimal `json:"bidPrice,omitempty"`
			BidQty   decimal.Decimal `json:"bidQty,omitempty"`
			AskPrice decimal.Decimal `json:"askPrice,omitempty"`
			AskQty   decimal.Decimal `json:"askQty,omitempty"`
		}{}
		if err = json.Unmarshal(resp, &bboL); err != nil {
			return BestBidAsk{}, errors.New(bn.Name() + " Unmarshal err! " + err.Error())
		}
		if len(bboL) == 0 {
			return BestBidAsk{}, errors.New(bn.Name() + " resp empty!")
		}
		bbo := bboL[0]
		return BestBidAsk{
			Symbol:   bbo.Symbol,
			BidPrice: bbo.BidPrice,
			BidQty:   bbo.BidQty,
			AskPrice: bbo.AskPrice,
			AskQty:   bbo.AskQty,
		}, nil
	}
	return BestBidAsk{}, errors.New("not support")
}
func (bn *Binance) FuturesGetAllFundingRate(typ string) (map[string]FundingRate, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/premiumIndex"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/premiumIndex"
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("FuturesGetAllFundingRate", resp)
	}

	frs := []struct {
		Symbol   string          `json:"symbol"`
		Fr       decimal.Decimal `json:"lastFundingRate"`
		NextTime int64           `json:"nextFundingTime"` // msec
	}{}

	err = json.Unmarshal(resp, &frs)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	all := make(map[string]FundingRate, len(frs))
	now := time.Now().Unix()
	for _, fr := range frs {
		if typ == "CM" {
			fr.Symbol = strings.ReplaceAll(fr.Symbol, "_PERP", "")
		}
		v := FundingRate{
			Symbol:   fr.Symbol,
			Val:      fr.Fr,
			UTime:    now,
			NextTime: fr.NextTime / 1000,
		}
		all[v.Symbol] = v
	}
	return all, nil
}
func (bn *Binance) FuturesGetFundingRateMarkPrice(typ, symbol string) (FundingRateMarkPrice, error) {
	if typ == "UM" {
		url := bnUMFuturesEndpoint + "/fapi/v1/premiumIndex?symbol=" + symbol
		_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
		if err != nil {
			return FundingRateMarkPrice{}, errors.New(bn.Name() + " net error! " + err.Error())
		}

		fr := struct {
			MarkPrice decimal.Decimal `json:"markPrice"`
			Fr        decimal.Decimal `json:"lastFundingRate"`
			NextTime  int64           `json:"nextFundingTime"` // msec
		}{}

		if err = json.Unmarshal(resp, &fr); err != nil {
			return FundingRateMarkPrice{}, errors.New(bn.Name() + " unmarshal error! " + err.Error())
		}
		return FundingRateMarkPrice{
			MarkPrice:   fr.MarkPrice,
			FundingRate: fr.Fr,
			NextTime:    fr.NextTime,
		}, nil
	} else if typ == "CM" {
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
		url := bnCMFuturesEndpoint + "/dapi/v1/premiumIndex?symbol=" + symbol
		_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
		if err != nil {
			return FundingRateMarkPrice{}, errors.New(bn.Name() + " net error! " + err.Error())
		}
		if resp[0] != '[' {
			return FundingRateMarkPrice{}, bn.handleExceptionResp("FuturesGetFundingRateMarkPrice", resp)
		}

		frs := []struct {
			MarkPrice decimal.Decimal `json:"markPrice"`
			Fr        decimal.Decimal `json:"lastFundingRate"`
			NextTime  int64           `json:"nextFundingTime"` // msec
		}{}

		if err = json.Unmarshal(resp, &frs); err != nil {
			return FundingRateMarkPrice{}, errors.New(bn.Name() + " unmarshal error! " + err.Error())
		}
		if len(frs) == 0 {
			return FundingRateMarkPrice{}, errors.New(bn.Name() + " resp empty")
		}
		return FundingRateMarkPrice{
			MarkPrice:   frs[0].MarkPrice,
			FundingRate: frs[0].Fr,
			NextTime:    frs[0].NextTime,
		}, nil
	}
	return FundingRateMarkPrice{}, errors.New("not support")
}
func (bn *Binance) FuturesGetAllAssets(typ string) (map[string]*FuturesAsset, error) {
	url := bnUMFuturesEndpoint + "/fapi/v3/balance"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/balance"
	}
	url += "?" + bn.httpQuerySign("")
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	alls := []struct {
		Symbol       string          `json:"asset,omitempty"`
		Total        decimal.Decimal `json:"balance,omitempty"`
		AvailBalance decimal.Decimal `json:"availableBalance,omitempty"` // 可用下单余额
	}{}
	err = json.Unmarshal(resp, &alls)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}

	assetsMap := make(map[string]*FuturesAsset, len(alls))
	for _, v := range alls {
		if v.Total.IsZero() && v.AvailBalance.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &FuturesAsset{
			Symbol: v.Symbol,
			Avail:  v.AvailBalance,
			Locked: v.Total.Sub(v.AvailBalance),
			Total:  v.Total,
		}
	}
	return assetsMap, nil
}
func (bn *Binance) FuturesGetKLine(typ, symbol, interval string,
	startTime, endTime, limit int64) ([]KLine, error) {
	params := fmt.Sprintf("symbol=%s&interval=%s&startTime=%d&limit=%d",
		symbol, interval, startTime*1000, limit)
	if endTime > 0 {
		params += fmt.Sprintf("&endTime=%d", endTime*1000-1)
	}
	url := bnUMFuturesEndpoint + "/fapi/v1/klines?" + params
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/klines?" + params
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("FuturesGetKLine", resp)
	}

	var klines [][]any
	err = json.Unmarshal(resp, &klines)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	all := make([]KLine, 0, len(klines))
	for _, v := range klines {
		kl := KLine{}
		kl.OpenTime = int64(v[0].(float64)) / 1000
		kl.OpenPrice, _ = decimal.NewFromString(v[1].(string))
		kl.HighPrice, _ = decimal.NewFromString(v[2].(string))
		kl.LowPrice, _ = decimal.NewFromString(v[3].(string))
		kl.ClosePrice, _ = decimal.NewFromString(v[4].(string))
		kl.Volume, _ = decimal.NewFromString(v[5].(string))
		kl.QuoteVolume, _ = decimal.NewFromString(v[7].(string))

		all = append(all, kl)
	}
	return all, nil
}
func (bn *Binance) FuturesPlaceOrder(typ, symbol, clientId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, orderType, timeInForce string,
	positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error) {
	if typ == "CM" {
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
	}
	if !qty.IsPositive() {
		return "", errors.New("qty too small! qty=" + qty.String())
	}
	query := fmt.Sprintf("&newOrderRespType=ACK&symbol=%s&side=%s&type=%s&quantity=%s",
		symbol, side, orderType, qty.String())
	if orderType == "LIMIT" {
		query += "&timeInForce=" + timeInForce + "&price=" + price.String()
	} else if orderType == "MARKET" {
	} else {
		return "", errors.New("not support order type:" + orderType)
	}
	if positionMode == 0 {
		query += "&positionSide=" + "BOTH"
	}
	if reduceOnly == 1 {
		query += "&reduceOnly=true" // 双开模式下不接受此参数
	}
	link := bnUMFuturesEndpoint + "/fapi/v1/order?" + bn.httpQuerySign(query)
	if typ == "CM" {
		link = bnCMFuturesEndpoint + "/dapi/v1/order?" + bn.httpQuerySign(query)
	}
	if bn.isUnified {
		link = bnUnifiedEndpoint + "/papi/v1/um/order?" + bn.httpQuerySign(query)
		if typ == "CM" {
			link = bnUnifiedEndpoint + "/papi/v1/cm/order?" + bn.httpQuerySign(query)
		}
	}
	_, resp, err := ihttp.Post(link, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return "", errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int    `json:"code,omitempty"`
		Msg     string `json:"msg,omitempty"`
		OrderId int64  `json:"orderId,omitempty"` //
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return "", errors.New(bn.Name() + " futures order fail! " + ret.Msg + " " + link)
	}
	return strconv.FormatInt(ret.OrderId, 10), nil
}
func (bn *Binance) FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/order"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/order"
	}
	if bn.isUnified {
		url = bnUnifiedEndpoint + "/papi/v1/um/order"
		if typ == "CM" {
			url = bnUnifiedEndpoint + "/papi/v1/cm/order"
		}
	}

	if typ == "CM" && strings.Index(symbol, "_") == -1 {
		symbol += "_PERP"
	}
	params := fmt.Sprintf("&symbol=%s&orderId=%s", symbol, orderId)
	if orderId == "" && cltId != "" {
		params = fmt.Sprintf("&symbol=%s&origClientOrderId=%s", symbol, cltId)
	}
	url += "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	order := struct {
		Code         int             `json:"code,omitempty"`
		Msg          string          `json:"msg,omitempty"`
		Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId      int64           `json:"orderId,omitempty"`
		ClientId     string          `json:"clientOrderId,omitempty"` // BTCUSDT
		Price        decimal.Decimal `json:"price,omitempty"`
		Quantity     decimal.Decimal `json:"origQty,omitempty"`     // 用户设置的原始订单数量
		ExecutedQty  decimal.Decimal `json:"executedQty,omitempty"` // 交易的订单数量
		CummQuoteQty decimal.Decimal `json:"cumQuote,omitempty"`    // 累计交易的金额 for UM
		CummBaseQty  decimal.Decimal `json:"cumBase,omitempty"`     // 累计交易的金额(标地数量) for CM
		AvgPrice     decimal.Decimal `json:"avgPrice,omitempty"`    // for CM
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`        // LIMIT/MARKET
		TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		Time         int64           `json:"time,omitempty"`
		UTime        int64           `json:"updateTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &order)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if order.Code != 0 {
		return nil, errors.New(order.Msg)
	}
	fo := &FuturesOrder{
		Symbol:    order.Symbol,
		OrderId:   strconv.FormatInt(order.OrderId, 10),
		ClientId:  order.ClientId,
		Price:     order.Price,
		Qty:       order.Quantity,
		FilledQty: order.ExecutedQty,
		FilledAmt: order.CummQuoteQty,
		Status:    order.Status,
		Type:      order.Type,
		Side:      order.Side,
		CTime:     order.Time,
		UTime:     order.UTime,
	}
	if typ == "CM" {
		fo.FilledAmt = order.CummBaseQty
		fo.AvgPrice = order.AvgPrice
	}
	return fo, nil
}
func (bn *Binance) FuturesGetOpenOrders(typ, symbol string) ([]*FuturesOrder, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/openOrders"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/openOrders"
	}
	url = bnUnifiedEndpoint + "/papi/v1/um/openOrders"
	if typ == "CM" {
		url = bnUnifiedEndpoint + "/papi/v1/cm/openOrders"
	}
	if typ == "CM" && strings.Index(symbol, "_") == -1 {
		symbol += "_PERP"
	}
	params := fmt.Sprintf("&symbol=%s", symbol)
	url += "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	orders := []struct {
		Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId      int64           `json:"orderId,omitempty"`
		ClientId     string          `json:"clientOrderId,omitempty"` // BTCUSDT
		Price        decimal.Decimal `json:"price,omitempty"`
		Quantity     decimal.Decimal `json:"origQty,omitempty"`     // 用户设置的原始订单数量
		ExecutedQty  decimal.Decimal `json:"executedQty,omitempty"` // 交易的订单数量
		CummQuoteQty decimal.Decimal `json:"cumQuote,omitempty"`    // 累计交易的金额 for UM
		CummBaseQty  decimal.Decimal `json:"cumBase,omitempty"`     // 累计交易的金额(标地数量) for CM
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`        // LIMIT/MARKET
		TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		Time         int64           `json:"time,omitempty"`
		UTime        int64           `json:"updateTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &orders)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if resp[0] == '{' {
		return nil, bn.handleExceptionResp("FuturesGetOpenOrders", resp)
	}
	oL := make([]*FuturesOrder, 0, len(orders))
	for _, order := range orders {
		fo := &FuturesOrder{
			Symbol:    order.Symbol,
			OrderId:   strconv.FormatInt(order.OrderId, 10),
			ClientId:  order.ClientId,
			Price:     order.Price,
			Qty:       order.Quantity,
			FilledQty: order.ExecutedQty,
			FilledAmt: order.CummQuoteQty,
			Status:    order.Status,
			Type:      order.Type,
			Side:      order.Side,
			CTime:     order.Time,
			UTime:     order.UTime,
		}
		if typ == "CM" {
			fo.FilledAmt = order.CummBaseQty
			fo.Symbol = strings.ReplaceAll(fo.Symbol, "_PERP", "")
		}
		oL = append(oL, fo)
	}
	return oL, nil
}
func (bn *Binance) FuturesCancelOrder(typ, symbol, orderId, cltId string) error {
	url := bnUMFuturesEndpoint + "/fapi/v1/order"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/order"
	}
	if bn.isUnified {
		url = bnUnifiedEndpoint + "/papi/v1/um/order"
		if typ == "CM" {
			url = bnUnifiedEndpoint + "/papi/v1/cm/order"
		}
	}
	if typ == "CM" && strings.Index(symbol, "_") == -1 {
		symbol += "_PERP"
	}
	params := "&symbol=" + symbol + "&orderId=" + orderId
	if orderId == "" && cltId != "" {
		params = "&symbol=" + symbol + "&origClientOrderId=" + cltId
	}
	url = url + "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Delete(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"code,omitempty"`
		Msg    string `json:"msg,omitempty"`
		Status string `json:"status,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(ret.Msg)
	}
	if ret.Status == "CANCELED" {
		return nil
	}
	return errors.New("cancel fail " + ret.Status)
}
func (bn *Binance) FuturesSwitchPositionMode(typ string /*BTCUSDT*/, mode int) error {
	m := ""
	if mode == 1 {
		m = "true"
	} else if mode == 0 {
		m = "false"
	}
	if m == "" {
		return errors.New("params error")
	}

	link := bnUMFuturesEndpoint + "/fapi/v1/positionSide/dual"
	if typ == "CM" {
		link = bnCMFuturesEndpoint + "/dapi/v1/positionSide/dual"
	}
	if bn.isUnified {
		link = bnUnifiedEndpoint + "/papi/v1/um/positionSide/dual"
		if typ == "CM" {
			link = bnUnifiedEndpoint + "/papi/v1/cm/positionSide/dual"
		}
	}
	params := "&dualSidePosition=" + m
	link += "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Post(link, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 200 && ret.Code != -4059 {
		return errors.New(ret.Msg)
	}
	return nil
}
func (bn *Binance) FuturesSwitchTradeMode(typ, symbol string, mode, leverage int) error {
	bn.futuresSwitchTradeMode(typ, symbol, mode)
	bn.futuresSetLeverage(typ, symbol, leverage)
	return nil
}
func (bn *Binance) futuresSwitchTradeMode(typ, symbol string /*BTCUSDT*/, mode int) error {
	if bn.isUnified { // 统一账户模式下不支持
		return nil
	}
	m := ""
	if mode == 1 {
		m = "ISOLATED"
	} else if mode == 0 {
		m = "CROSSED"
	}
	if m == "" {
		return errors.New("params error")
	}

	link := bnUMFuturesEndpoint + "/fapi/v1/marginType"
	if typ == "CM" {
		link = bnCMFuturesEndpoint + "/dapi/v1/marginType"
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
	}
	params := "&marginType=" + m + "&symbol=" + symbol
	link += "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Post(link, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 200 && ret.Code != -4046 {
		return errors.New(ret.Msg)
	}
	return nil
}
func (bn *Binance) futuresSetLeverage(typ, symbol string /*BTCUSDT*/, leverage int) error {
	if leverage < 1 || leverage > 125 {
		return errors.New("leverage error 1~125")
	}

	link := bnUMFuturesEndpoint + "/fapi/v1/leverage"
	if typ == "CM" {
		link = bnCMFuturesEndpoint + "/dapi/v1/leverage"
	}
	if bn.isUnified {
		link = bnUnifiedEndpoint + "/papi/v1/um/leverage"
		if typ == "CM" {
			link = bnUnifiedEndpoint + "/papi/v1/cm/leverage"
		}
	}
	if typ == "CM" && strings.Index(symbol, "_") == -1 {
		symbol += "_PERP"
	}
	ls := strconv.FormatInt(int64(leverage), 10)
	params := "&leverage=" + ls + "&symbol=" + symbol
	link += "?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Post(link, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 200 {
		return errors.New(ret.Msg)
	}
	return nil
}
func (bn *Binance) FuturesMaintMargin(typ, symbol string) ([]*FuturesLeverageBracket, error) {
	url := bnUMFuturesEndpoint + "/fapi/v1/leverageBracket"
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v2/leverageBracket"
	}
	if bn.isUnified {
		url = bnUnifiedEndpoint + "/papi/v1/um/leverageBracket"
		if typ == "CM" {
			url = bnUnifiedEndpoint + "/papi/v1/cm/leverageBracket"
		}
	}
	if typ == "CM" && strings.Index(symbol, "_") == -1 {
		symbol += "_PERP"
	}
	url = url + "?" + bn.httpQuerySign("&symbol="+symbol)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("FuturesMaintMargin", resp)
	}
	type Bracket struct {
		Bracket          int64           `json:"bracket,omitempty"`
		InitialLeverage  int64           `json:"initialLeverage,omitempty"`
		QtyCap           decimal.Decimal `json:"qtyCap,omitempty"`        // 该层对应的数量上限
		QtyFloor         decimal.Decimal `json:"qtyFloor,omitempty"`      // 该层对应的数量下限
		NotionalCap      decimal.Decimal `json:"notionalCap,omitempty"`   // 该层对应的数量上限
		NotionalFloor    decimal.Decimal `json:"notionalFloor,omitempty"` // 该层对应的数量下限
		MaintMarginRatio decimal.Decimal `json:"maintMarginRatio,omitempty"`
		Cum              decimal.Decimal `json:"cum,omitempty"`
	}
	ret := []struct {
		Symbol   string    `json:"symbol,omitempty"`
		Brackets []Bracket `json:"brackets,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret) == 0 {
		return nil, errors.New(bn.Name() + " resp empty")
	}
	lbs := make([]*FuturesLeverageBracket, 0, len(ret[0].Brackets))
	for _, v := range ret[0].Brackets {
		lbs = append(lbs, &FuturesLeverageBracket{
			Bracket:          v.Bracket,
			InitialLeverage:  v.InitialLeverage,
			QtyCap:           v.QtyCap,
			QtyFloor:         v.QtyFloor,
			NotionalCap:      v.NotionalCap,
			NotionalFloor:    v.NotionalFloor,
			MaintMarginRatio: v.MaintMarginRatio,
			Cum:              v.Cum,
		})
	}
	return lbs, nil
}
func (bn *Binance) FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error) {
	url := bnUMFuturesEndpoint + "/fapi/v2/positionRisk?" + bn.httpQuerySign("")
	if typ == "CM" {
		url = bnCMFuturesEndpoint + "/dapi/v1/positionRisk?" + bn.httpQuerySign("")
	}
	if bn.isUnified {
		url = bnUnifiedEndpoint + "/papi/v1/um/positionRisk?" + bn.httpQuerySign("")
		if typ == "CM" {
			url = bnUnifiedEndpoint + "/papi/v1/cm/positionRisk?" + bn.httpQuerySign("")
		}
	}
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	if resp[0] == '{' {
		return nil, bn.handleExceptionResp("FuturesGetAllPositionList", resp)
	}
	recv := []struct {
		Symbol     string          `json:"symbol"`
		EntryPrice decimal.Decimal `json:"entryPrice,omitempty"`
		Leverage   decimal.Decimal `json:"leverage,omitempty"`
		LiqPrice   decimal.Decimal `json:"liquidationPrice,omitempty"`
		//MarkPrice decimal.Decimal `json:"markPrice,omitempty"`
		// 符号代表多空方向, 正数为多，负数为空
		PositionQty   decimal.Decimal `json:"positionAmt,omitempty"`
		NotionalVal   decimal.Decimal `json:"notionalValue,omitempty"` // for CM
		Notional      decimal.Decimal `json:"notional,omitempty"`      // for UM
		UnrealisedPnl decimal.Decimal `json:"unRealizedProfit,omitempty"`
		Side          string          `json:"positionSide,omitempty"` // BOTH/SELL/BUY

		Time int64 `json:"updateTime,omitempty"` // msec
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	positionM := make(map[string]*FuturesPosition)
	for _, v := range recv {
		side := bn.toStdSide(v.Side)
		if v.Side == "BOTH" { // 单仓模式
			side = "SELL"
			if v.PositionQty.IsPositive() {
				side = "BUY"
			}
			v.PositionQty = v.PositionQty.Abs()
		} else {
			continue // 只支持单仓模式
		}
		cp := FuturesPosition{
			Symbol:           strings.ReplaceAll(v.Symbol, "_PERP", ""), // BTCUSDT
			Side:             side,
			PositionQty:      v.PositionQty,
			EntryPrice:       v.EntryPrice,
			UnRealizedProfit: v.UnrealisedPnl,
			LiqPrice:         v.LiqPrice,
			Leverage:         v.Leverage,
			UTime:            v.Time,
		}
		positionM[cp.Symbol] = &cp
	}

	return positionM, nil
}
