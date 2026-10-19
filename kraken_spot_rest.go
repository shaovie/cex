package cex

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

// = assets
func (kk *Kraken) SpotSupported() bool {
	return true
}
func (kk *Kraken) SpotServerTime() (int64, error) {
	url := kkSpotEndpoint + "/0/public/Time"
	_, resp, err := ihttp.Get(url, kkApiDeadline, nil)
	if err != nil {
		return 0, errors.New(kk.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Err    []string `json:"error"`
		Result struct {
			Timestamp int64 `json:"unixtime"` // second
		} `json:"result"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(kk.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Err) != 0 {
		return 0, errors.New(recv.Err[0])
	}
	return recv.Result.Timestamp * 1000, nil
}
func (kk *Kraken) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := kkSpotEndpoint + "/0/public/AssetPairs"
	_, resp, err := ihttp.Get(link, kkApiDeadline, nil)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}

	type Rule struct {
		WSName      string          `json:"wsname"`
		AltName     string          `json:"altname"`
		Base        string          `json:"base"`
		Quote       string          `json:"quote"`
		Status      string          `json:"status"`
		TickSz      decimal.Decimal `json:"tick_size"`
		LotSz       int64           `json:"lot_decimals"`
		MinOrderQty decimal.Decimal `json:"ordermin"`
		MinNotional decimal.Decimal `json:"costmin"`
	}
	ret := struct {
		Error  []string         `json:"error"`
		Result map[string]*Rule `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tkkSpotSymbolMap := make(map[string]string)
	tkkSpotWssSymbolMap := make(map[string]string)
	tkkXStocksSymbolMap := make(map[string]string)
	tkkSpotAltSymbolMap := make(map[string]string)
	for id, pair := range ret.Result {
		if pair.Status != "online" && pair.Status != "post_only" { // open for trading
			continue
		}
		base := kk.toStdSymbol(pair.Base)
		quote := kk.toStdSymbol(pair.Quote)
		pow := decimal.NewFromInt(10).Pow(decimal.NewFromInt(pair.LotSz))
		lotSz := decimal.NewFromFloat(1.0).Div(pow).Truncate(int32(pair.LotSz))
		ep := &SpotExchangePairRule{
			Symbol:        base + quote,
			Base:          base,
			Quote:         quote,
			Status:        pair.Status,
			PriceTickSize: pair.TickSz,
			QtyStep:       lotSz,
			MinOrderQty:   pair.MinOrderQty,
			MaxOrderQty:   decimal.NewFromFloat(999999999999.99),
			MinPrice:      pair.TickSz,
			MaxPrice:      decimal.NewFromFloat(999999999999.99),
			MinNotional:   pair.MinNotional,
			Time:          now,
		}
		all[ep.Symbol] = ep
		tkkSpotSymbolMap[ep.Symbol] = id
		tkkSpotWssSymbolMap[ep.Symbol] = base + "/" + quote
		tkkSpotAltSymbolMap[id] = ep.Symbol
		if len(pair.AltName) > 0 {
			tkkSpotAltSymbolMap[pair.AltName] = ep.Symbol
		}
	}

	// get xstocks
	link = kkSpotEndpoint + "/0/public/AssetPairs?aclass_base=tokenized_asset"
	_, resp, err = ihttp.Get(link, kkApiDeadline, nil)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	ret2 := struct {
		Error  []string         `json:"error"`
		Result map[string]*Rule `json:"result"`
	}{}

	err = json.Unmarshal(resp, &ret2)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	for id, pair := range ret2.Result {
		_, _, ok := strings.Cut(id, "x")
		if !ok {
			continue
		}
		if pair.Status != "online" && pair.Status != "post_only" { // open for trading
			continue
		}
		pow := decimal.NewFromInt(10).Pow(decimal.NewFromInt(pair.LotSz))
		lotSz := decimal.NewFromFloat(1.0).Div(pow).Truncate(int32(pair.LotSz))
		ep := &SpotExchangePairRule{
			Symbol:        id,
			Base:          pair.Base,
			Quote:         "USD",
			Status:        pair.Status,
			PriceTickSize: pair.TickSz,
			QtyStep:       lotSz,
			MinOrderQty:   pair.MinOrderQty,
			MaxOrderQty:   decimal.NewFromFloat(999999999999.99),
			MinPrice:      pair.TickSz,
			MaxPrice:      decimal.NewFromFloat(999999999999.99),
			MinNotional:   pair.MinNotional,
			Time:          now,
		}
		all[ep.Symbol] = ep
		tkkSpotSymbolMap[ep.Symbol] = id
		tkkSpotWssSymbolMap[ep.Symbol] = ep.Base + "/" + ep.Quote
		tkkXStocksSymbolMap[ep.Symbol] = id
		tkkSpotAltSymbolMap[id] = ep.Symbol
		if len(pair.AltName) > 0 {
			tkkSpotAltSymbolMap[pair.AltName] = ep.Symbol
		}
	}

	kkSpotSymbolMapMtx.Lock()
	kkSpotSymbolMap = tkkSpotSymbolMap
	kkSpotSymbolMapMtx.Unlock()

	kkSpotWssSymbolMapMtx.Lock()
	kkSpotWssSymbolMap = tkkSpotWssSymbolMap
	kkSpotWssSymbolMapMtx.Unlock()

	kkXStocksSymbolMapMtx.Lock()
	kkXStocksSymbolMap = tkkXStocksSymbolMap
	kkXStocksSymbolMapMtx.Unlock()

	kkSpotAltSymbolMapMtx.Lock()
	kkSpotAltSymbolMap = tkkSpotAltSymbolMap
	kkSpotAltSymbolMapMtx.Unlock()
	return all, nil
}
func (kk *Kraken) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/0/private/Balance"
	link := kkSpotEndpoint + path
	values := url.Values{}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string          `json:"error,omitempty"`
		Result map[string]string `json:"result,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return nil, errors.New(kk.Name() + " spot get assets fail! " + recv.Error[0])
	}
	if len(recv.Result) == 0 {
		return nil, errors.New(kk.Name() + " spot get assets fail!")
	}
	spotAssets := make(map[string]*SpotAsset)
	for k, asset := range recv.Result {
		symbol := strings.ReplaceAll(kk.toStdSymbol(k), "x.T", "x")
		v, _ := decimal.NewFromString(asset)
		as := &SpotAsset{
			Symbol: symbol,
			Avail:  v,
			Total:  v,
		}
		spotAssets[as.Symbol] = as
	}

	return spotAssets, nil
}
func (kk *Kraken) SpotGetBBO(symbol string) (BestBidAsk, error) {
	symbolS := kk.getSpotSymbol(symbol)
	url := kkSpotEndpoint + "/0/public/Depth?count=1&pair=" + symbolS
	if kk.isXStocksSymbol(symbol) {
		url += "&asset_class=tokenized_asset"
	}
	_, resp, err := ihttp.Get(url, kkApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(kk.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Err    []string `json:"error"`
		Result map[string]struct {
			Asks [][]interface{} `json:"asks"`
			Bids [][]interface{} `json:"bids"`
		} `json:"result"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return BestBidAsk{}, errors.New(kk.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Err) != 0 {
		return BestBidAsk{}, errors.New(kk.Name() + " resp err: " + recv.Err[0])
	}
	for _, v := range recv.Result {
		if len(v.Bids) > 0 && len(v.Asks) > 0 {
			bidPrice, _ := decimal.NewFromString(v.Bids[0][0].(string))
			bidQty, _ := decimal.NewFromString(v.Bids[0][1].(string))
			askPrice, _ := decimal.NewFromString(v.Asks[0][0].(string))
			askQty, _ := decimal.NewFromString(v.Asks[0][1].(string))
			return BestBidAsk{
				Symbol:   symbol,
				BidPrice: bidPrice,
				BidQty:   bidQty,
				AskPrice: askPrice,
				AskQty:   askQty,
			}, nil
		}
	}
	return BestBidAsk{}, errors.New(kk.Name() + " resp empty!")
}
func (kk *Kraken) SpotPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	symbolS := kk.getSpotSymbol(symbol)
	path := "/0/private/AddOrder"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("ordertype", kk.fromStdOrderType(orderType))
	values.Set("type", kk.fromStdSide(side))
	values.Set("pair", symbolS)
	values.Set("price", price.String())
	values.Set("volume", qty.String())
	values.Set("stptype", "cancel-both") // 自成交保护: 自成交时买卖两单都撤(调用方不需要传)
	if clientId != "" {
		if len(clientId) > 18 {
			return "", errors.New(kk.Name() + " cltId too long! must le 18")
		}
		values.Set("cl_ord_id", clientId)
	}
	if timeInForce != "" {
		values.Set("timeinforce", timeInForce) // GTC, IOC, FOK
	}
	if orderType == "LIMIT" && postOnly {
		values.Set("oflags", "post")
	}
	if kk.isXStocksSymbol(symbol) {
		values.Set("asset_class", "tokenized_asset")
	}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return "", errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			OrderIds []string `json:"txid,omitempty"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return "", errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return "", errors.New("spot order fail! " + recv.Error[0])
	}

	if len(recv.Result.OrderIds) == 0 {
		return "", errors.New(kk.Name() + " spot order fail!")
	}

	return recv.Result.OrderIds[0], nil
}
func (kk *Kraken) SpotCancelOrder(symbol string, orderId, cltId string) error {
	path := "/0/private/CancelOrder"
	link := kkSpotEndpoint + path
	values := url.Values{}
	if orderId == "" && cltId != "" {
		values.Set("cl_ord_id", cltId)
	} else if orderId != "" {
		values.Set("txid", orderId)
	}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			Count int64 `json:"count,omitempty"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return errors.New(kk.Name() + " spot cancel order fail! " + recv.Error[0])
	}
	return nil
}

