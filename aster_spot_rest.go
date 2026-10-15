package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// AsterSpotOrder 现货订单
type AsterSpotOrder struct {
	Code         int             `json:"code,omitempty"`
	Msg          string          `json:"msg,omitempty"`
	Symbol       string          `json:"symbol,omitempty"`
	OrderId      json.Number     `json:"orderId,omitempty"`
	ClientId     string          `json:"clientOrderId,omitempty"`
	Price        decimal.Decimal `json:"price"`
	AvgPrice     decimal.Decimal `json:"avgPrice"`
	OrigQty      decimal.Decimal `json:"origQty"`
	ExecutedQty  decimal.Decimal `json:"executedQty"`
	CummQuoteQty decimal.Decimal `json:"cumQuote"`
	Status       string          `json:"status,omitempty"`
	Type         string          `json:"type,omitempty"`
	TimeInForce  string          `json:"timeInForce,omitempty"`
	Side         string          `json:"side,omitempty"`
	Time         int64           `json:"time,omitempty"`
	UpdateTime   int64           `json:"updateTime,omitempty"`
}

func (as *Aster) SpotSupported() bool {
	return true
}
func (as *Aster) SpotServerTime() (int64, error) {
	link := asSpotEndpoint + "/api/v3/time"
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return 0, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"serverTime,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(as.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (as *Aster) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := asSpotEndpoint + "/api/v3/exchangeInfo"
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code    int    `json:"code,omitempty"`
		Msg     string `json:"msg,omitempty"`
		Symbols []struct {
			Symbol  string `json:"symbol,omitempty"`
			Base    string `json:"baseAsset,omitempty"`
			Quote   string `json:"quoteAsset,omitempty"`
			Status  string `json:"status,omitempty"` // TRADING
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
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Symbols) == 0 {
		return nil, as.handleExceptionResp("SpotLoadAllPairRule", resp)
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
			Status: "online",
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
func (as *Aster) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	link := asSpotEndpoint + "/api/v3/ticker/24hr"
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("SpotGetAll24hTicker", resp)
	}
	tickers := []struct {
		Symbol      string          `json:"symbol"`
		Last        decimal.Decimal `json:"lastPrice"`
		Volume      decimal.Decimal `json:"volume"`
		QuoteVolume decimal.Decimal `json:"quoteVolume"`
	}{}
	if err = json.Unmarshal(resp, &tickers); err != nil {
		return nil, errors.New(as.Name() + " Unmarshal err! " + err.Error())
	}
	if len(tickers) == 0 {
		return nil, errors.New(as.Name() + " resp empty")
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
func (as *Aster) SpotGetBBO(symbol string) (BestBidAsk, error) {
	link := asSpotEndpoint + "/api/v3/ticker/bookTicker?symbol=" + symbol
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code     int             `json:"code,omitempty"`
		Msg      string          `json:"msg,omitempty"`
		Symbol   string          `json:"symbol,omitempty"`
		BidPrice decimal.Decimal `json:"bidPrice"`
		BidQty   decimal.Decimal `json:"bidQty"`
		AskPrice decimal.Decimal `json:"askPrice"`
		AskQty   decimal.Decimal `json:"askQty"`
		Time     int64           `json:"time,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return BestBidAsk{}, errors.New(as.Name() + " Unmarshal err! " + err.Error())
	}
	if len(ret.Symbol) == 0 {
		return BestBidAsk{}, as.handleExceptionResp("SpotGetBBO", resp)
	}
	return BestBidAsk{
		Symbol:   ret.Symbol,
		Time:     ret.Time,
		BidPrice: ret.BidPrice,
		BidQty:   ret.BidQty,
		AskPrice: ret.AskPrice,
		AskQty:   ret.AskQty,
	}, nil
}
func (as *Aster) SpotGetOrderBook(symbol string, depth int64) (*OrderBookDepth, error) {
	if depth <= 0 || depth > 1000 {
		depth = 100 // 该接口默认100档, 可选[5, 10, 20, 50, 100, 500, 1000]
	}
	link := asSpotEndpoint + "/api/v3/depth?symbol=" + symbol +
		"&limit=" + strconv.FormatInt(depth, 10)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code     int                  `json:"code,omitempty"`
		Msg      string               `json:"msg,omitempty"`
		UpdateId int64                `json:"lastUpdateId,omitempty"`
		Time     int64                `json:"T,omitempty"`
		Bids     [][2]decimal.Decimal `json:"bids,omitempty"`
		Asks     [][2]decimal.Decimal `json:"asks,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Bids) == 0 || len(recv.Asks) == 0 {
		return nil, as.handleExceptionResp("SpotGetOrderBook", resp)
	}
	obd := &OrderBookDepth{
		Symbol: symbol,
		Level:  int(depth),
		Time:   recv.Time,
		Bids:   make([]Ticker, 0, len(recv.Bids)),
		Asks:   make([]Ticker, 0, len(recv.Asks)),
	}
	for _, v := range recv.Bids {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range recv.Asks {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	return obd, nil
}
func (as *Aster) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/v3/account"
	link := asSpotEndpoint + path + "?" + as.buildQuery("")
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code     int    `json:"code,omitempty"`
		Msg      string `json:"msg,omitempty"`
		Balances []struct {
			Symbol string          `json:"asset,omitempty"`
			Free   decimal.Decimal `json:"free"`
			Locked decimal.Decimal `json:"locked"`
		} `json:"balances,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Balances) == 0 {
		return nil, as.handleExceptionResp("SpotGetAllAssets", resp)
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
func (as *Aster) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	var f SpotTradeFee
	path := "/api/v3/commissionRate"
	link := asSpotEndpoint + path + "?" + as.buildQuery("symbol="+symbol)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return f, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int             `json:"code,omitempty"`
		Msg    string          `json:"msg,omitempty"`
		Symbol string          `json:"symbol,omitempty"`
		Maker  decimal.Decimal `json:"makerCommissionRate"`
		Taker  decimal.Decimal `json:"takerCommissionRate"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return f, errors.New(as.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Symbol) == 0 {
		return f, as.handleExceptionResp("SpotGetTradeFee", resp)
	}
	f.Maker = recv.Maker
	f.Taker = recv.Taker
	return f, nil
}
func (as *Aster) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {
	if orderType != "LIMIT" && orderType != "MARKET" {
		return "", errors.New(as.Name() + " not support order type! " + orderType)
	}
	params := "symbol=" + symbol + "&side=" + as.toStdSide(side) + "&type=" + orderType
	if cltId != "" {
		params += "&newClientOrderId=" + cltId
	}
	if orderType == "LIMIT" {
		if postOnly { // 只做Maker
			params += "&timeInForce=GTX"
		} else if timeInForce != "" {
			params += "&timeInForce=" + as.toStdTimeInForce(timeInForce)
		}
		params += "&price=" + price.String() + "&quantity=" + qty.String()
	} else if amt.IsPositive() { // 市价买 amt为计价币金额
		params += "&quoteOrderQty=" + amt.String()
	} else {
		params += "&quantity=" + qty.String()
	}
	path := "/api/v3/order"
	link := asSpotEndpoint + path
	_, resp, err := as.Post(link, as.buildBody(params), asApiDeadline, as.buildHeaders())
	if err != nil {
		return "", errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return "", as.handleExceptionResp("SpotPlaceOrder", resp)
	}
	return ret.OrderId.String(), nil
}
func (as *Aster) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return errors.New(as.Name() + " orderId or cltId empty!")
	}
	path := "/api/v3/order"
	link := asSpotEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Delete(link, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
		Status  string      `json:"status,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return as.handleExceptionResp("SpotCancelOrder", resp)
	}
	return nil
}

func (as *Aster) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return nil, errors.New(as.Name() + " orderId or cltId empty!")
	}
	path := "/api/v3/order"
	link := asSpotEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := AsterSpotOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return nil, as.handleExceptionResp("SpotGetOrder", resp)
	}
	return as.toStdOrder(&ret), nil
}
func (as *Aster) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	params := ""
	if symbol != "" {
		params = "symbol=" + symbol
	}
	return as.spotGetOrderList("/api/v3/openOrders", params)
}
func (as *Aster) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	return as.spotGetOrderList("/api/v3/allOrders", "symbol="+symbol+"&limit=100")
}
func (as *Aster) spotGetOrderList(path, params string) ([]*SpotOrder, error) {
	link := asSpotEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("SpotGetOpenOrders", resp)
	}
	ret := []AsterSpotOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*SpotOrder, 0, len(ret))
	for i := range ret {
		dl = append(dl, as.toStdOrder(&ret[i]))
	}
	return dl, nil
}

// toStdOrder 交易所订单转标准订单
func (as *Aster) toStdOrder(v *AsterSpotOrder) *SpotOrder {
	return &SpotOrder{
		Symbol:      v.Symbol,
		OrderId:     v.OrderId.String(),
		ClientId:    v.ClientId,
		Price:       v.Price,
		Qty:         v.OrigQty,
		FilledQty:   v.ExecutedQty,
		FilledAmt:   v.CummQuoteQty,
		AvgPrice:    v.AvgPrice,
		Status:      as.toStdOrderStatus(v.Status),
		Type:        as.toStdOrderType(v.Type),
		TimeInForce: as.toStdTimeInForce(v.TimeInForce),
		Side:        as.toStdSide(v.Side),
		CTime:       v.Time,
		UTime:       v.UpdateTime,
	}
}
