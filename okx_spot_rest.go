package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

func (ok *Okx) SpotSupported() bool {
	return true
}
func (ok *Okx) SpotServerTime() (int64, error) {
	url := okUniEndpoint + "/api/v5/public/time"
	_, resp, err := ihttp.Get(url, okApiDeadline, nil)
	if err != nil {
		return 0, errors.New(ok.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			Time string `json:"ts,omitempty"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(ok.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "0" {
		return 0, errors.New("resp error: " + recv.Msg)
	}
	if len(recv.Data) == 0 {
		return 0, errors.New("resp empty")
	}
	ts, _ := strconv.ParseInt(recv.Data[0].Time, 10, 64)
	return ts, nil
}
func (ok *Okx) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := okUniEndpoint + "/api/v5/public/instruments?instType=SPOT"
	retCode, resp, err := ihttp.Get(url, okApiDeadline, nil)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}

	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			InstId      string `json:"instId"`
			Base        string `json:"baseCcy"`
			Quote       string `json:"quoteCcy"`
			State       string `json:"state"`
			PriceTickSz string `json:"tickSz"`
			QtyStep     string `json:"lotSz"`
			MinOrderQty string `json:"minSz"`
			MaxOrderQty string `json:"maxLmtSz"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		return nil, errors.New("resp error: " + ret.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tokxSpotSymbolMap := make(map[string]string)
	for _, pair := range ret.Data {
		if pair.State != "live" { // open for trading
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:   pair.Base + pair.Quote,
			Base:     pair.Base,
			Quote:    pair.Quote,
			Status:   "online",
			MaxPrice: decimal.NewFromFloat(999999999.99),
			Time:     now,
		}
		ep.PriceTickSize, _ = decimal.NewFromString(pair.PriceTickSz)
		ep.QtyStep, _ = decimal.NewFromString(pair.QtyStep)
		ep.MinOrderQty, _ = decimal.NewFromString(pair.MinOrderQty)
		ep.MaxOrderQty, _ = decimal.NewFromString(pair.MaxOrderQty)
		ep.MinPrice = ep.PriceTickSize
		all[ep.Symbol] = ep
		tokxSpotSymbolMap[ep.Symbol] = pair.InstId
	}

	okxSpotSymbolMapMtx.Lock()
	okxSpotSymbolMap = tokxSpotSymbolMap
	okxSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (ok *Okx) SpotGetAll24hTicker() (map[string]Pub24hTicker, error) {
	path := "/api/v5/market/tickers"
	url := okUniEndpoint + path + "?instType=SPOT"
	retCode, resp, err := ihttp.Get(url, okApiDeadline, nil)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	tickers := Okx24hTickers{}
	err = easyjson.Unmarshal(resp, &tickers)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal error! " + err.Error())
	}
	if len(tickers.Data) == 0 {
		return nil, errors.New(ok.Name() + " resp empty")
	}
	allTk := make(map[string]Pub24hTicker, len(tickers.Data))
	for _, tk := range tickers.Data {
		firstDash := strings.Index(tk.Symbol, "-")
		if firstDash == -1 {
			continue
		}
		sym := tk.Symbol[:firstDash] + tk.Symbol[firstDash+1:]
		v := Pub24hTicker{
			Symbol:      sym,
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (ok *Okx) SpotPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	if timeInForce == "IOC" || timeInForce == "FOK" {
		orderType = timeInForce
	}
	if orderType == "MARKET" && side == "BUY" {
		qty = amt
	}
	ordType := ok.fromStdOrderType(orderType)
	if orderType == "LIMIT" && postOnly {
		ordType = "post_only"
	}
	symbolS := ok.getSpotSymbol(symbol)
	payload := `{"instId":"` + symbolS + `"` +
		`,"sz":"` + qty.String() + `"`
	if clientId != "" {
		payload += `,"clOrdId":"` + clientId + `"`
	}
	payload += "" +
		`,"px":"` + price.String() + `"` +
		`,"side":"` + ok.fromStdSide(side) + `"` +
		`,"ordType":"` + ordType + `"` + // LIMIT/MARKET/LIMIT_MAKER
		`,"tdMode":"cash"` +
		`}`
	path := "/api/v5/trade/order"
	headers := ok.buildHeaders("POST", path, payload)
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, []byte(payload), okApiDeadline, headers)
	if err != nil {
		return "", errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return "", errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode,omitempty"`
			SMsg    string `json:"sMsg,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		if len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return "", errors.New(ok.Name() + " fail! code=" + ret.Code + " msg=" + ret.Msg)
	}

	if len(ret.Data) == 0 {
		return "", errors.New(ok.Name() + " fail! response emtpy")
	}

	return ret.Data[0].OrderId, nil
}
func (ok *Okx) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	path := "/api/v5/account/balance"
	url := okUniEndpoint + path
	headers := ok.buildHeaders("GET", path, "")
	retCode, resp, err := ihttp.Get(url, okApiDeadline, headers)
	if err != nil {
		return nil, errors.New(ok.Name() + " spot get assets net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			SCode  string `json:"sCode,omitempty"`
			SMsg   string `json:"sMsg,omitempty"`
			Detail []struct {
				Symbol string          `json:"ccy"`
				Total  decimal.Decimal `json:"eq"`
				Free   decimal.Decimal `json:"cashBal"`
				Locked decimal.Decimal `json:"frozenBal"`
			} `json:"details,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		if len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return nil, errors.New(ok.Name() + " resp err! " + ret.Msg)
	}

	spotAssets := make(map[string]*SpotAsset)
	for _, dt := range ret.Data {
		for _, asset := range dt.Detail {
			as := &SpotAsset{
				Symbol: asset.Symbol,
				Avail:  asset.Free,
				Locked: asset.Locked,
				Total:  asset.Total,
			}
			spotAssets[as.Symbol] = as
		}
	}

	return spotAssets, nil
}
func (ok *Okx) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	symbolS := ok.getSpotSymbol(symbol)
	path := "/api/v5/trade/order?instId=" + symbolS + "&ordId=" + orderId
	if orderId == "" && cltId != "" {
		path = "/api/v5/trade/order?instId=" + symbolS + "&clOrdId=" + cltId
	}
	headers := ok.buildHeaders("GET", path, "")
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Get(url, okApiDeadline, headers)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			SCode       string          `json:"sCode,omitempty"`
			SMsg        string          `json:"sMsg,omitempty"`
			Symbol      string          `json:"instId"`
			OrderId     string          `json:"ordId"`
			ClientId    string          `json:"clOrdId"`
			AvgPrice    string          `json:"avgPx,omitempty"`
			Price       string          `json:"px,omitempty"`
			Qty         decimal.Decimal `json:"sz"`
			ExecutedQty string          `json:"accFillSz"`
			Status      string          `json:"state"`
			Type        string          `json:"ordType"`
			Side        string          `json:"side"`
			FeeCoin     string          `json:"feeCcy,omitempty"`
			FeeQty      string          `json:"fee,omitempty"`
			Time        string          `json:"cTime"`
			UTime       string          `json:"uTime,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" || len(ret.Data) == 0 {
		if ret.Code != "0" && len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return nil, errors.New(ok.Name() + " resp err! " + ret.Msg)
	}

	dt := ret.Data[0]
	ctime, _ := strconv.ParseInt(dt.Time, 10, 64)
	utime, _ := strconv.ParseInt(dt.UTime, 10, 64)
	avgPrice, _ := decimal.NewFromString(dt.AvgPrice)
	so := &SpotOrder{
		Symbol:   symbol,
		OrderId:  dt.OrderId,
		ClientId: dt.ClientId,
		Qty:      dt.Qty,
		Status:   ok.toStdOrderStatus(dt.Status),
		Type:     ok.toStdOrderType(dt.Type),
		Side:     ok.toStdSide(dt.Side),
		FeeAsset: dt.FeeCoin,
		CTime:    ctime,
		UTime:    utime,
	}
	so.Price, _ = decimal.NewFromString(dt.Price)
	so.FilledQty, _ = decimal.NewFromString(dt.ExecutedQty)
	so.FilledAmt = so.FilledQty.Mul(avgPrice)
	so.FeeQty, _ = decimal.NewFromString(dt.FeeQty)
	return so, nil
}
func (ok *Okx) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	symbolS := ok.getSpotSymbol(symbol)
	path := "/api/v5/trade/cancel-order"
	payload := `{"instId":"` + symbolS + `","ordId":"` + orderId + `"}`
	if orderId == "" && cltId != "" {
		payload = `{"instId":"` + symbolS + `","clOrdId":"` + cltId + `"}`
	}
	headers := ok.buildHeaders("POST", path, payload)
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, []byte(payload), okApiDeadline, headers)
	if err != nil {
		return errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" || len(ret.Data) == 0 {
		if ret.Code != "0" && len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return errors.New(ok.Name() + " resp error! " + ret.Msg)
	}

	dt := ret.Data[0]
	if dt.SCode != "0" {
		return errors.New(ok.Name() + " resp error! " + dt.SMsg)
	}
	return nil
}

// SpotPlaceOrderMultiple 批量下单, 一次最多20单
func (ok *Okx) SpotPlaceOrderMultiple(orders []SpotPostOrder) error {
	if len(orders) == 0 {
		return errors.New(ok.Name() + " orders empty!")
	}
	type BatchOrder struct {
		InstId  string `json:"instId"`
		TdMode  string `json:"tdMode"`
		ClOrdId string `json:"clOrdId,omitempty"`
		Sz      string `json:"sz"`
		Px      string `json:"px,omitempty"`
		Side    string `json:"side"`
		OrdType string `json:"ordType"`
	}
	orderL := make([]BatchOrder, 0, len(orders))
	for i := range orders {
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return errors.New(ok.Name() + " not support order type! " + orders[i].Type)
		}
		orderType := orders[i].Type
		if orders[i].TimeInForce == "IOC" || orders[i].TimeInForce == "FOK" {
			orderType = orders[i].TimeInForce
		}
		qty := orders[i].Qty
		if orders[i].Type == "MARKET" && orders[i].Side == "BUY" { // 市价买 sz为计价币金额
			qty = orders[i].Amt
		}
		po := BatchOrder{
			InstId:  ok.getSpotSymbol(orders[i].Symbol),
			TdMode:  "cash",
			ClOrdId: orders[i].ClientId,
			Sz:      qty.String(),
			Side:    ok.fromStdSide(orders[i].Side),
			OrdType: ok.fromStdOrderType(orderType),
		}
		if len(po.InstId) == 0 {
			return errors.New(ok.Name() + " unknown symbol! " + orders[i].Symbol)
		}
		if orders[i].Type == "LIMIT" {
			po.Px = orders[i].Price.String()
			if orders[i].PostOnly { // 只做Maker
				po.OrdType = "post_only"
			}
		}
		orderL = append(orderL, po)
	}
	payload, _ := json.Marshal(orderL)
	path := "/api/v5/trade/batch-orders"
	headers := ok.buildHeaders("POST", path, string(payload))
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, payload, okApiDeadline, headers)
	if err != nil {
		return errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		return errors.New(ok.Name() + " fail! code=" + ret.Code + " msg=" + ret.Msg)
	}
	success, failMsg := 0, ""
	for _, v := range ret.Data {
		if v.SCode == "0" {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.SCode + " " + v.SMsg
		}
	}
	if success == 0 {
		return errors.New(ok.Name() + " batch order all fail! " + failMsg)
	}
	return nil
}

// SpotCancelOrderMultiple 批量撤单, 一次最多20单
func (ok *Okx) SpotCancelOrderMultiple(symbol string /*BTCUSDT*/, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(ok.Name() + " orderIds or cltIds empty!")
	}
	instId := ok.getSpotSymbol(symbol)
	if len(instId) == 0 {
		return errors.New(ok.Name() + " unknown symbol! " + symbol)
	}
	type CancelOrder struct {
		InstId  string `json:"instId"`
		OrdId   string `json:"ordId,omitempty"`
		ClOrdId string `json:"clOrdId,omitempty"`
	}
	orderL := make([]CancelOrder, 0, len(orderIds)+len(cltIds))
	for _, v := range orderIds {
		orderL = append(orderL, CancelOrder{InstId: instId, OrdId: v})
	}
	for _, v := range cltIds {
		orderL = append(orderL, CancelOrder{InstId: instId, ClOrdId: v})
	}
	payload, _ := json.Marshal(orderL)
	path := "/api/v5/trade/cancel-batch-orders"
	headers := ok.buildHeaders("POST", path, string(payload))
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, payload, okApiDeadline, headers)
	if err != nil {
		return errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		return errors.New(ok.Name() + " fail! code=" + ret.Code + " msg=" + ret.Msg)
	}
	success, failMsg := 0, ""
	for _, v := range ret.Data {
		if v.SCode == "0" {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.SCode + " " + v.SMsg
		}
	}
	if success == 0 {
		return errors.New(ok.Name() + " batch cancel all fail! " + failMsg)
	}
	return nil
}
func (ok *Okx) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	symbolS := ok.getSpotSymbol(symbol)
	path := "/api/v5/trade/orders-pending?instType=SPOT&instId=" + symbolS
	headers := ok.buildHeaders("GET", path, "")
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Get(url, okApiDeadline, headers)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			SCode       string          `json:"sCode,omitempty"`
			SMsg        string          `json:"sMsg,omitempty"`
			Symbol      string          `json:"instId"`
			OrderId     string          `json:"ordId"`
			ClientId    string          `json:"clOrdId"`
			AvgPrice    string          `json:"avgPx,omitempty"`
			Price       string          `json:"px"`
			Qty         decimal.Decimal `json:"sz"`
			ExecutedQty string          `json:"accFillSz"`
			Status      string          `json:"state"`
			Type        string          `json:"ordType"`
			Side        string          `json:"side"`
			FeeCoin     string          `json:"feeCcy,omitempty"`
			FeeQty      string          `json:"fee,omitempty"`
			Time        string          `json:"cTime"`
			UTime       string          `json:"uTime,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" || len(ret.Data) == 0 {
		if ret.Code != "0" && len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return nil, errors.New(ok.Name() + " resp fail! " + ret.Msg)
	}

	orders := make([]*SpotOrder, 0, len(ret.Data))
	for _, order := range ret.Data {
		ctime, _ := strconv.ParseInt(order.Time, 10, 64)
		utime, _ := strconv.ParseInt(order.UTime, 10, 64)
		avgPrice, _ := decimal.NewFromString(order.AvgPrice)
		so := &SpotOrder{
			Symbol:   strings.ReplaceAll(order.Symbol, "-", ""),
			OrderId:  order.OrderId,
			ClientId: order.ClientId,
			Qty:      order.Qty,
			Status:   ok.toStdOrderStatus(order.Status),
			Type:     ok.toStdOrderType(order.Type),
			Side:     ok.toStdSide(order.Side),
			FeeAsset: order.FeeCoin,
			CTime:    ctime,
			UTime:    utime,
		}
		so.Price, _ = decimal.NewFromString(order.Price)
		so.FilledQty, _ = decimal.NewFromString(order.ExecutedQty)
		so.FilledAmt = so.FilledQty.Mul(avgPrice)
		so.FeeQty, _ = decimal.NewFromString(order.FeeQty)
		orders = append(orders, so)
	}
	return orders, nil
}