// SpotAmendOrder 修改订单: price/qty为零表示不修改; post_only不传时默认false, 需要保持只做Maker时必须显式传true
func (kk *Kraken) SpotAmendOrder(symbol, orderId, cltId string, price, qty decimal.Decimal,
	postOnly bool) (string, error) {
	if orderId == "" && cltId == "" {
		return "", errors.New(kk.Name() + " orderId or cltId empty!")
	}
	if price.IsZero() && qty.IsZero() {
		return "", errors.New(kk.Name() + " price and qty both empty!")
	}
	nonce := kk.nextNonce()
	req := struct {
		Nonce      string `json:"nonce"`
		TxId       string `json:"txid,omitempty"`
		CltId      string `json:"cl_ord_id,omitempty"`
		OrderQty   string `json:"order_qty,omitempty"`
		LimitPrice string `json:"limit_price,omitempty"`
		PostOnly   bool   `json:"post_only,omitempty"`
	}{Nonce: nonce, PostOnly: postOnly}
	if orderId != "" {
		req.TxId = orderId
	} else {
		req.CltId = cltId
	}
	if !qty.IsZero() {
		req.OrderQty = qty.String()
	}
	if !price.IsZero() {
		req.LimitPrice = price.String()
	}
	payload, _ := json.Marshal(&req)
	path := "/0/private/AmendOrder"
	link := kkSpotEndpoint + path
	headers := kk.buildJsonHeaders(path, nonce, payload)
	_, resp, err := ihttp.Post(link, payload, kkApiDeadline, headers)
	if err != nil {
		return "", errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			AmendId string `json:"amend_id,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return "", errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return "", errors.New(kk.Name() + " amend order fail! " + recv.Error[0])
	}
	if recv.Result.AmendId == "" {
		return "", errors.New(kk.Name() + " amend order fail!")
	}
	return recv.Result.AmendId, nil
}

func (kk *Kraken) SpotPlaceOrderMultipleSupported() bool { return true }

// SpotPlaceOrderMultiple 批量下单: 文档要求一次2~15单且只能同一个交易对
// 只有1单时用单笔接口, 超过15单自动分批(不足2单的那批也用单笔接口)
// 整批校验不过会被全部拒绝; 撮合阶段被拒的只拒单笔, 所以只要有成功的就算成功
func (kk *Kraken) SpotPlaceOrderMultiple(orders []SpotPostOrder) ([]PostOrderResult, error) {
	if len(orders) == 0 {
		return nil, errors.New(kk.Name() + " orders empty!")
	}
	symbol := orders[0].Symbol
	for i := range orders { // 一批只能一个交易对
		if orders[i].Symbol != symbol {
			return nil, errors.New(kk.Name() + " batch orders must be same symbol!")
		}
		if len(orders[i].ClientId) > 18 {
			return nil, errors.New(kk.Name() + " cltId too long! must le 18")
		}
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return nil, errors.New(kk.Name() + " not support order type! " + orders[i].Type)
		}
	}
	var res []PostOrderResult
	for i := 0; i < len(orders); i += 15 {
		end := i + 15
		if end > len(orders) {
			end = len(orders)
		}
		if end-i == 1 { // 交易所不接受单笔批量, 换成单笔下单
			orderId, err := kk.SpotPlaceOrder(orders[i].Symbol, orders[i].ClientId, orders[i].Price,
				orders[i].Amt, orders[i].Qty, orders[i].Side, orders[i].TimeInForce,
				orders[i].Type, orders[i].PostOnly)
			if err != nil {
				return res, err
			}
			res = append(res, PostOrderResult{ClientId: orders[i].ClientId, OrderId: orderId})
			continue
		}
		batchRes, err := kk.spotPlaceOrderBatch(orders[i:end])
		res = append(res, batchRes...)
		if err != nil {
			return res, err
		}
	}
	return res, nil
}
func (kk *Kraken) spotPlaceOrderBatch(orders []SpotPostOrder) ([]PostOrderResult, error) {
	symbol := orders[0].Symbol
	symbolS := kk.getSpotSymbol(symbol)
	if symbolS == "" {
		return nil, errors.New(kk.Name() + " unknown symbol! " + symbol)
	}
	type BatchOrder struct {
		Type        string `json:"type"`
		OrdType     string `json:"ordertype"`
		Price       string `json:"price,omitempty"`
		Volume      string `json:"volume"`
		TimeInForce string `json:"timeinforce,omitempty"`
		ClientId    string `json:"cl_ord_id,omitempty"`
		OFlags      string `json:"oflags,omitempty"`
		StpType     string `json:"stptype"` // 自成交保护: 自成交时买卖两单都撤
	}
	orderL := make([]BatchOrder, 0, len(orders))
	for i := range orders {
		po := BatchOrder{
			Type:        kk.fromStdSide(orders[i].Side),
			OrdType:     kk.fromStdOrderType(orders[i].Type),
			Volume:      orders[i].Qty.String(),
			TimeInForce: orders[i].TimeInForce,
			ClientId:    orders[i].ClientId,
			StpType:     "cancel-both",
		}
		if orders[i].Type == "LIMIT" {
			po.Price = orders[i].Price.String()
			if orders[i].PostOnly {
				po.OFlags = "post"
			}
		}
		orderL = append(orderL, po)
	}
	nonce := kk.nextNonce()
	req := struct {
		Nonce      string       `json:"nonce"`
		Orders     []BatchOrder `json:"orders"`
		Pair       string       `json:"pair"`
		AssetClass string       `json:"asset_class,omitempty"`
	}{Nonce: nonce, Orders: orderL, Pair: symbolS}
	if kk.isXStocksSymbol(symbol) {
		req.AssetClass = "tokenized_asset"
	}
	payload, _ := json.Marshal(&req)
	path := "/0/private/AddOrderBatch"
	link := kkSpotEndpoint + path
	headers := kk.buildJsonHeaders(path, nonce, payload)
	_, resp, err := ihttp.Post(link, payload, kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			Orders []struct {
				Txid  string `json:"txid,omitempty"`
				Error string `json:"error,omitempty"` // 撮合阶段被拒的单笔
			} `json:"orders,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Error) > 0 {
		return nil, errors.New(kk.Name() + " batch order fail! " + ret.Error[0])
	}
	var res []PostOrderResult
	failMsg := ""
	for _, v := range ret.Result.Orders {
		if len(v.Txid) > 0 {
			res = append(res, PostOrderResult{OrderId: v.Txid})
		} else if len(failMsg) == 0 {
			failMsg = v.Error
		}
	}
	if len(res) == 0 {
		return nil, errors.New(kk.Name() + " batch order all fail! " + failMsg)
	}
	return res, nil
}

