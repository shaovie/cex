package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (bg *Bitget) SpotSupported() bool {
	return true
}

// SpotServerTime 统一账户没有独立的服务器时间接口, 用公共接口
func (bg *Bitget) SpotServerTime() (int64, error) {
	path := "/api/v2/public/time"
	link := bgEndpoint + path
	_, resp, err := bg.Get(link, bgApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			ServerTime string `json:"serverTime,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return 0, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	t, _ := strconv.ParseInt(recv.Data.ServerTime, 10, 64)
	return t, nil
}
func (bg *Bitget) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	path := "/api/v3/market/instruments"
	params := "category=SPOT"
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.GetWithCache(link, bgApiDeadline, nil, spotPairRuleCacheFile(bg.Name()))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			Symbol            string          `json:"symbol"`
			Base              string          `json:"baseCoin,omitempty"`
			Quote             string          `json:"quoteCoin,omitempty"`
			Status            string          `json:"status,omitempty"`
			PricePrecision    string          `json:"pricePrecision,omitempty"`
			QuantityPrecision string          `json:"quantityPrecision,omitempty"`
			MinOrderQty       decimal.Decimal `json:"minOrderQty"`
			MaxOrderQty       decimal.Decimal `json:"maxOrderQty"`
			MinOrderAmount    decimal.Decimal `json:"minOrderAmount"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}

	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Data {
		if pair.Status != "online" { // online 可交易
			continue
		}
		pricePrecision, _ := strconv.Atoi(pair.PricePrecision)
		quantityPrecision, _ := strconv.Atoi(pair.QuantityPrecision)
		ep := &SpotExchangePairRule{
			Symbol:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
			Status:        "online",
			PriceTickSize: PowOneTenth(pricePrecision),
			QtyStep:       PowOneTenth(quantityPrecision),
			MinOrderQty:   pair.MinOrderQty,
			MaxOrderQty:   pair.MaxOrderQty,
			MinNotional:   pair.MinOrderAmount,
			Time:          now,
		}
		ep.MinPrice = decimal.NewFromFloat(0.000000001)
		ep.MaxPrice = decimal.NewFromFloat(999999999)
		if !ep.MaxOrderQty.IsPositive() {
			ep.MaxOrderQty = decimal.NewFromFloat(999999999)
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (bg *Bitget) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	path := "/api/v3/market/tickers"
	params := "category=SPOT"
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string             `json:"code,omitempty"`
		Msg  string             `json:"msg,omitempty"`
		Data []BitgetSpotTicker `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data) == 0 {
		return nil, errors.New(bg.Name() + " resp empty")
	}

	allTk := make(map[string]Pub24hTicker, len(recv.Data))
	for _, tk := range recv.Data {
		v := Pub24hTicker{
			Symbol:      tk.Symbol,
			LastPrice:   tk.toDecimal(tk.LastPrice),
			Volume:      tk.toDecimal(tk.BaseVolume),
			QuoteVolume: tk.toDecimal(tk.QuoteVolume),
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (bg *Bitget) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/v3/account/assets"
	link := bgEndpoint + path
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, "", ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			Assets []BitgetAsset `json:"assets"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}

	assetsMap := make(map[string]*SpotAsset, len(recv.Data.Assets))
	for _, v := range recv.Data.Assets {
		if v.Available.IsZero() && v.Balance.IsZero() && v.Locked.IsZero() {
			continue
		}
		assetsMap[v.Coin] = &SpotAsset{
			Symbol: v.Coin,
			Avail:  v.Available,
			Locked: v.Locked,
			Total:  v.Balance,
		}
	}
	return assetsMap, nil
}
func (bg *Bitget) SpotGetBBO(symbol string) (BestBidAsk, error) {
	path := "/api/v3/market/orderbook"
	params := "category=SPOT&limit=1&symbol=" + symbol
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			Ts string               `json:"ts,omitempty"`
			A  [][2]decimal.Decimal `json:"a,omitempty"`
			B  [][2]decimal.Decimal `json:"b,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return BestBidAsk{}, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return BestBidAsk{}, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.A) == 0 || len(recv.Data.B) == 0 {
		return BestBidAsk{}, errors.New(bg.Name() + " resp empty!")
	}
	ts, _ := strconv.ParseInt(recv.Data.Ts, 10, 64)
	return BestBidAsk{
		Symbol:   symbol,
		Time:     ts,
		BidPrice: recv.Data.B[0][0],
		BidQty:   recv.Data.B[0][1],
		AskPrice: recv.Data.A[0][0],
		AskQty:   recv.Data.A[0][1],
	}, nil
}
func (bg *Bitget) SpotGetOrderBook(symbol string, depth int64) (*OrderBookDepth, error) {
	if depth <= 0 || depth > 1000 {
		depth = 1000 // 该接口最大1000档
	}
	path := "/api/v3/market/orderbook"
	params := "category=SPOT&limit=" + strconv.FormatInt(depth, 10) + "&symbol=" + symbol
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			Ts string               `json:"ts,omitempty"`
			A  [][2]decimal.Decimal `json:"a,omitempty"`
			B  [][2]decimal.Decimal `json:"b,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.A) == 0 || len(recv.Data.B) == 0 {
		return nil, errors.New(bg.Name() + " resp empty!")
	}
	ts, _ := strconv.ParseInt(recv.Data.Ts, 10, 64)
	obd := &OrderBookDepth{
		Symbol: symbol,
		Level:  int(depth),
		Time:   ts,
		Bids:   make([]Ticker, 0, len(recv.Data.B)),
		Asks:   make([]Ticker, 0, len(recv.Data.A)),
	}
	for _, v := range recv.Data.B {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range recv.Data.A {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	return obd, nil
}
func (bg *Bitget) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	var f SpotTradeFee
	path := "/api/v3/account/fee-rate"
	params := "category=SPOT&symbol=" + symbol
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return f, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			MakerFeeRate decimal.Decimal `json:"makerFeeRate"`
			TakerFeeRate decimal.Decimal `json:"takerFeeRate"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return f, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "00000" {
		return f, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	f.Maker = recv.Data.MakerFeeRate
	f.Taker = recv.Data.TakerFeeRate
	return f, nil
}
func (bg *Bitget) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	req := struct {
		Category    string `json:"category"`
		Symbol      string `json:"symbol"`
		Side        string `json:"side"`
		OrderType   string `json:"orderType"`
		TimeInForce string `json:"timeInForce,omitempty"`
		Price       string `json:"price,omitempty"`
		Qty         string `json:"qty"`
		ClientOid   string `json:"clientOid,omitempty"`
		StpMode     string `json:"stpMode,omitempty"`
	}{
		Category:  "SPOT",
		Symbol:    symbol,
		Side:      bg.fromStdSide(side),
		OrderType: bg.fromStdOrderType(orderType),
		StpMode:   "cancel_both", // 做市需要
	}
	if orderType == "LIMIT" {
		req.Price = price.String()
		req.Qty = qty.String()
	} else if orderType == "MARKET" {
		// 市价买 qty为计价币金额, 市价卖为交易币数量
		if side == "BUY" {
			req.Qty = amt.String()
		} else {
			req.Qty = qty.String()
		}
	} else {
		return "", errors.New(bg.Name() + " not support order type! " + orderType)
	}
	if timeInForce != "" {
		req.TimeInForce = bg.fromStdTimeInForce(timeInForce)
	} else if orderType == "MARKET" {
		req.TimeInForce = "ioc"
	}
	if postOnly {
		req.TimeInForce = "post_only"
	}
	if cltId != "" {
		req.ClientOid = cltId
	}
	payload, _ := json.Marshal(req)
	path := "/api/v3/trade/place-order"
	link := bgEndpoint + path
	_, resp, err := bg.Post(link, payload, bgApiDeadline,
		bg.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return "", errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			OrderId   string `json:"orderId,omitempty"`
			ClientOid string `json:"clientOid,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return "", errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data.OrderId) == 0 {
		return "", errors.New(bg.Name() + " orderId is empty!")
	}
	return ret.Data.OrderId, nil
}

