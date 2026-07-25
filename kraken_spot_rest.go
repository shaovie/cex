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
