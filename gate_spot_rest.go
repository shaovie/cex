package cex

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ilog"
)

func (gt *Gate) SpotSupported() bool {
	return true
}
func (gt *Gate) SpotServerTime() (int64, error) {
	path := "/api/v4/spot/time"
	url := gtUniEndpoint + path
	_, resp, err := gt.Get(url, gtApiDeadline, nil)
	if err != nil {
		return 0, errors.New(gt.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"server_time,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (gt *Gate) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	path := "/api/v4/spot/currency_pairs"
	url := gtUniEndpoint + path
	_, resp, err := gt.Get(url, gtApiDeadline, nil)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}

	recv := []struct {
		Id           string          `json:"id"`
		Base         string          `json:"base,omitempty"`
		Quote        string          `json:"quote,omitempty"`
		Status       string          `json:"trade_status"`
		MinNotional  decimal.Decimal `json:"min_quote_amount"`
		QtyPrecision int32           `json:"amount_precision,omitempty"`
		MinBaseQty   decimal.Decimal `json:"min_base_amount"`
		Precision    int32           `json:"precision,omitempty"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tgtSpotSymbolMap := make(map[string]string)
	for _, pair := range recv {
		if pair.Status != "tradable" {
			continue
		}

		ep := &SpotExchangePairRule{
			Symbol:      pair.Base + pair.Quote,
			Base:        pair.Base,
			Quote:       pair.Quote,
			Status:      "online",
			Time:        now,
			MinNotional: pair.MinNotional,
		}
		ep.MinPrice = decimal.NewFromFloat(0.000000001)
		ep.MaxPrice = decimal.NewFromFloat(999999999)
		ep.PriceTickSize = PowOneTenth(int(pair.Precision))
		ep.QtyStep = PowOneTenth(int(pair.QtyPrecision))
		ep.MinOrderQty = pair.MinBaseQty
		ep.MaxOrderQty = decimal.NewFromFloat(999999999)
		all[ep.Symbol] = ep
		tgtSpotSymbolMap[ep.Symbol] = pair.Id
	}
	gtSpotSymbolMapMtx.Lock()
	gtSpotSymbolMap = tgtSpotSymbolMap
	gtSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (gt *Gate) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	path := "/api/v4/spot/tickers"
	url := gtUniEndpoint + path
	_, resp, err := gt.Get(url, gtApiDeadline, nil)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] == '{' {
		return nil, gt.handleExceptionResp("SpotGetAll24hTicker", resp)
	}

	tickers := []GateSpot24hTicker{}
	if err = json.Unmarshal(resp, &tickers); err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	if len(tickers) == 0 {
		return nil, errors.New(gt.Name() + " resp empty")
	}
	allTk := make(map[string]Pub24hTicker, len(tickers))
	for _, tk := range tickers {
		v := Pub24hTicker{
			Symbol:      strings.ReplaceAll(tk.Symbol, "_", ""),
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (gt *Gate) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/v4/spot/accounts"
	url := gtUniEndpoint + path
	headers := gt.buildHeaders("GET", path, "", "")
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	assetL := []struct {
		Symbol    string          `json:"currency"`
		Available decimal.Decimal `json:"available"`
		Locked    decimal.Decimal `json:"locked"`
	}{}
	err = json.Unmarshal(resp, &assetL)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] == '{' {
		return nil, gt.handleExceptionResp("SpotGetAllAssets", resp)
	}

	assetsMap := make(map[string]*SpotAsset)
	for _, v := range assetL {
		if v.Available.IsZero() && v.Locked.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &SpotAsset{
			Symbol: v.Symbol,
			Avail:  v.Available,
			Locked: v.Locked,
			Total:  v.Available.Add(v.Locked),
		}
	}
	return assetsMap, nil
}
func (gt *Gate) SpotGetBBO(symbol string) (BestBidAsk, error) {
	symbolS := gt.getSpotSymbol(symbol)
	path := "/api/v4/spot/order_book?limit=1&currency_pair=" + symbolS
	url := gtUniEndpoint + path
	headers := gt.buildHeaders("GET", path, "", "")
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return BestBidAsk{}, errors.New(gt.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Label string               `json:"label"`
		Msg   string               `json:"message"`
		Asks  [][2]decimal.Decimal `json:"asks"`
		Bids  [][2]decimal.Decimal `json:"bids"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return BestBidAsk{}, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}

	if ret.Label != "" {
		return BestBidAsk{}, errors.New(gt.Name() + " request fail! err=" + ret.Msg)
	}
	if len(ret.Asks) == 0 || len(ret.Bids) == 0 {
		return BestBidAsk{}, errors.New(gt.Name() + " resp is empty!")
	}

	return BestBidAsk{
		Symbol:   symbol,
		BidPrice: ret.Bids[0][0],
		BidQty:   ret.Bids[0][1],
		AskPrice: ret.Asks[0][0],
		AskQty:   ret.Asks[0][1],
	}, nil
}
func (gt *Gate) SpotPlaceOrder(symbol, clientId string,
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	path := "/api/v4/spot/orders"
	url := gtUniEndpoint + path
	symbolS := gt.getSpotSymbol(symbol)
	payload := `{"currency_pair":"` + symbolS + `"`
	if clientId != "" {
		payload += `,"text":"t-` + clientId + `"` // len(clientId) LessOrEqual than 28
	}
	if orderType == "LIMIT" {
		payload += `,"price":"` + price.String() + `"`
	} else if orderType == "MARKET" && side == "BUY" {
		qty = amt
	}
	if timeInForce != "" {
		payload += `,"time_in_force":"` + gt.fromStdTimeInForce(timeInForce) + `"`
	} else if orderType == "MARKET" {
		payload += `,"time_in_force":"ioc"`
	}
	payload += "" +
		`,"amount":"` + qty.String() + `"` +
		`,"side":"` + gt.fromStdSide(side) + `"` +
		`,"type":"` + gt.fromStdOrderType(orderType) + `"` + // LIMIT/MARKET
		`}`
	headers := gt.buildHeaders("POST", path, "", payload)
	_, resp, err := gt.Post(url, []byte(payload), gtApiDeadline, headers)
	if err != nil {
		return "", errors.New(gt.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Label   string `json:"label"`
		Msg     string `json:"message"`
		OrderId string `json:"id"`
		Status  string `json:"status"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Label != "" {
		return "", errors.New(gt.Name() + " request fail! err=" + ret.Msg)
	}

	if len(ret.OrderId) == 0 {
		return "", errors.New(gt.Name() + " orderid is empty!")
	}

	return ret.OrderId, nil
}

func (gt *Gate) SpotPlaceOrderMultipleSupported() bool { return true }

// SpotPlaceOrderMultiple 批量下单: 文档要求每单必须带text(自定义order id), 一次最多4个交易对且每对最多10单
// 这里按交易对分组, 每组10单一批, 超出自动分批; 某一批失败立刻返回错误
func (gt *Gate) SpotPlaceOrderMultiple(orders []SpotPostOrder) error {
	if len(orders) == 0 {
		return errors.New(gt.Name() + " orders empty!")
	}
	type BatchOrder struct {
		CurrencyPair string `json:"currency_pair"`
		Text         string `json:"text"`
		Price        string `json:"price,omitempty"`
		Amount       string `json:"amount"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		TimeInForce  string `json:"time_in_force,omitempty"`
	}
	symbolL := make([]string, 0, 4)
	orderMap := make(map[string][]SpotPostOrder, 4)
	for i := range orders {
		if len(orders[i].ClientId) == 0 { // 批量下单必须带自定义order id
			return errors.New(gt.Name() + " batch order needs clientId!")
		}
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return errors.New(gt.Name() + " not support order type! " + orders[i].Type)
		}
		if _, ok := orderMap[orders[i].Symbol]; !ok {
			symbolL = append(symbolL, orders[i].Symbol)
		}
		orderMap[orders[i].Symbol] = append(orderMap[orders[i].Symbol], orders[i])
	}
	for _, symbol := range symbolL {
		symbolS := gt.getSpotSymbol(symbol)
		if len(symbolS) == 0 {
			return errors.New(gt.Name() + " unknown symbol! " + symbol)
		}
		orderL := orderMap[symbol]
		for i := 0; i < len(orderL); i += 10 { // 一个交易对一次最多10单
			end := i + 10
			if end > len(orderL) {
				end = len(orderL)
			}
			batchL := make([]BatchOrder, 0, end-i)
			for k := i; k < end; k++ {
				qty := orderL[k].Qty
				if orderL[k].Type == "MARKET" && orderL[k].Side == "BUY" { // 市价买 amount为计价币金额
					qty = orderL[k].Amt
				}
				po := BatchOrder{
					CurrencyPair: symbolS,
					Text:         "t-" + orderL[k].ClientId,
					Amount:       qty.String(),
					Side:         gt.fromStdSide(orderL[k].Side),
					Type:         gt.fromStdOrderType(orderL[k].Type),
				}
				if orderL[k].Type == "LIMIT" {
					po.Price = orderL[k].Price.String()
				}
				if orderL[k].TimeInForce != "" {
					po.TimeInForce = gt.fromStdTimeInForce(orderL[k].TimeInForce)
				} else if orderL[k].Type == "MARKET" {
					po.TimeInForce = "ioc"
				}
				batchL = append(batchL, po)
			}
			payload, _ := json.Marshal(batchL)
			path := "/api/v4/spot/batch_orders"
			headers := gt.buildHeaders("POST", path, "", string(payload))
			_, resp, err := gt.Post(gtUniEndpoint+path, payload, gtApiDeadline, headers)
			if err != nil {
				return errors.New(gt.Name() + " net error! " + err.Error())
			}
			if len(resp) == 0 || resp[0] != '[' {
				return gt.handleExceptionResp("SpotPlaceOrderMultiple", resp)
			}
			ret := []struct {
				OrderId   string `json:"order_id,omitempty"`
				Id        string `json:"id,omitempty"`
				Succeeded bool   `json:"succeeded,omitempty"`
				Label     string `json:"label,omitempty"`
				Message   string `json:"message,omitempty"`
			}{}
			if err = json.Unmarshal(resp, &ret); err != nil {
				return errors.New(gt.Name() + " unmarshal fail! " + err.Error() + string(resp))
			}
			if len(ret) == 0 {
				return errors.New(gt.Name() + " resp empty! " + string(resp))
			}
			success, failMsg := 0, ""
			for _, v := range ret {
				if v.Succeeded || len(v.OrderId) > 0 || len(v.Id) > 0 {
					success++
				} else if len(failMsg) == 0 {
					failMsg = v.Label + " " + v.Message
				}
			}
			if success == 0 {
				return errors.New(gt.Name() + " batch order all fail! " + failMsg)
			}
		}
	}
	return nil
}
func (gt *Gate) SpotCancelOrder(symbol string, orderId, cltId string) error {
	symbolS := gt.getSpotSymbol(symbol)
	if orderId == "" && cltId != "" {
		orderId = "t-" + cltId
	}
	path := "/api/v4/spot/orders/" + orderId
	params := "currency_pair=" + symbolS
	headers := gt.buildHeaders("DELETE", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	_, resp, err := gt.Delete(url, gtApiDeadline, headers)
	if err != nil {
		return errors.New(gt.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Label string `json:"label"`
		Msg   string `json:"message"`

		Status string `json:"status"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(gt.Name() + " unmarshal fail! " + err.Error() + string(resp))
	}
	if ret.Label != "" {
		return errors.New(gt.Name() + " request fail! " + ret.Msg)
	}

	if gt.toStdOrderStatus(ret.Status) != "CANCELED" {
		return errors.New(gt.Name() + " cancel failed! status now: " + ret.Status)
	}
	return nil
}

func (gt *Gate) SpotCancelOrderMultipleSupported() bool { return true }

// SpotCancelOrderMultiple 批量撤单: 一次最多20单, 超出自动分批; 只要有成功的就算成功
func (gt *Gate) SpotCancelOrderMultiple(symbol string, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(gt.Name() + " orderIds or cltIds empty!")
	}
	symbolS := gt.getSpotSymbol(symbol)
	if len(symbolS) == 0 {
		return errors.New(gt.Name() + " unknown symbol! " + symbol)
	}
	type CancelOrder struct {
		CurrencyPair string `json:"currency_pair"`
		Id           string `json:"id"`
	}
	orderL := make([]CancelOrder, 0, len(orderIds)+len(cltIds))
	for _, v := range orderIds {
		orderL = append(orderL, CancelOrder{CurrencyPair: symbolS, Id: v})
	}
	for _, v := range cltIds { // 自定义order id用t-前缀
		orderL = append(orderL, CancelOrder{CurrencyPair: symbolS, Id: "t-" + v})
	}
	for i := 0; i < len(orderL); i += 20 { // 一次最多20单
		end := i + 20
		if end > len(orderL) {
			end = len(orderL)
		}
		payload, _ := json.Marshal(orderL[i:end])
		path := "/api/v4/spot/cancel_batch_orders"
		headers := gt.buildHeaders("POST", path, "", string(payload))
		_, resp, err := gt.Post(gtUniEndpoint+path, payload, gtApiDeadline, headers)
		if err != nil {
			return errors.New(gt.Name() + " net error! " + err.Error())
		}
		if len(resp) == 0 || resp[0] != '[' {
			return gt.handleExceptionResp("SpotCancelOrderMultiple", resp)
		}
		ret := []struct {
			Id        string `json:"id,omitempty"`
			Succeeded bool   `json:"succeeded,omitempty"`
			Label     string `json:"label,omitempty"`
			Message   string `json:"message,omitempty"`
		}{}
		if err = json.Unmarshal(resp, &ret); err != nil {
			return errors.New(gt.Name() + " unmarshal fail! " + err.Error() + string(resp))
		}
		if len(ret) == 0 {
			return errors.New(gt.Name() + " resp empty! " + string(resp))
		}
		success, failMsg := 0, ""
		for _, v := range ret {
			if v.Succeeded {
				success++
			} else if len(failMsg) == 0 {
				failMsg = v.Label + " " + v.Message
			}
		}
		if success == 0 {
			return errors.New(gt.Name() + " batch cancel all fail! " + failMsg)
		}
	}
	return nil
}

func (gt *Gate) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	symbolS := gt.getSpotSymbol(symbol)
	if orderId == "" && cltId != "" {
		orderId = "t-" + cltId
	}
	path := "/api/v4/spot/orders/" + orderId
	params := "currency_pair=" + symbolS
	headers := gt.buildHeaders("GET", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	order := struct {
		Label string `json:"label"`
		Msg   string `json:"message"`

		Symbol       string          `json:"currency_pair"`
		OrderId      string          `json:"id"`
		ClientId     string          `json:"text"`
		Price        decimal.Decimal `json:"price"`
		Qty          decimal.Decimal `json:"amount"`
		ExecutedQty  decimal.Decimal `json:"filled_amount"`
		CummQuoteQty decimal.Decimal `json:"filled_total"`
		Left         decimal.Decimal `json:"left"`
		Status       string          `json:"status"`
		Type         string          `json:"type"`
		TimeInForce  string          `json:"time_in_force"` // GTC/FOK/IOC
		Side         string          `json:"side"`
		FeeCoin      string          `json:"fee_currency"`
		FeeQty       decimal.Decimal `json:"fee"`
		GtQty        decimal.Decimal `json:"gt_fee"`    //
		Event        string          `json:"event"`     //
		FinishAs     string          `json:"finish_as"` //
		Time         int64           `json:"create_time_ms"`
		UTime        int64           `json:"update_time_ms"`
	}{}
	err = json.Unmarshal(resp, &order)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}
	if order.Label != "" {
		return nil, errors.New(gt.Name() + " resp err! " + order.Msg)
	}

	clientId := ""
	idx := strings.Index(order.ClientId, "t-")
	if idx != -1 && len(order.ClientId) > 2 {
		clientId = order.ClientId[2:]
	}
	if !order.GtQty.IsZero() {
		order.FeeCoin = "GT"
		order.FeeQty = order.GtQty
	}
	if order.Left.IsPositive() && order.Qty.GreaterThan(order.Left) {
		order.Status = "partially_filled"
	}
	if order.Event == "put" {
		order.Status = "open"
	} else if order.Event == "update" {
		if order.FinishAs == "open" {
			order.Status = "partially_filled"
		}
	} else if order.Event == "finish" {
		if order.FinishAs == "filled" {
			order.Status = "filled"
		} else if order.FinishAs == "cancelled" {
			order.Status = "cancelled"
		}
	}
	if order.Status == "" {
		ilog.Error(gt.Name() + " unknow s status" + string(resp))
	}
	return &SpotOrder{
		Symbol:      symbol,
		OrderId:     order.OrderId,
		ClientId:    clientId,
		Price:       order.Price,
		Qty:         order.Qty,
		FilledQty:   order.ExecutedQty,
		FilledAmt:   order.CummQuoteQty,
		Status:      gt.toStdOrderStatus(order.Status),
		Type:        gt.toStdOrderType(order.Type),
		TimeInForce: gt.toStdTimeInForce(order.TimeInForce),
		Side:        gt.toStdSide(order.Side),
		FeeQty:      order.FeeQty.Neg(),
		FeeAsset:    order.FeeCoin,
		CTime:       order.Time,
		UTime:       order.UTime,
	}, nil
}
func (gt *Gate) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	path := "/api/v4/spot/open_orders"
	url := gtUniEndpoint + path
	headers := gt.buildHeaders("GET", path, "", "")
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, gt.handleExceptionResp("SpotGetOpenOrders", resp)
	}
	orders := []struct {
		L []struct {
			Symbol       string          `json:"currency_pair"`
			OrderId      string          `json:"id"`
			ClientId     string          `json:"text"`
			Price        decimal.Decimal `json:"price"`
			Qty          decimal.Decimal `json:"amount"`
			ExecutedQty  decimal.Decimal `json:"filled_amount"`
			CummQuoteQty decimal.Decimal `json:"filled_total"`
			Left         decimal.Decimal `json:"left"`
			Status       string          `json:"status"`
			Type         string          `json:"type"`
			TimeInForce  string          `json:"time_in_force"` // GTC/FOK/IOC
			Side         string          `json:"side"`
			FeeCoin      string          `json:"fee_currency"`
			FeeQty       decimal.Decimal `json:"fee"`
			GtQty        decimal.Decimal `json:"gt_fee"`    //
			Event        string          `json:"event"`     //
			FinishAs     string          `json:"finish_as"` //
			Time         int64           `json:"create_time_ms"`
			UTime        int64           `json:"update_time_ms"`
		} `json:"orders"`
	}{}
	err = json.Unmarshal(resp, &orders)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}

	dl := make([]*SpotOrder, 0, len(orders))
	for _, vl := range orders {
		for _, order := range vl.L {
			clientId := ""
			idx := strings.Index(order.ClientId, "t-")
			if idx != -1 && len(order.ClientId) > 2 {
				clientId = order.ClientId[2:]
			}
			if !order.GtQty.IsZero() {
				order.FeeCoin = "GT"
				order.FeeQty = order.GtQty
			}
			if order.Left.IsPositive() && order.Qty.GreaterThan(order.Left) {
				order.Status = "partially_filled"
			}
			if order.Event == "put" {
				order.Status = "open"
			} else if order.Event == "update" {
				if order.FinishAs == "open" {
					order.Status = "partially_filled"
				}
			} else if order.Event == "finish" {
				if order.FinishAs == "filled" {
					order.Status = "filled"
				} else if order.FinishAs == "cancelled" {
					order.Status = "cancelled"
				}
			}
			if order.Status == "" {
				ilog.Error(gt.Name() + " unknow s status" + string(resp))
			}
			so := SpotOrder{
				Symbol:      strings.ReplaceAll(order.Symbol, "_", ""),
				OrderId:     order.OrderId,
				ClientId:    clientId,
				Price:       order.Price,
				Qty:         order.Qty,
				FilledQty:   order.ExecutedQty,
				FilledAmt:   order.CummQuoteQty,
				Status:      gt.toStdOrderStatus(order.Status),
				Type:        gt.toStdOrderType(order.Type),
				TimeInForce: gt.toStdTimeInForce(order.TimeInForce),
				Side:        gt.toStdSide(order.Side),
				FeeQty:      order.FeeQty.Neg(),
				FeeAsset:    order.FeeCoin,
				CTime:       order.Time,
				UTime:       order.UTime,
			}
			dl = append(dl, &so)
		}
	}
	return dl, nil
}

// SpotGetFilledOrders 查已结束的订单(全成/已撤): 按订单结束时间过滤, 不传交易对就是所有
func (gt *Gate) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	path := "/api/v4/spot/orders"
	params := "status=finished&limit=100"
	if symbol != "" {
		symbolS := gt.getSpotSymbol(symbol)
		if len(symbolS) == 0 {
			return nil, errors.New(gt.Name() + " unknown symbol! " + symbol)
		}
		params += "&currency_pair=" + symbolS
	}
	headers := gt.buildHeaders("GET", path, params, "")
	_, resp, err := gt.Get(gtUniEndpoint+path+"?"+params, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, gt.handleExceptionResp("SpotGetFilledOrders", resp)
	}
	orders := []struct {
		Symbol       string          `json:"currency_pair"`
		OrderId      string          `json:"id"`
		ClientId     string          `json:"text"`
		Price        decimal.Decimal `json:"price"`
		Qty          decimal.Decimal `json:"amount"`
		ExecutedQty  decimal.Decimal `json:"filled_amount"`
		CummQuoteQty decimal.Decimal `json:"filled_total"`
		Status       string          `json:"status"`
		Type         string          `json:"type"`
		TimeInForce  string          `json:"time_in_force"` // GTC/FOK/IOC
		Side         string          `json:"side"`
		FeeCoin      string          `json:"fee_currency"`
		FeeQty       decimal.Decimal `json:"fee"`
		GtQty        decimal.Decimal `json:"gt_fee"`
		Time         int64           `json:"create_time_ms"`
		UTime        int64           `json:"update_time_ms"`
	}{}
	if err = json.Unmarshal(resp, &orders); err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*SpotOrder, 0, len(orders))
	for i := range orders {
		clientId := ""
		idx := strings.Index(orders[i].ClientId, "t-")
		if idx != -1 && len(orders[i].ClientId) > 2 {
			clientId = orders[i].ClientId[2:]
		}
		if !orders[i].GtQty.IsZero() {
			orders[i].FeeCoin = "GT"
			orders[i].FeeQty = orders[i].GtQty
		}
		dl = append(dl, &SpotOrder{
			Symbol:      strings.ReplaceAll(orders[i].Symbol, "_", ""),
			OrderId:     orders[i].OrderId,
			ClientId:    clientId,
			Price:       orders[i].Price,
			Qty:         orders[i].Qty,
			FilledQty:   orders[i].ExecutedQty,
			FilledAmt:   orders[i].CummQuoteQty,
			Status:      gt.toStdOrderStatus(orders[i].Status), // 已结束的单子状态是最终的, 不再按剩余量改成部分成交
			Type:        gt.toStdOrderType(orders[i].Type),
			TimeInForce: gt.toStdTimeInForce(orders[i].TimeInForce),
			Side:        gt.toStdSide(orders[i].Side),
			FeeQty:      orders[i].FeeQty.Neg(),
			FeeAsset:    orders[i].FeeCoin,
			CTime:       orders[i].Time,
			UTime:       orders[i].UTime,
		})
	}
	return dl, nil
}