func (kk *Kraken) SpotCancelOrderMultipleSupported() bool { return true }

// SpotCancelOrderMultiple 批量撤单: txid和cl_ord_id都可以传, 只要有成功的就算成功
func (kk *Kraken) SpotCancelOrderMultiple(symbol string, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(kk.Name() + " orderIds or cltIds empty!")
	}
	if len(orderIds)+len(cltIds) == 1 { // 只有1单时用单笔接口
		if len(orderIds) == 1 {
			return kk.SpotCancelOrder(symbol, orderIds[0], "")
		}
		return kk.SpotCancelOrder(symbol, "", cltIds[0])
	}
	nonce := kk.nextNonce()
	req := struct {
		Nonce  string   `json:"nonce"`
		Orders []string `json:"orders,omitempty"`
		CltIds []string `json:"cl_ord_ids,omitempty"`
	}{Nonce: nonce, Orders: orderIds, CltIds: cltIds}
	payload, _ := json.Marshal(&req)
	path := "/0/private/CancelOrderBatch"
	link := kkSpotEndpoint + path
	headers := kk.buildJsonHeaders(path, nonce, payload)
	_, resp, err := ihttp.Post(link, payload, kkApiDeadline, headers)
	if err != nil {
		return errors.New(kk.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			Count int64 `json:"count,omitempty"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Error) > 0 {
		return errors.New(kk.Name() + " batch cancel fail! " + ret.Error[0])
	}
	if ret.Result.Count == 0 {
		return errors.New(kk.Name() + " batch cancel all fail! " + string(resp))
	}
	return nil
}
func (kk *Kraken) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	path := "/0/private/QueryOrders"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("consolidate_taker", "true")
	values.Set("txid", orderId)
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	type orderDesc struct {
		Symbol    string          `json:"pair"`
		OrderType string          `json:"ordertype"` // market/limit
		Side      string          `json:"type"`      // buy/sell
		Price     decimal.Decimal `json:"price"`
	}
	type tradeInfo struct {
		ClientId     string          `json:"cl_ord_id"`
		Status       string          `json:"status"` // pending,open,closed,canceled,expired
		Desc         orderDesc       `json:"descr"`
		Qty          decimal.Decimal `json:"vol"`
		ExecutedQty  decimal.Decimal `json:"vol_exec"`
		CummQuoteQty decimal.Decimal `json:"cost"`
		AvgPrice     decimal.Decimal `json:"price"`
		Fee          decimal.Decimal `json:"fee"`
		CTime        float64         `json:"opentm"`
		DoneTime     float64         `json:"closetm"`
	}
	ret := struct {
		Error  []string              `json:"error"`
		Result map[string]*tradeInfo `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Error) > 0 {
		return nil, errors.New(kk.Name() + " spot get order fail! " + ret.Error[0])
	}

	ord := ret.Result[orderId]
	if ord == nil {
		return nil, errors.New(kk.Name() + " spot get order fail! orderId:" + orderId)
	}
	feeAsset := SpotSymbolQuote(kk.Name(), symbol) // Kraken手续费扣的全是报价币
	return &SpotOrder{
		Symbol:    symbol,
		OrderId:   orderId,
		ClientId:  ord.ClientId,
		Price:     ord.Desc.Price,
		Qty:       ord.Qty,
		FilledQty: ord.ExecutedQty,
		FilledAmt: ord.CummQuoteQty,
		AvgPrice:  ord.AvgPrice,
		Status:    kk.toStdOrderStatus(ord.Status),
		Type:      kk.toStdOrderType(ord.Desc.OrderType),
		Side:      kk.toStdSide(ord.Desc.Side),
		FeeQty:    ord.Fee.Neg(),
		FeeAsset:  feeAsset,
		CTime:     int64(ord.CTime * 1000),
		UTime:     int64(ord.DoneTime * 1000),
	}, nil
}

