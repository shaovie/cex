package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (kc *Kucoin) SpotSupported() bool {
	return true
}
func (kc *Kucoin) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	path := "/api/ua/v1/market/instrument"
	params := "tradeType=SPOT"
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, nil)
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
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
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
	path := "/api/ua/v1/market/ticker"
	params := "tradeType=SPOT&symbol=" + symbolS
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, nil)
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
	if err = json.Unmarshal(resp, &recv); err != nil {
		return BestBidAsk{}, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "200000" {
		return BestBidAsk{}, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
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
func (kc *Kucoin) SpotGetOrderBook(symbol string, depth int64) (*OrderBookDepth, error) {
	symbolS := kc.getSpotSymbol(symbol)
	if depth <= 0 {
		depth = 20
	}
	path := "/api/ua/v1/market/orderbook"
	params := "tradeType=SPOT&symbol=" + symbolS +
		"&limit=" + strconv.FormatInt(depth, 10)
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Time int64                `json:"time,omitempty"`
			Asks [][2]decimal.Decimal `json:"asks,omitempty"`
			Bids [][2]decimal.Decimal `json:"bids,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.Bids) == 0 || len(recv.Data.Asks) == 0 {
		return nil, errors.New(kc.Name() + " resp empty!")
	}
	obd := &OrderBookDepth{
		Symbol: symbol,
		Level:  int(depth),
		Time:   recv.Data.Time,
		Bids:   make([]Ticker, 0, len(recv.Data.Bids)),
		Asks:   make([]Ticker, 0, len(recv.Data.Asks)),
	}
	for _, v := range recv.Data.Bids {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range recv.Data.Asks {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	return obd, nil
}
func (kc *Kucoin) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/ua/v1/unified/account/balance"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "", ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Accounts []struct {
				Currencies []KucoinCurrency `json:"currencies"`
			} `json:"accounts"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	assetsMap := make(map[string]*SpotAsset)
	for _, acc := range recv.Data.Accounts {
		for _, v := range acc.Currencies {
			if v.Available.IsZero() && v.Balance.IsZero() && v.Hold.IsZero() {
				continue
			}
			assetsMap[v.Currency] = &SpotAsset{
				Symbol: v.Currency,
				Avail:  v.Available,
				Locked: v.Hold,
				Total:  v.Balance,
			}
		}
	}
	return assetsMap, nil
}
func (kc *Kucoin) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	var f SpotTradeFee
	path := "/api/ua/v1/user/fee-rate"
	params := "tradeType=SPOT&symbol=" + kc.getSpotSymbol(symbol)
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return f, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			List []struct {
				MakerFeeRate decimal.Decimal `json:"makerFeeRate"`
				TakerFeeRate decimal.Decimal `json:"takerFeeRate"`
			} `json:"list"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return f, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "200000" {
		return f, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.List) == 0 {
		return f, errors.New(kc.Name() + " resp empty!")
	}
	f.Maker = recv.Data.List[0].MakerFeeRate
	f.Taker = recv.Data.List[0].TakerFeeRate
	return f, nil
}
func (kc *Kucoin) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {
	if orderType != "LIMIT" && orderType != "MARKET" {
		return "", errors.New(kc.Name() + " not support order type! " + orderType)
	}
	req := struct {
		TradeType   string `json:"tradeType"`
		Symbol      string `json:"symbol"`
		ClientOid   string `json:"clientOid,omitempty"`
		Side        string `json:"side"`
		OrderType   string `json:"orderType"`
		Size        string `json:"size"`
		SizeUnit    string `json:"sizeUnit"`
		Price       string `json:"price,omitempty"`
		PostOnly    bool   `json:"postOnly,omitempty"`
		TimeInForce string `json:"timeInForce,omitempty"`
	}{
		TradeType: "SPOT",
		Symbol:    kc.getSpotSymbol(symbol),
		Side:      kc.fromStdSide(side),
		OrderType: orderType,
		SizeUnit:  "BASECCY",
		PostOnly:  postOnly,
	}
	if req.Symbol == "" {
		return "", errors.New(kc.Name() + " unknown symbol! " + symbol)
	}
	if cltId != "" {
		req.ClientOid = cltId
	}
	if orderType == "LIMIT" {
		req.Price = price.String()
		req.Size = qty.String()
	} else if amt.IsPositive() { // 市价买 size为计价币金额
		req.Size = amt.String()
		req.SizeUnit = "QUOTECCY"
	} else {
		req.Size = qty.String()
	}
	if timeInForce != "" {
		req.TimeInForce = kc.fromStdTimeInForce(timeInForce)
	}
	payload, _ := json.Marshal(req)
	path := "/api/ua/v1/unified/order/place"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Post(link, payload, kcApiDeadline,
		kc.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return "", errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderId string `json:"orderId,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return "", errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data.OrderId) == 0 {
		return "", errors.New(kc.Name() + " orderId is empty!")
	}
	return ret.Data.OrderId, nil
}
func (kc *Kucoin) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	req := struct {
		TradeType string `json:"tradeType"`
		Symbol    string `json:"symbol"`
		OrderId   string `json:"orderId,omitempty"`
		ClientOid string `json:"clientOid,omitempty"`
	}{
		TradeType: "SPOT",
		Symbol:    kc.getSpotSymbol(symbol),
	}
	if orderId != "" {
		req.OrderId = orderId
	} else if cltId != "" {
		req.ClientOid = cltId
	} else {
		return errors.New(kc.Name() + " orderId or cltId empty!")
	}
	payload, _ := json.Marshal(req)
	path := "/api/ua/v1/unified/order/cancel"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Post(link, payload, kcApiDeadline,
		kc.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
	}
	return nil
}

