package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

func (bo *Bigone) SpotSupported() bool {
	return true
}
func (bo *Bigone) SpotServerTime() (int64, error) {
	url := boSpotEndpoint + "/ping"
	_, resp, err := ihttp.Get(url, boApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bo.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Timestamp int64 `json:"Timestamp,omitempty"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return 0, errors.New(bo.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return 0, errors.New(recv.Msg)
	}
	return recv.Data.Timestamp / 1000000, nil
}
func (bo *Bigone) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := boSpotEndpoint + "/asset_pairs"
	_, resp, err := ihttp.Get(url, boApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data []struct {
			Symbol      string          `json:"name"`
			QuoteScale  int             `json:"quote_scale"`
			BaseScale   int             `json:"base_scale"`
			MinNotional decimal.Decimal `json:"min_quote_value"`
		} `json:"data,omitempty"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(recv.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tboSpotSymbolMap := make(map[string]string)
	for _, pair := range recv.Data {
		base, quote, ok := strings.Cut(pair.Symbol, "-")
		if !ok {
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        base + quote,
			Base:          base,
			Quote:         quote,
			Time:          now,
			MinNotional:   pair.MinNotional,
			QtyStep:       PowOneTenth(pair.BaseScale),
			PriceTickSize: PowOneTenth(pair.QuoteScale),
			MaxPrice:      decimal.NewFromFloat(999999999),
			MaxOrderQty:   decimal.NewFromFloat(999999999),
		}
		ep.MinPrice = ep.PriceTickSize
		ep.MinOrderQty = ep.QtyStep
		all[ep.Symbol] = ep
		tboSpotSymbolMap[ep.Symbol] = pair.Symbol
	}
	boSpotSymbolMapMtx.Lock()
	boSpotSymbolMap = tboSpotSymbolMap
	boSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (bo *Bigone) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	url := boSpotEndpoint + "/viewer/accounts"
	jwt := "Bearer " + bo.jwt()
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	type boAsset struct {
		Symbol  string          `json:"asset_symbol"`
		Balance decimal.Decimal `json:"balance"`
		Locked  decimal.Decimal `json:"locked_balance"`
	}
	type Resp struct {
		Code int       `json:"code,omitempty"`
		Msg  string    `json:"message,omitempty"`
		Data []boAsset `json:"data,omitempty"`
	}
	assetL := Resp{}
	err = json.Unmarshal(resp, &assetL)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal error! " + err.Error())
	}
	if assetL.Code != 0 {
		return nil, errors.New(bo.Name() + " get asset fail! " + assetL.Msg)
	}

	assetsMap := make(map[string]*SpotAsset)
	for _, v := range assetL.Data {
		if v.Balance.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &SpotAsset{
			Symbol: v.Symbol,
			Avail:  v.Balance.Sub(v.Locked),
			Locked: v.Locked,
			Total:  v.Balance,
		}
	}
	return assetsMap, nil
}
func (bo *Bigone) SpotPlaceOrderMultiple(orders []SpotPostOrder) error {
	type PostOrder struct {
		Symbol string `json:"asset_pair_name"`
		Side string `json:"side"`
		Type string `json:"typ"`
		ClientId string `json:"client_order_id,omitempty"`
		PostOnly bool `json:"post_only,omitempty"`
		Price string `json:"price,omitempty"`
		Qty decimal.Decimal `json:"amount"`
	}
	poL := make([]PostOrder, 0, len(orders))
	for i := range orders {
		symbolS := boSpotSymbolMap[orders[i].Symbol]
		po := PostOrder {
			PostOnly: orders[i].PostOnly,
			Symbol: symbolS,
			Side: bo.fromStdSide(orders[i].Side),
			Qty: orders[i].Qty,
			Type: bo.fromStdOrderType(orders[i].Type),
			ClientId: orders[i].ClientId,
		}
		if orders[i].Type == "LIMIT" {
			po.Price = orders[i].Price.String()
		}
		poL = append(poL, po)
	}
	url := boSpotEndpoint + "/viewer/orders/multi"
	jwt := "Bearer " + bo.jwt()
	payload, _ := json.Marshal(poL)
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	_, resp, err := ihttp.Post(url, payload, boApiDeadline, header)
	if err != nil {
		return errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		/*
		Data []struct {
			OrderId int64  `json:"id,omitempty"`
			ClientId string `json:"client_order_id"`
			Status  string `json:"state,omitempty"`
		} `json:"data"`
		*/
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bo.Name() + " unmarshal fail! " + err.Error() + ", "+ string(resp))
	}
	if ret.Code != 0 {
		return errors.New(bo.Name() + " fail! msg=" + ret.Msg)
	}

	return nil
}
func (bo *Bigone) SpotPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {

	symbolS := boSpotSymbolMap[symbol]
	url := boSpotEndpoint + "/viewer/orders"
	jwt := "Bearer " + bo.jwt()
	payload := `{"asset_pair_name":"` + symbolS + `"`
	if clientId != "" {
		payload += `,"client_order_id":"` + clientId + `"` // len(clientId) LessOrEqual than 36
	}
	if orderType == "LIMIT" {
		payload += `,"price":"` + price.String() + `"`
		if postOnly {
			payload += `,"post_only":true`
		}
	}
	payload += "" +
		`,"amount":"` + qty.String() + `"` +
		`,"side":"` + bo.fromStdSide(side) + `"` +
		`,"type":"` + bo.fromStdOrderType(orderType) + `"` + // LIMIT/MARKET
		`}`
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	_, resp, err := ihttp.Post(url, []byte(payload), boApiDeadline, header)
	if err != nil {
		return "", errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			OrderId int64  `json:"id,omitempty"`
			Status  string `json:"state,omitempty"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(bo.Name() + " unmarshal fail! " + err.Error() + ", "+ string(resp))
	}
	if ret.Code != 0 {
		return "", errors.New(bo.Name() + " fail! msg=" + ret.Msg)
	}

	if ret.Data.OrderId == 0 {
		return "", errors.New(bo.Name() + " order Id is empty!")
	}
	return strconv.FormatInt(ret.Data.OrderId, 10), nil
}
func (bo *Bigone) SpotCancelOrder(symbol, orderId, cltId string) error {
	url := boSpotEndpoint + "/viewer/order/cancel"
	payload := `{"order_id":` + orderId + `}`
	if orderId == "" && cltId != "" {
		payload = `{"client_order_id":"` + cltId + `"}`
	}

	jwt := "Bearer " + bo.jwt()
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	respCode, resp, err := ihttp.Post(url, []byte(payload), boApiDeadline, header)
	if err != nil {
		return errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Status string `json:"state,omitempty"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bo.Name() + " cancel order unmarshal fail! " + err.Error()+strconv.FormatInt(int64(respCode), 10))
	}
	if ret.Code != 0 {
		return errors.New(bo.Name() + " cancel order fail! " + ret.Msg)
	}
	st := bo.toStdOrderStatus(ret.Data.Status)
	if st != "CANCELED" {
		return errors.New(bo.Name() + " cancel failed! status now: " + ret.Data.Status)
	}
	return nil
}
func (bo *Bigone) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	url := boSpotEndpoint + "/viewer/order?order_id=" + orderId
	if orderId == "" && cltId != "" {
		url = boSpotEndpoint + "/viewer/order?client_order_id=" + cltId
	}
	jwt := "Bearer " + bo.jwt()
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Symbol      string          `json:"asset_pair_name,omitempty"`
			OrderId     int64           `json:"id,omitempty"`
			Price       decimal.Decimal `json:"price"`
			Qty         decimal.Decimal `json:"amount"`
			ExecutedQty decimal.Decimal `json:"filled_amount"`
			AvgPrice    decimal.Decimal `json:"avg_deal_price"`
			Status      string          `json:"state,omitempty"`
			Type        string          `json:"type,omitempty"`
			Side        string          `json:"side,omitempty"`
			ClientId    string          `json:"client_order_id,omitempty"`
			Time        string          `json:"created_at,omitempty"`
			UTime       string          `json:"updated_at,omitempty"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " get order fail! " + ret.Msg)
	}

	ctime, _ := time.Parse(time.RFC3339, ret.Data.Time)
	utime, _ := time.Parse(time.RFC3339, ret.Data.UTime)
	status := bo.toStdOrderStatus(ret.Data.Status)
	if status == "NEW" && ret.Data.ExecutedQty.IsPositive() {
		status = "PARTIALLY_FILLED"
	}
	return &SpotOrder{
		Symbol:    symbol,
		OrderId:   strconv.FormatInt(ret.Data.OrderId, 10),
		ClientId:  ret.Data.ClientId,
		Price:     ret.Data.Price,
		Qty:       ret.Data.Qty,
		FilledQty: ret.Data.ExecutedQty,
		FilledAmt: ret.Data.ExecutedQty.Mul(ret.Data.AvgPrice),
		Status:    status,
		Type:      bo.toStdOrderType(ret.Data.Type),
		Side:      bo.toStdSide(ret.Data.Side),
		CTime:     ctime.UnixMilli(),
		UTime:     utime.UnixMilli(),
	}, nil
}
func (bo *Bigone) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	symbolS := boSpotSymbolMap[symbol]
	url := boSpotEndpoint + "/viewer/orders?limit=200&state=PENDING&asset_pair_name=" + symbolS
	jwt := "Bearer " + bo.jwt()
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		L    []struct {
			Symbol      string          `json:"asset_pair_name,omitempty"`
			Id          int64           `json:"id,omitempty"`
			ClientId    string          `json:"client_order_id,omitempty"`
			Price       decimal.Decimal `json:"price"`
			Qty         decimal.Decimal `json:"amount"`
			ExecutedQty decimal.Decimal `json:"filled_amount"`
			AvgPrice    decimal.Decimal `json:"avg_deal_price"`
			Status      string          `json:"state,omitempty"`
			Type        string          `json:"type,omitempty"`
			Side        string          `json:"side,omitempty"`
			Time        string          `json:"created_at,omitempty"`
			UTime       string          `json:"updated_at,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " openorder fail! " + ret.Msg)
	}

	dl := make([]*SpotOrder, 0, len(ret.L))
	for _, v := range ret.L {
		ctime, _ := time.Parse(time.RFC3339, v.Time)
		utime, _ := time.Parse(time.RFC3339, v.UTime)
		so := SpotOrder{
			Symbol:    symbol,
			OrderId:   strconv.FormatInt(v.Id, 10),
			ClientId:  v.ClientId,
			Price:     v.Price,
			Qty:       v.Qty,
			FilledQty: v.ExecutedQty,
			FilledAmt: v.ExecutedQty.Mul(v.AvgPrice),
			Status:    bo.toStdOrderStatus(v.Status),
			Type:      bo.toStdOrderType(v.Type),
			Side:      bo.toStdSide(v.Side),
			CTime:     ctime.UnixMilli(),
			UTime:     utime.UnixMilli(),
		}
		dl = append(dl, &so)
	}
	return dl, nil
}
func (bo *Bigone) SpotGetTradeFee(symbol string) (SpotTradeFee, error) {
	symbolS := boSpotSymbolMap[symbol]
	var f SpotTradeFee
	url := boSpotEndpoint + "/viewer/trading_fees?asset_pair_names=" + symbolS
	jwt := "Bearer " + bo.jwt()
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return f, errors.New(bo.Name() + " error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		L    []struct {
			Symbol string          `json:"asset_pair_name,omitempty"`
			Maker  decimal.Decimal `json:"maker_fee_rate"`
			Taker  decimal.Decimal `json:"taker_fee_rate"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return f, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return f, errors.New(bo.Name() + " get trade fee fail! " + ret.Msg)
	}

	for i := range ret.L {
		if ret.L[i].Symbol == symbolS {
			return SpotTradeFee{
				Maker:    ret.L[i].Maker,
				Taker:    ret.L[i].Taker,
				Discount: decimal.NewFromFloat(1.0),
			}, nil
		}
	}
	return f, errors.New("not found")
}