// spotGetOrderList 查订单列表: 未成交(OpenOrders)和已成交(ClosedOrders)的订单结构一样
// 交易所不支持按交易对过滤, 取回来后自己筛; 每页最多50条
func (kk *Kraken) spotGetOrderList(path, api string, values url.Values,
	symbol string) ([]*SpotOrder, error) {
	link := kkSpotEndpoint + path
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	type orderDesc struct {
		Symbol    string          `json:"pair"` // 交易所pair名, 如XBTUSD
		OrderType string          `json:"ordertype"`
		Side      string          `json:"type"`
		Price     decimal.Decimal `json:"price"`
	}
	type orderInfo struct {
		ClientId     string          `json:"cl_ord_id"`
		Status       string          `json:"status"`
		Desc         orderDesc       `json:"descr"`
		Qty          decimal.Decimal `json:"vol"`
		ExecutedQty  decimal.Decimal `json:"vol_exec"`
		CummQuoteQty decimal.Decimal `json:"cost"`
		AvgPrice     decimal.Decimal `json:"price"`
		Fee          decimal.Decimal `json:"fee"`
		CTime        float64         `json:"opentm"`
		DoneTime     float64         `json:"closetm"`
	}
	ret := struct {
		Error  []string `json:"error"`
		Result struct {
			Open   map[string]*orderInfo `json:"open"`   // OpenOrders
			Closed map[string]*orderInfo `json:"closed"` // ClosedOrders
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Error) > 0 {
		return nil, errors.New(kk.Name() + " " + api + " fail! " + ret.Error[0])
	}
	dl := make([]*SpotOrder, 0, len(ret.Result.Open)+len(ret.Result.Closed))
	for _, orderM := range []map[string]*orderInfo{ret.Result.Open, ret.Result.Closed} {
		for orderId, ord := range orderM {
			stdSymbol := kk.getSpotStdSymbol(ord.Desc.Symbol)
			if len(stdSymbol) == 0 { // 规则没加载时兜底
				stdSymbol = strings.ToUpper(ord.Desc.Symbol)
			}
			if symbol != "" && stdSymbol != symbol {
				continue
			}
			dl = append(dl, &SpotOrder{
				Symbol:    stdSymbol,
				OrderId:   orderId,
				ClientId:  ord.ClientId,
				Price:     ord.Desc.Price,
				Qty:       ord.Qty,
				FilledQty: ord.ExecutedQty,
				FilledAmt: ord.CummQuoteQty,
				AvgPrice:  ord.AvgPrice,
				Status:    kk.toStdOrderStatus(ord.Status),
				Type:      kk.toStdOrderType(ord.Desc.OrderType),
				Side:      kk.toStdSide(ord.Desc.Side),
				FeeQty:    ord.Fee.Neg(),
				FeeAsset:  SpotSymbolQuote(kk.Name(), stdSymbol), // Kraken手续费扣的全是报价币
				CTime:     int64(ord.CTime * 1000),
				UTime:     int64(ord.DoneTime * 1000),
			})
		}
	}
	return dl, nil
}
func (kk *Kraken) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	values := url.Values{}
	values.Set("trades", "false") // 不需要订单里的成交明细
	return kk.spotGetOrderList("/0/private/OpenOrders", "get open orders", values, symbol)
}

// SpotGetFilledOrders 查已结束订单(全成/已撤/过期), 每页最多50条
func (kk *Kraken) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	values := url.Values{}
	values.Set("trades", "false") // 不需要订单里的成交明细
	return kk.spotGetOrderList("/0/private/ClosedOrders", "get filled orders", values, symbol)
}
