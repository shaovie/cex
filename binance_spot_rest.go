package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bn *Binance) SpotSupported() bool {
	return true
}
func (bn *Binance) SpotServerTime() (int64, error) {
	url := bnSpotEndpoint + "/api/v3/time"
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
func (bn *Binance) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := bnSpotEndpoint + "/api/v3/exchangeInfo?permissions=SPOT&symbolStatus=TRADING"
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code    int    `json:"code,omitempty"`
		Msg     string `json:"msg,omitempty"`
		Symbols []struct {
			Symbol  string `json:"symbol,omitempty"`
			Base    string `json:"baseAsset,omitempty"`
			Quote   string `json:"quoteAsset,omitempty"`
			Status  string `json:"status"`
			Filters []struct {
				FilterType string          `json:"filterType"`
				MaxPrice   decimal.Decimal `json:"maxPrice,omitempty"`
				MinPrice   decimal.Decimal `json:"minPrice,omitempty"`
				TickSize   decimal.Decimal `json:"tickSize,omitempty"`

				MaxQty   decimal.Decimal `json:"maxQty,omitempty"`
				MinQty   decimal.Decimal `json:"minQty,omitempty"`
				StepSize decimal.Decimal `json:"stepSize,omitempty"`

				MinNotional decimal.Decimal `json:"minNotional,omitempty"`
			} `json:"filters,omitempty"`
		} `json:"symbols,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bn.Name() + " api err! " + recv.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Symbols {
		if pair.Status != "TRADING" {
			continue
		}

		ep := &SpotExchangePairRule{
			Symbol: pair.Symbol,
			Base:   pair.Base,
			Quote:  pair.Quote,
			Time:   now,
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
			} else if flt.FilterType == "NOTIONAL" {
				ep.MinNotional = flt.MinNotional
			}
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (bn *Binance) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	url := bnSpotEndpoint + "/api/v3/ticker/24hr"
	_, resp, err := ihttp.Get(url, bnApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("SpotGetAll24hTicker", resp)
	}
	tickers := []struct {
		Symbol      string          `json:"symbol"`
		Last        decimal.Decimal `json:"lastPrice"`
		Volume      decimal.Decimal `json:"volume"`
		QuoteVolume decimal.Decimal `json:"quoteVolume"`
	}{}
	if err = json.Unmarshal(resp, &tickers); err != nil {
		return nil, errors.New(bn.Name() + " Unmarshal err! " + err.Error())
	}
	if len(tickers) == 0 {
		return nil, errors.New(bn.Name() + " resp empty")
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
func (bn *Binance) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	url := bnSpotEndpoint + "/api/v3/account?" + bn.httpQuerySign("")
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code     int    `json:"code,omitempty"`
		Msg      string `json:"msg,omitempty"`
		Balances []struct {
			Symbol string          `json:"asset,omitempty"`
			Free   decimal.Decimal `json:"free,omitempty"`
			Locked decimal.Decimal `json:"locked,omitempty"`
		} `json:"balances,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 || len(recv.Msg) != 0 {
		return nil, errors.New(bn.Name() + " api err! " + recv.Msg)
	}

	assetsMap := make(map[string]*SpotAsset, len(recv.Balances))
	for _, v := range recv.Balances {
		if v.Free.IsZero() && v.Locked.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &SpotAsset{
			Symbol: v.Symbol,
			Avail:  v.Free,
			Locked: v.Locked,
			Total:  v.Free.Add(v.Locked),
		}
	}
	return assetsMap, nil
}
func (bn *Binance) SpotPlaceOrder(symbol, cltId string /*BTCUSDT*/, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {
	params := fmt.Sprintf("&newOrderRespType=ACK&symbol=%s&side=%s&type=%s",
		symbol, side, orderType)
	if cltId != "" {
		params += "&newClientOrderId=" + cltId
	}
	if orderType == "LIMIT" {
		params += "&timeInForce=" + timeInForce + "&price=" + price.String()
		params += "&quantity=" + qty.String()
	} else if orderType == "MARKET" {
		if side == "BUY" {
			params += "&quoteOrderQty=" + qty.String()
		} else if side == "SELL" {
			params += "&quantity=" + qty.String()
		}
	} else {
		return "", errors.New("not support order type:" + orderType)
	}
	url := bnSpotEndpoint + "/api/v3/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Post(url, nil, bnApiDeadline, headers)
	if err != nil {
		return "", errors.New(bn.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Symbol   string `json:"symbol,omitempty"` // BTCUSDT
		OrderId  int64  `json:"orderId,omitempty"`
		ClientId string `json:"clientOrderId,omitempty"`
		Time     int64  `json:"transactTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return "", errors.New(bn.Name() + " api err! " + ret.Msg)
	}

	return strconv.FormatInt(ret.OrderId, 10), nil
}
func (bn *Binance) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	params := fmt.Sprintf("&symbol=%s", symbol)
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return errors.New(bn.Name() + " orderId or cltId empty!")
	}
	url := bnSpotEndpoint + "/api/v3/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Delete(url, bnApiDeadline, headers)
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Status string `json:"status,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(ret.Msg)
	}
	if ret.Status != "CANCELED" {
		return errors.New(bn.Name() + " cancel failed! status now: " + ret.Status)
	}
	return nil
}
func (bn *Binance) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	params := fmt.Sprintf("&symbol=%s", symbol)
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return nil, errors.New(bn.Name() + " orderId or cltId empty!")
	}
	url := bnSpotEndpoint + "/api/v3/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Get(url, bnApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	order := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId      int64           `json:"orderId,omitempty"`
		ClientId     string          `json:"clientOrderId,omitempty"` // BTCUSDT
		Price        decimal.Decimal `json:"price,omitempty"`
		Quantity     decimal.Decimal `json:"origQty,omitempty"`             // 用户设置的原始订单数量
		ExecutedQty  decimal.Decimal `json:"executedQty,omitempty"`         // 交易的订单数量
		CummQuoteQty decimal.Decimal `json:"cummulativeQuoteQty,omitempty"` // 累计交易的金额
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
	return &SpotOrder{
		Symbol:      order.Symbol,
		OrderId:     strconv.FormatInt(order.OrderId, 10),
		ClientId:    order.ClientId,
		Price:       order.Price,
		Qty:         order.Quantity,
		FilledQty:   order.ExecutedQty,
		FilledAmt:   order.CummQuoteQty,
		Status:      order.Status,
		Type:        order.Type,
		TimeInForce: order.TimeInForce,
		Side:        order.Side,
		CTime:       order.Time,
		UTime:       order.UTime,
	}, nil
}
func (bn *Binance) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	params := ""
	if symbol != "" {
		params += "&symbol=" + symbol
	}
	url := bnSpotEndpoint + "/api/v3/openOrders?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("SpotGetOpenOrders", resp)
	}
	orders := []struct {
		Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId      int64           `json:"orderId,omitempty"`
		ClientId     string          `json:"clientOrderId,omitempty"`
		Price        decimal.Decimal `json:"price,omitempty"`
		Quantity     decimal.Decimal `json:"origQty,omitempty"`             // 用户设置的原始订单数量
		ExecutedQty  decimal.Decimal `json:"executedQty,omitempty"`         // 交易的订单数量
		CummQuoteQty decimal.Decimal `json:"cummulativeQuoteQty,omitempty"` // 累计交易的金额
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`        // LIMIT/MARKET
		TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		Time         int64           `json:"time,omitempty"`
		UTime        int64           `json:"updateTime,omitempty"`
	}{}
	err = json.Unmarshal(resp, &orders)
	if err != nil {
		return nil, errors.New(bn.Name() + " Unmarshal err! " + err.Error())
	}
	dl := make([]*SpotOrder, 0, len(orders))
	for _, order := range orders {
		so := SpotOrder{
			Symbol:      order.Symbol,
			OrderId:     strconv.FormatInt(order.OrderId, 10),
			ClientId:    order.ClientId,
			Price:       order.Price,
			Qty:         order.Quantity,
			FilledQty:   order.ExecutedQty,
			FilledAmt:   order.CummQuoteQty,
			Status:      order.Status,
			Type:        order.Type,
			TimeInForce: order.TimeInForce,
			Side:        order.Side,
			CTime:       order.Time,
			UTime:       order.UTime,
		}
		dl = append(dl, &so)
	}

	return dl, nil
}