func (bg *Bitget) SpotPlaceOrderMultipleSupported() bool { return true }

// SpotPlaceOrderMultiple 批量下单: 一次最多20单, 超过自动分批; 支持部分成功
// 市价买qty用计价币金额, 市价卖用交易币数量; postOnly优先
func (bg *Bitget) SpotPlaceOrderMultiple(orders []SpotPostOrder) ([]PostOrderResult, error) {
	if len(orders) == 0 {
		return nil, errors.New(bg.Name() + " orders empty!")
	}
	type BatchOrder struct {
		Category    string `json:"category"`
		Symbol      string `json:"symbol"`
		Side        string `json:"side"`
		OrderType   string `json:"orderType"`
		TimeInForce string `json:"timeInForce,omitempty"`
		Price       string `json:"price,omitempty"`
		Qty         string `json:"qty"`
		ClientOid   string `json:"clientOid,omitempty"`
		StpMode     string `json:"stpMode,omitempty"`
	}
	orderL := make([]BatchOrder, 0, len(orders))
	for i := range orders {
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return nil, errors.New(bg.Name() + " not support order type! " + orders[i].Type)
		}
		po := BatchOrder{
			Category:  "SPOT",
			Symbol:    orders[i].Symbol,
			Side:      bg.fromStdSide(orders[i].Side),
			OrderType: bg.fromStdOrderType(orders[i].Type),
			ClientOid: orders[i].ClientId,
			StpMode:   "cancel_both", // 做市需要
		}
		if orders[i].Type == "LIMIT" {
			po.Price = orders[i].Price.String()
			po.Qty = orders[i].Qty.String()
		} else if orders[i].Side == "BUY" { // 市价买 qty为计价币金额
			po.Qty = orders[i].Amt.String()
		} else { // 市价卖为交易币数量
			po.Qty = orders[i].Qty.String()
		}
		if orders[i].PostOnly {
			po.TimeInForce = "post_only"
		} else if orders[i].TimeInForce != "" {
			po.TimeInForce = bg.fromStdTimeInForce(orders[i].TimeInForce)
		} else if orders[i].Type == "MARKET" {
			po.TimeInForce = "ioc"
		}
		orderL = append(orderL, po)
	}
	var res []PostOrderResult
	for i := 0; i < len(orderL); i += 20 { // 一次最多20单
		end := i + 20
		if end > len(orderL) {
			end = len(orderL)
		}
		payload, _ := json.Marshal(orderL[i:end])
		path := "/api/v3/trade/place-batch"
		link := bgEndpoint + path
		_, resp, err := bg.Post(link, payload, bgApiDeadline,
			bg.buildHeaders("POST", path, "", string(payload)))
		if err != nil {
			return res, errors.New(bg.Name() + " net error! " + err.Error())
		}
		ret := struct {
			Code string `json:"code,omitempty"`
			Msg  string `json:"msg,omitempty"`
			Data []struct {
				OrderId   string `json:"orderId,omitempty"`
				ClientOid string `json:"clientOid,omitempty"`
				Code      string `json:"code,omitempty"`
				Msg       string `json:"msg,omitempty"`
			} `json:"data"`
		}{}
		if err = json.Unmarshal(resp, &ret); err != nil {
			return res, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
		}
		if ret.Code != "00000" {
			return res, errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
		}
		before, failMsg := len(res), ""
		for _, v := range ret.Data {
			if len(v.OrderId) > 0 {
				res = append(res, PostOrderResult{ClientId: v.ClientOid, OrderId: v.OrderId})
			} else if len(failMsg) == 0 {
				failMsg = v.Code + " " + v.Msg
			}
		}
		if len(res) == before {
			return res, errors.New(bg.Name() + " batch order all fail! " + failMsg)
		}
	}
	return res, nil
}
func (bg *Bitget) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	req := struct {
		Category  string `json:"category"`
		OrderId   string `json:"orderId,omitempty"`
		ClientOid string `json:"clientOid,omitempty"`
	}{Category: "SPOT"}
	if orderId != "" {
		req.OrderId = orderId
	} else if cltId != "" {
		req.ClientOid = cltId
	} else {
		return errors.New(bg.Name() + " orderId or cltId empty!")
	}
	payload, _ := json.Marshal(req)
	path := "/api/v3/trade/cancel-order"
	link := bgEndpoint + path
	_, resp, err := bg.Post(link, payload, bgApiDeadline,
		bg.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			OrderId   string `json:"orderId,omitempty"`
			ClientOid string `json:"clientOid,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}
	return nil
}

func (bg *Bitget) SpotCancelOrderMultipleSupported() bool { return true }

// SpotCancelOrderMultiple 批量撤单: 一次最多20单, 超过自动分批; 支持部分成功
// 文档要求同一请求只能用orderId或clientOid其中一种, 所以orderIds和cltIds分开各发一次
func (bg *Bitget) SpotCancelOrderMultiple(symbol string, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(bg.Name() + " orderIds or cltIds empty!")
	}
	type CancelOrder struct {
		Category  string `json:"category"`
		Symbol    string `json:"symbol"`
		OrderId   string `json:"orderId,omitempty"`
		ClientOid string `json:"clientOid,omitempty"`
	}
	orderL := make([]CancelOrder, 0, len(orderIds)+len(cltIds))
	for _, v := range orderIds {
		orderL = append(orderL, CancelOrder{Category: "SPOT", Symbol: symbol, OrderId: v})
	}
	for i := 0; i < len(orderL); i += 20 { // 一次最多20单
		end := i + 20
		if end > len(orderL) {
			end = len(orderL)
		}
		payload, _ := json.Marshal(orderL[i:end])
		if err := bg.spotCancelOrderBatch(payload); err != nil {
			return err
		}
	}
	orderL = orderL[:0]
	for _, v := range cltIds {
		orderL = append(orderL, CancelOrder{Category: "SPOT", Symbol: symbol, ClientOid: v})
	}
	for i := 0; i < len(orderL); i += 20 {
		end := i + 20
		if end > len(orderL) {
			end = len(orderL)
		}
		payload, _ := json.Marshal(orderL[i:end])
		if err := bg.spotCancelOrderBatch(payload); err != nil {
			return err
		}
	}
	return nil
}
func (bg *Bitget) spotCancelOrderBatch(payload []byte) error {
	path := "/api/v3/trade/cancel-batch"
	link := bgEndpoint + path
	_, resp, err := bg.Post(link, payload, bgApiDeadline,
		bg.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId   string `json:"orderId,omitempty"`
			ClientOid string `json:"clientOid,omitempty"`
			Code      string `json:"code,omitempty"`
			Msg       string `json:"msg,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}
	success, failMsg := 0, ""
	for _, v := range ret.Data {
		if len(v.Code) == 0 || v.Code == "00000" {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.Code + " " + v.Msg
		}
	}
	if success == 0 {
		return errors.New(bg.Name() + " batch cancel all fail! " + failMsg)
	}
	return nil
}

