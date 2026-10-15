package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

func (mc *Mexc) SpotSupported() bool {
	return true
}
func (mc *Mexc) SpotServerTime() (int64, error) {
	link := mcUniEndpoint + "/api/v3/time"
	_, resp, err := mc.Get(link, mcApiDeadline, nil)
	if err != nil {
		return 0, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"serverTime,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (mc *Mexc) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := mcUniEndpoint + "/api/v3/exchangeInfo"
	_, resp, err := mc.Get(link, mcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code    int    `json:"code,omitempty"`
		Msg     string `json:"msg,omitempty"`
		Symbols []struct {
			Symbol            string          `json:"symbol,omitempty"`
			Base              string          `json:"baseAsset,omitempty"`
			Quote             string          `json:"quoteAsset,omitempty"`
			Status            string          `json:"status"` // 1 开放, 2 暂停, 3 下线
			QuotePrecision    int32           `json:"quotePrecision,omitempty"`
			BaseSizePrecision decimal.Decimal `json:"baseSizePrecision"`
			MinNotional       decimal.Decimal `json:"quoteAmountPrecision"`
		} `json:"symbols,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Symbols) == 0 {
		return nil, mc.handleExceptionResp("SpotLoadAllPairRule", resp)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Symbols {
		if pair.Status != "1" { // 开放交易
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
			Status:        "online",
			PriceTickSize: PowOneTenth(int(pair.QuotePrecision)),
			QtyStep:       pair.BaseSizePrecision,
			MinOrderQty:   pair.BaseSizePrecision,
			MaxOrderQty:   decimal.NewFromFloat(999999999),
			MaxPrice:      decimal.NewFromFloat(999999999),
			MinNotional:   pair.MinNotional,
			Time:          now,
		}
		ep.MinPrice = ep.PriceTickSize
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (mc *Mexc) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	link := mcUniEndpoint + "/api/v3/ticker/24hr"
	_, resp, err := mc.Get(link, mcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
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
func (mc *Mexc) SpotGetBBO(symbol string) (BestBidAsk, error) {
	link := mcUniEndpoint + "/api/v3/ticker/bookTicker?symbol=" + symbol
	_, resp, err := mc.Get(link, mcApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(mc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code     int             `json:"code,omitempty"`
		Msg      string          `json:"msg,omitempty"`
		Symbol   string          `json:"symbol,omitempty"`
		BidPrice decimal.Decimal `json:"bidPrice"`
		BidQty   decimal.Decimal `json:"bidQty"`
		AskPrice decimal.Decimal `json:"askPrice"`
		AskQty   decimal.Decimal `json:"askQty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return BestBidAsk{}, errors.New(mc.Name() + " Unmarshal err! " + err.Error())
	}
	if len(ret.Symbol) == 0 {
		return BestBidAsk{}, mc.handleExceptionResp("SpotGetBBO", resp)
	}
	return BestBidAsk{
		Symbol:   ret.Symbol,
		BidPrice: ret.BidPrice,
		BidQty:   ret.BidQty,
		AskPrice: ret.AskPrice,
		AskQty:   ret.AskQty,
	}, nil
}
func (mc *Mexc) SpotGetOrderBook(symbol string, depth int64) (*OrderBookDepth, error) {
	if depth <= 0 || depth > 5000 {
		depth = 100 // 该接口默认100档, 最大5000档
	}
	link := mcUniEndpoint + "/api/v3/depth?symbol=" + symbol +
		"&limit=" + strconv.FormatInt(depth, 10)
	_, resp, err := mc.Get(link, mcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code     int                  `json:"code,omitempty"`
		Msg      string               `json:"msg,omitempty"`
		UpdateId int64                `json:"lastUpdateId,omitempty"`
		Time     int64                `json:"timestamp,omitempty"`
		Bids     [][2]decimal.Decimal `json:"bids,omitempty"`
		Asks     [][2]decimal.Decimal `json:"asks,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Bids) == 0 || len(recv.Asks) == 0 {
		return nil, mc.handleExceptionResp("SpotGetOrderBook", resp)
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
func (mc *Mexc) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/v3/account"
	link := mcUniEndpoint + path + "?" + mc.buildQuery("")
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code     int    `json:"code,omitempty"`
		Msg      string `json:"msg,omitempty"`
		Balances []struct {
			Symbol string          `json:"asset"`
			Free   decimal.Decimal `json:"free"`
			Locked decimal.Decimal `json:"locked"`
		} `json:"balances,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Balances) == 0 {
		return nil, mc.handleExceptionResp("SpotGetAllAssets", resp)
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
func (mc *Mexc) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	var f SpotTradeFee
	path := "/api/v3/tradeFee"
	link := mcUniEndpoint + path + "?" + mc.buildQuery("symbol="+symbol)
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return f, errors.New(mc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			MakerCommission decimal.Decimal `json:"makerCommission"`
			TakerCommission decimal.Decimal `json:"takerCommission"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return f, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return f, mc.handleExceptionResp("SpotGetTradeFee", resp)
	}
	f.Maker = recv.Data.MakerCommission
	f.Taker = recv.Data.TakerCommission
	return f, nil
}
func (mc *Mexc) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {
	if orderType != "LIMIT" && orderType != "MARKET" {
		return "", errors.New(mc.Name() + " not support order type! " + orderType)
	}
	orderTypeS := orderType
	if orderType == "LIMIT" {
		if postOnly {
			orderTypeS = "LIMIT_MAKER"
		} else if timeInForce == "IOC" {
			orderTypeS = "IMMEDIATE_OR_CANCEL"
		} else if timeInForce == "FOK" {
			orderTypeS = "FILL_OR_KILL"
		}
	}
	params := "symbol=" + symbol + "&side=" + mc.fromStdSide(side) + "&type=" + orderTypeS
	if cltId != "" {
		params += "&newClientOrderId=" + cltId
	}
	if orderType == "LIMIT" {
		params += "&price=" + price.String() + "&quantity=" + qty.String()
	} else if amt.IsPositive() { // 市价买 amt为计价币金额
		params += "&quoteOrderQty=" + amt.String()
	} else {
		params += "&quantity=" + qty.String()
	}
	path := "/api/v3/order"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Post(link, nil, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return "", errors.New(mc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return "", mc.handleExceptionResp("SpotPlaceOrder", resp)
	}
	return ret.OrderId.String(), nil
}
func (mc *Mexc) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return errors.New(mc.Name() + " orderId or cltId empty!")
	}
	path := "/api/v3/order"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Delete(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return errors.New(mc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
		Status  string      `json:"status,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return mc.handleExceptionResp("SpotCancelOrder", resp)
	}
	return nil
}

func (mc *Mexc) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return nil, errors.New(mc.Name() + " orderId or cltId empty!")
	}
	path := "/api/v3/order"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	ret := MexcSpotOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return nil, mc.handleExceptionResp("SpotGetOrder", resp)
	}
	return mc.toStdOrder(&ret), nil
}
func (mc *Mexc) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	params := ""
	if symbol != "" {
		params = "symbol=" + symbol
	}
	return mc.spotGetOrderList("/api/v3/openOrders", params)
}
func (mc *Mexc) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	return mc.spotGetOrderList("/api/v3/allOrders", "symbol="+symbol+"&limit=100")
}
func (mc *Mexc) spotGetOrderList(path, params string) ([]*SpotOrder, error) {
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, mc.handleExceptionResp("SpotGetOpenOrders", resp)
	}
	ret := []MexcSpotOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*SpotOrder, 0, len(ret))
	for i := range ret {
		dl = append(dl, mc.toStdOrder(&ret[i]))
	}
	return dl, nil
}

// toStdOrder 交易所订单转标准订单
func (mc *Mexc) toStdOrder(v *MexcSpotOrder) *SpotOrder {
	return &SpotOrder{
		Symbol:      v.Symbol,
		OrderId:     v.OrderId.String(),
		ClientId:    v.ClientId,
		Price:       v.Price,
		Qty:         v.OrigQty,
		FilledQty:   v.ExecutedQty,
		FilledAmt:   v.filledAmt(),
		Status:      mc.toStdOrderStatus(v.Status),
		Type:        mc.toStdOrderType(v.Type),
		TimeInForce: mc.toStdTimeInForce(v.TimeInForce),
		Side:        mc.toStdSide(v.Side),
		CTime:       v.Time,
		UTime:       v.UpdateTime,
	}
}
