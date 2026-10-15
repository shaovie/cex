package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

func (bb *Bybit) SpotSupported() bool {
	return true
}
func (bb *Bybit) SpotServerTime() (int64, error) {
	return bb.serverTime()
}
func (bb *Bybit) serverTime() (int64, error) {
	url := bbUniEndpoint + "/v5/market/time"
	_, resp, err := bb.Get(url, bbApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Result struct {
			Time string `json:"timeNano,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	t, _ := strconv.ParseInt(recv.Result.Time, 10, 64)
	return t / 1000000, nil
}
func (bb *Bybit) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := bbUniEndpoint + "/v5/market/instruments-info?category=spot&limit=1000"
	type instrumentsResp struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			Cursor string `json:"nextPageCursor,omitempty"`
			List   []struct {
				Symbol        string `json:"symbol,omitempty"`
				Base          string `json:"baseCoin,omitempty"`
				Quote         string `json:"quoteCoin,omitempty"`
				Status        string `json:"status,omitempty"`
				LotSizeFilter struct {
					MaxQty      decimal.Decimal `json:"maxOrderQty"`
					MinQty      decimal.Decimal `json:"minOrderQty"`
					MinAmt      decimal.Decimal `json:"minOrderAmt"`
					MaxAmt      decimal.Decimal `json:"maxOrderAmt"`
					StepSize    decimal.Decimal `json:"basePrecision"`
					AmtStepSize decimal.Decimal `json:"quotePrecision"`
				} `json:"lotSizeFilter"`
				PriceFilter struct {
					TickSize decimal.Decimal `json:"tickSize"`
				} `json:"priceFilter"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	cursor, lastCursor := "", ""
	for {
		link := url
		if cursor != "" {
			link += "&cursor=" + cursor
		}
		_, resp, err := bb.Get(link, bbApiDeadline, nil)
		if err != nil {
			return nil, errors.New(bb.Name() + " net error! " + err.Error())
		}
		var recv instrumentsResp
		if err = json.Unmarshal(resp, &recv); err != nil {
			return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
		}
		if recv.Code != 0 {
			return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
		}
		for _, pair := range recv.Result.List {
			if pair.Status != "Trading" {
				continue
			}

			ep := &SpotExchangePairRule{
				Symbol:        pair.Symbol,
				Base:          pair.Base,
				Quote:         pair.Quote,
				Status:        "online",
				PriceTickSize: pair.PriceFilter.TickSize,
				MaxOrderQty:   pair.LotSizeFilter.MaxQty,
				MinOrderQty:   pair.LotSizeFilter.MinQty,
				MinPrice:      pair.PriceFilter.TickSize,
				MaxPrice:      decimal.NewFromFloat(9999999999999999.99),
				QtyStep:       pair.LotSizeFilter.StepSize,
				MinNotional:   pair.LotSizeFilter.MinAmt,
				Time:          now,
			}
			all[ep.Symbol] = ep
		}
		cursor = recv.Result.Cursor
		if cursor == "" || cursor == lastCursor { // 无下一页, 或游标未推进
			break
		}
		lastCursor = cursor
	}
	return all, nil
}
func (bb *Bybit) SpotGetBBO(symbol string) (BestBidAsk, error) {
	url := bbUniEndpoint + "/v5/market/tickers?category=spot&symbol=" + symbol
	_, resp, err := bb.Get(url, bbApiDeadline, nil)
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
		} `json:"result"`
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
func (bb *Bybit) SpotGetOrderBook(symbol string, depth int64) (*OrderBookDepth, error) {
	url := bbUniEndpoint + "/v5/market/orderbook?category=spot&symbol=" + symbol
	if depth > 0 {
		url += "&limit=" + strconv.FormatInt(depth, 10)
	}
	_, resp, err := bb.Get(url, bbApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			Symbol string               `json:"s,omitempty"`
			Bids   [][2]decimal.Decimal `json:"b,omitempty"`
			Asks   [][2]decimal.Decimal `json:"a,omitempty"`
			Time   int64                `json:"ts,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bb.Name() + ": " + ret.Msg)
	}
	if len(ret.Result.Bids) == 0 || len(ret.Result.Asks) == 0 {
		return nil, errors.New(bb.Name() + " resp empty")
	}

	obd := &OrderBookDepth{
		Symbol: ret.Result.Symbol,
		Level:  int(depth),
		Time:   ret.Result.Time,
		Bids:   make([]Ticker, 0, len(ret.Result.Bids)),
		Asks:   make([]Ticker, 0, len(ret.Result.Asks)),
	}
	for _, v := range ret.Result.Bids {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range ret.Result.Asks {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	return obd, nil
}
func (bb *Bybit) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	query := "accountType=UNIFIED"
	url := bbUniEndpoint + "/v5/account/wallet-balance?" + query
	_, resp, err := bb.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Time   int64  `json:"time,omitempty"`
		Result struct {
			List []struct {
				Coin []struct {
					Total  decimal.Decimal `json:"equity"`
					Avail  decimal.Decimal `json:"walletBalance"`
					Locked decimal.Decimal `json:"locked"`
					Symbol string          `json:"coin,omitempty"`
				} `json:"coin,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	if len(recv.Result.List) == 0 {
		return nil, errors.New(bb.Name() + " resp empty")
	}

	assetsMap := make(map[string]*SpotAsset)
	for _, v := range recv.Result.List {
		if len(v.Coin) == 0 {
			continue
		}
		for _, as := range v.Coin {
			assetsMap[as.Symbol] = &SpotAsset{
				Symbol: as.Symbol,
				Avail:  as.Avail,
				Total:  as.Avail,
			}
		}
	}
	return assetsMap, nil
}
func (bb *Bybit) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	params := map[string]any{
		"category":    "spot",
		"symbol":      symbol,
		"isLeverage":  0,
		"side":        bb.fromStdSide(side),
		"orderType":   bb.fromStdOrderType(orderType),
		"orderFilter": "Order",
	}
	if cltId != "" {
		params["orderLinkId"] = cltId
	}
	if orderType == "MARKET" {
		if amt.IsPositive() {
			params["qty"] = amt.String()
			params["marketUnit"] = "quoteCoin"
		} else {
			params["qty"] = qty.String()
			params["marketUnit"] = "baseCoin"
		}
	} else {
		params["qty"] = qty.String()
		params["price"] = price.String()
		if timeInForce != "" {
			params["timeInForce"] = timeInForce
		}
		if orderType == "LIMIT" && postOnly {
			params["timeInForce"] = "PostOnly"
		}
	}
	body, _ := json.Marshal(params)
	url := bbUniEndpoint + "/v5/order/create"
	_, resp, err := bb.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			OrderId string `json:"orderId"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return "", errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return "", errors.New(recv.Msg)
	}
	return recv.Result.OrderId, nil
}
func (bb *Bybit) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	params := map[string]any{
		"category":    "spot",
		"symbol":      symbol,
		"orderFilter": "Order",
	}
	if orderId != "" {
		params["orderId"] = orderId
	} else if cltId != "" {
		params["orderLinkId"] = cltId
	} else {
		return errors.New(bb.Name() + " orderId or cltId empty!")
	}
	body, _ := json.Marshal(params)
	url := bbUniEndpoint + "/v5/order/cancel"
	_, resp, err := bb.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			OrderId string `json:"orderId"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return errors.New(recv.Msg)
	}
	return nil
}