func (bg *Bitget) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	path := "/api/v3/trade/order-info"
	params := ""
	if orderId != "" {
		params = "orderId=" + orderId
	} else if cltId != "" {
		params = "clientOid=" + cltId
	} else {
		return nil, errors.New(bg.Name() + " orderId or cltId empty!")
	}
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string          `json:"code,omitempty"`
		Msg  string          `json:"msg,omitempty"`
		Data BitgetSpotOrder `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	if len(recv.Data.OrderId) == 0 {
		return nil, errors.New(bg.Name() + " resp empty!")
	}
	return bg.toStdOrder(&recv.Data), nil
}
func (bg *Bitget) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	path := "/api/v3/trade/unfilled-orders"
	dl := make([]*SpotOrder, 0, 128)
	cursor, lastCursor := "", "" // 游标翻页: 不带cursor取第一页, 之后用上一页返回的cursor
	for {
		params := "category=SPOT&limit=100"
		if symbol != "" {
			params += "&symbol=" + symbol
		}
		if cursor != "" {
			params += "&cursor=" + cursor
		}
		link := bgEndpoint + path + "?" + params
		_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
		if err != nil {
			return nil, errors.New(bg.Name() + " net error! " + err.Error())
		}
		recv := struct {
			Code string `json:"code,omitempty"`
			Msg  string `json:"msg,omitempty"`
			Data struct {
				List   []BitgetSpotOrder `json:"list"`
				Cursor string            `json:"cursor"`
			} `json:"data"`
		}{}
		if err = json.Unmarshal(resp, &recv); err != nil {
			return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
		}
		if recv.Code != "00000" {
			return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
		}
		for i := range recv.Data.List {
			dl = append(dl, bg.toStdOrder(&recv.Data.List[i]))
		}
		if len(recv.Data.List) == 0 || recv.Data.Cursor == "" || recv.Data.Cursor == lastCursor {
			break
		}
		lastCursor, cursor = cursor, recv.Data.Cursor
	}
	return dl, nil
}
func (bg *Bitget) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	path := "/api/v3/trade/history-orders"
	params := "category=SPOT&limit=100"
	if symbol != "" {
		params += "&symbol=" + symbol
	}
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			List []BitgetSpotOrder `json:"list"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + recv.Code + " " + recv.Msg)
	}
	dl := make([]*SpotOrder, 0, len(recv.Data.List))
	for i := range recv.Data.List {
		dl = append(dl, bg.toStdOrder(&recv.Data.List[i]))
	}
	return dl, nil
}

// toStdOrder 交易所订单转标准订单
func (bg *Bitget) toStdOrder(v *BitgetSpotOrder) *SpotOrder {
	ctime, _ := strconv.ParseInt(v.CreatedTime, 10, 64)
	utime, _ := strconv.ParseInt(v.UpdatedTime, 10, 64)
	price := v.Price
	if price.IsZero() {
		price = v.AvgPrice
	}
	o := &SpotOrder{
		Symbol:      strings.ToUpper(v.Symbol),
		OrderId:     v.OrderId,
		ClientId:    v.ClientOid,
		Price:       price,
		Qty:         v.Qty,
		FilledQty:   v.CumExecQty,
		FilledAmt:   v.CumExecValue,
		AvgPrice:    v.AvgPrice,
		Status:      bg.toStdOrderStatus(v.OrderStatus),
		Type:        bg.toStdOrderType(v.OrderType),
		TimeInForce: bg.toStdTimeInForce(v.TimeInForce),
		Side:        bg.toStdSide(v.Side),
		FeeQty:      v.Fee.Neg(),
		FeeAsset:    v.FeeCoin,
		CTime:       ctime,
		UTime:       utime,
	}
	return o
}
