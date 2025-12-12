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
		} `json:"data,omitempty"`
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
		sb := strings.Split(strings.ToUpper(pair.Symbol), "-")
		if len(sb) != 2 {
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        sb[0] + sb[1],
			Base:          sb[0],
			Quote:         sb[1],
			Time:          now,
			QtyStep:       PowOneTenth(pair.BaseScale),
			PriceTickSize: PowOneTenth(pair.QuoteScale),
			MinPrice:      pair.MinNotional,
			MaxPrice:      decimal.NewFromFloat(999999999),
			MaxOrderQty:   decimal.NewFromFloat(999999999),
		}
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
		v.Symbol = strings.ToUpper(v.Symbol)
		assetsMap[v.Symbol] = &SpotAsset{
			Symbol: v.Symbol,
			Avail:  v.Balance.Sub(v.Locked),
			Locked: v.Locked,
			Total:  v.Balance,
		}
	}
	return assetsMap, nil
}
func (bo *Bigone) SpotPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, timeInForce, orderType string) (string, error) {

	symbolS := boSpotSymbolMap[symbol]
	url := boSpotEndpoint + "/viewer/orders"
	jwt := "Bearer " + bo.jwt()
	payload := `{"asset_pair_name":"` + symbolS + `"`
	if clientId != "" {
		payload += `,"client_order_id":"` + clientId + `"` // len(clientId) LessOrEqual than 36
	}
	if orderType == "LIMIT" {
		payload += `,"price":"` + price.String() + `"`
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
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(bo.Name() + " unmarshal fail! " + err.Error())
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
	_, resp, err := ihttp.Post(url, []byte(payload), boApiDeadline, header)
	if err != nil {
		return errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Status string `json:"state,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bo.Name() + " cancel order unmarshal fail! " + err.Error())
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
			Price       decimal.Decimal `json:"price,omitempty"`
			Qty         decimal.Decimal `json:"amount,omitempty"`
			ExecutedQty decimal.Decimal `json:"filled_amount,omitempty"`
			AvgPrice    decimal.Decimal `json:"avg_deal_price,omitempty"`
			Status      string          `json:"state,omitempty"`
			Type        string          `json:"type,omitempty"`
			Side        string          `json:"side,omitempty"`
			ClientId    string          `json:"client_order_id,omitempty"`
			Time        string          `json:"created_at,omitempty"`
			UTime       string          `json:"updated_at,omitempty"`
		} `json:"data,omitempty"`
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
	return &SpotOrder{
		Symbol:    symbol,
		OrderId:   strconv.FormatInt(ret.Data.OrderId, 10),
		ClientId:  ret.Data.ClientId,
		Price:     ret.Data.Price,
		Qty:       ret.Data.Qty,
		FilledQty: ret.Data.ExecutedQty,
		FilledAmt: ret.Data.ExecutedQty.Mul(ret.Data.AvgPrice),
		Status:    bo.toStdOrderStatus(ret.Data.Status),
		Type:      bo.toStdOrderType(ret.Data.Type),
		Side:      bo.toStdSide(ret.Data.Side),
		CTime:     ctime.UnixMilli(),
		UTime:     utime.UnixMilli(),
	}, nil
}