func (kc *Kucoin) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	path := "/api/ua/v1/unified/order/detail"
	params := "tradeType=SPOT&symbol=" + kc.getSpotSymbol(symbol)
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&clientOid=" + cltId
	} else {
		return nil, errors.New(kc.Name() + " orderId or cltId empty!")
	}
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data KucoinSpotOrder `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.OrderId) == 0 {
		return nil, errors.New(kc.Name() + " resp empty!")
	}
	return kc.toStdOrder(&recv.Data), nil
}
func (kc *Kucoin) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	return kc.spotGetOrderList("/api/ua/v1/unified/order/open-list", symbol)
}
func (kc *Kucoin) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	return kc.spotGetOrderList("/api/ua/v1/unified/order/history", symbol)
}
func (kc *Kucoin) spotGetOrderList(path, symbol string) ([]*SpotOrder, error) {
	params := "tradeType=SPOT&pageSize=100"
	if symbol != "" {
		params += "&symbol=" + kc.getSpotSymbol(symbol)
	}
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []KucoinSpotOrder `json:"items"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	dl := make([]*SpotOrder, 0, len(recv.Data.Items))
	for i := range recv.Data.Items {
		dl = append(dl, kc.toStdOrder(&recv.Data.Items[i]))
	}
	return dl, nil
}

// toStdOrder 交易所订单转标准订单
func (kc *Kucoin) toStdOrder(v *KucoinSpotOrder) *SpotOrder {
	ctime, _ := strconv.ParseInt(v.CreatedAt, 10, 64)
	utime, _ := strconv.ParseInt(v.UpdatedAt, 10, 64)
	return &SpotOrder{
		Symbol:      strings.ReplaceAll(v.Symbol, "-", ""),
		OrderId:     v.OrderId,
		ClientId:    v.ClientOid,
		Price:       v.Price,
		Qty:         v.Size,
		FilledQty:   v.FilledSize,
		FilledAmt:   v.FilledSize.Mul(v.AvgPrice),
		AvgPrice:    v.AvgPrice,
		Status:      kc.toStdOrderStatus(v.Status),
		Type:        kc.toStdOrderType(v.OrderType),
		TimeInForce: kc.toStdTimeInForce(v.TimeInForce),
		Side:        kc.toStdSide(v.Side),
		FeeQty:      v.Fee.Neg(),
		FeeAsset:    v.FeeCurrency,
		CTime:       ctime,
		UTime:       utime,
	}
}