// SpotPlaceOrderMultiple 批量下单, 现货一次最多10单
func (bb *Bybit) SpotPlaceOrderMultiple(orders []SpotPostOrder) error {
	if len(orders) == 0 {
		return errors.New(bb.Name() + " orders empty!")
	}
	type BatchOrder struct {
		Symbol      string `json:"symbol"`
		IsLeverage  int    `json:"isLeverage"`
		Side        string `json:"side"`
		OrderType   string `json:"orderType"`
		Qty         string `json:"qty"`
		MarketUnit  string `json:"marketUnit,omitempty"`
		Price       string `json:"price,omitempty"`
		TimeInForce string `json:"timeInForce,omitempty"`
		OrderLinkId string `json:"orderLinkId,omitempty"`
	}
	orderL := make([]BatchOrder, 0, len(orders))
	for i := range orders {
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return errors.New(bb.Name() + " not support order type! " + orders[i].Type)
		}
		po := BatchOrder{
			Symbol:      orders[i].Symbol,
			IsLeverage:  0,
			Side:        bb.fromStdSide(orders[i].Side),
			OrderType:   bb.fromStdOrderType(orders[i].Type),
			OrderLinkId: orders[i].ClientId,
		}
		if orders[i].Type == "MARKET" {
			// 市价买 qty为计价币金额, 市价卖为交易币数量
			if orders[i].Amt.IsPositive() {
				po.Qty = orders[i].Amt.String()
				po.MarketUnit = "quoteCoin"
			} else {
				po.Qty = orders[i].Qty.String()
				po.MarketUnit = "baseCoin"
			}
		} else {
			po.Qty = orders[i].Qty.String()
			po.Price = orders[i].Price.String()
			if orders[i].TimeInForce != "" {
				po.TimeInForce = orders[i].TimeInForce
			}
			if orders[i].PostOnly { // 只做Maker
				po.TimeInForce = "PostOnly"
			}
		}
		orderL = append(orderL, po)
	}
	body, _ := json.Marshal(struct {
		Category string       `json:"category"`
		Request  []BatchOrder `json:"request"`
	}{Category: "spot", Request: orderL})
	url := bbUniEndpoint + "/v5/order/create-batch"
	_, resp, err := bb.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	if err != nil {
		return errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				OrderId string `json:"orderId,omitempty"`
				Reject  string `json:"rejectReason,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return errors.New(bb.Name() + " " + recv.Msg)
	}
	success, failMsg := 0, ""
	for _, v := range recv.Result.List {
		if len(v.OrderId) > 0 {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.Reject
		}
	}
	if success == 0 {
		return errors.New(bb.Name() + " batch order all fail! " + failMsg)
	}
	return nil
}

// SpotCancelOrderMultiple 批量撤单, 现货一次最多10单
func (bb *Bybit) SpotCancelOrderMultiple(symbol string /*BTCUSDT*/, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(bb.Name() + " orderIds or cltIds empty!")
	}
	type CancelOrder struct {
		Symbol      string `json:"symbol"`
		OrderId     string `json:"orderId,omitempty"`
		OrderLinkId string `json:"orderLinkId,omitempty"`
	}
	orderL := make([]CancelOrder, 0, len(orderIds)+len(cltIds))
	for _, v := range orderIds {
		orderL = append(orderL, CancelOrder{Symbol: symbol, OrderId: v})
	}
	for _, v := range cltIds {
		orderL = append(orderL, CancelOrder{Symbol: symbol, OrderLinkId: v})
	}
	body, _ := json.Marshal(struct {
		Category string        `json:"category"`
		Request  []CancelOrder `json:"request"`
	}{Category: "spot", Request: orderL})
	url := bbUniEndpoint + "/v5/order/cancel-batch"
	_, resp, err := bb.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	if err != nil {
		return errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				OrderId string `json:"orderId,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return errors.New(bb.Name() + " " + recv.Msg)
	}
	if len(recv.Result.List) == 0 {
		return errors.New(bb.Name() + " batch cancel resp empty! " + string(resp))
	}
	return nil
}
func (bb *Bybit) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	query := "category=spot&symbol=" + symbol
	if orderId != "" {
		query += "&orderId=" + orderId
	} else if cltId != "" {
		query += "&orderLinkId=" + cltId
	} else {
		return nil, errors.New(bb.Name() + " orderId or cltId empty!")
	}
	url := bbUniEndpoint + "/v5/order/realtime?" + query
	_, resp, err := bb.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol       string            `json:"symbol,omitempty"` // BTCUSDT
				OrderId      string            `json:"orderId,omitempty"`
				ClientId     string            `json:"orderLinkId,omitempty"`
				Price        decimal.Decimal   `json:"price"`
				Quantity     decimal.Decimal   `json:"qty"`                   // 用户设置的原始订单数量
				Type         string            `json:"orderType,omitempty"`   // LIMIT/MARKET
				TimeInForce  string            `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string            `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal   `json:"cumExecQty"`   // 交易的订单数量
				CummQuoteQty decimal.Decimal   `json:"cumExecValue"` // 累计交易的金额
				FeeQty       decimal.Decimal   `json:"cumExecFee"`
				Status       string            `json:"orderStatus,omitempty"`
				Time         string            `json:"createdTime,omitempty"`
				UTime        string            `json:"updatedTime,omitempty"`
				FeeDetail    map[string]string `json:"cumFeeDetail,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	if len(recv.Result.List) == 0 {
		return nil, errors.New(bb.Name() + " resp empty")
	}
	order := recv.Result.List[0]
	o := &SpotOrder{
		Symbol:      order.Symbol,
		OrderId:     order.OrderId,
		ClientId:    order.ClientId,
		Price:       order.Price,
		Qty:         order.Quantity,
		FilledQty:   order.ExecutedQty,
		FilledAmt:   order.CummQuoteQty,
		Status:      bb.toStdOrderStatus(order.Status),
		Type:        bb.toStdOrderType(order.Type),
		TimeInForce: order.TimeInForce,
		Side:        bb.toStdSide(order.Side),
	}
	o.CTime, _ = strconv.ParseInt(order.Time, 10, 64)
	o.UTime, _ = strconv.ParseInt(order.UTime, 10, 64)
	for k, v := range order.FeeDetail {
		o.FeeAsset = k
		o.FeeQty, _ = decimal.NewFromString(v)
		break
	}
	return o, nil
}
func (bb *Bybit) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	query := "category=spot&limit=50&symbol=" + symbol
	url := bbUniEndpoint + "/v5/order/realtime?" + query
	_, resp, err := bb.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol       string            `json:"symbol,omitempty"` // BTCUSDT
				OrderId      string            `json:"orderId,omitempty"`
				ClientId     string            `json:"orderLinkId,omitempty"`
				Price        decimal.Decimal   `json:"price"`
				Quantity     decimal.Decimal   `json:"qty"`                   // 用户设置的原始订单数量
				Type         string            `json:"orderType,omitempty"`   // LIMIT/MARKET
				TimeInForce  string            `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string            `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal   `json:"cumExecQty"`   // 交易的订单数量
				CummQuoteQty decimal.Decimal   `json:"cumExecValue"` // 累计交易的金额
				FeeQty       decimal.Decimal   `json:"cumExecFee"`
				Status       string            `json:"orderStatus,omitempty"`
				Time         string            `json:"createdTime,omitempty"`
				UTime        string            `json:"updatedTime,omitempty"`
				FeeDetail    map[string]string `json:"cumFeeDetail,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	if len(recv.Result.List) == 0 {
		return nil, errors.New(bb.Name() + " resp empty")
	}
	dl := make([]*SpotOrder, 0, len(recv.Result.List))
	for _, order := range recv.Result.List {
		o := &SpotOrder{
			Symbol:      order.Symbol,
			OrderId:     order.OrderId,
			ClientId:    order.ClientId,
			Price:       order.Price,
			Qty:         order.Quantity,
			FilledQty:   order.ExecutedQty,
			FilledAmt:   order.CummQuoteQty,
			Status:      bb.toStdOrderStatus(order.Status),
			Type:        bb.toStdOrderType(order.Type),
			TimeInForce: order.TimeInForce,
			Side:        bb.toStdSide(order.Side),
		}
		o.CTime, _ = strconv.ParseInt(order.Time, 10, 64)
		o.UTime, _ = strconv.ParseInt(order.UTime, 10, 64)
		for k, v := range order.FeeDetail {
			o.FeeAsset = k
			o.FeeQty, _ = decimal.NewFromString(v)
			break
		}
		dl = append(dl, o)
	}
	return dl, nil
}
func (bb *Bybit) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	query := "category=spot&symbol=" + symbol
	url := bbUniEndpoint + "/v5/account/fee-rate?" + query
	_, resp, err := bb.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
	if err != nil {
		return SpotTradeFee{}, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol   string          `json:"symbol"` // BTCUSDT
				TakerFee decimal.Decimal `json:"takerFeeRate"`
				MakerFee decimal.Decimal `json:"makerFeeRate"`
			} `json:"list,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return SpotTradeFee{}, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return SpotTradeFee{}, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	if len(recv.Result.List) == 0 {
		return SpotTradeFee{}, errors.New(bb.Name() + " resp empty")
	}
	for _, v := range recv.Result.List {
		if v.Symbol == symbol {
			return SpotTradeFee{
				Maker:    v.MakerFee,
				Taker:    v.TakerFee,
				Discount: decimal.NewFromFloat(1.0),
			}, nil
		}
	}
	return SpotTradeFee{}, errors.New("not found")
}
