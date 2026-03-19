package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bn *Binance) MarginSupported() bool {
	return true
}
func (bn *Binance) MarginGetCrossAccountInfo() (*MarginCrossAccountInfo, error) {
	url := bnMarginEndpoint + "/sapi/v1/margin/account?" + bn.httpQuerySign("")
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		UserAssets []struct {
			Symbol   string          `json:"asset"`
			Borrowed decimal.Decimal `json:"borrowed"`
			Free     decimal.Decimal `json:"free"`
			Interest decimal.Decimal `json:"interest"`
			Locked   decimal.Decimal `json:"locked"`
			NetAsset decimal.Decimal `json:"netAsset"`
		} `json:"userAssets"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 || len(recv.Msg) != 0 {
		return nil, errors.New(bn.Name() + " api err! " + recv.Msg)
	}

	mac := MarginCrossAccountInfo{}
	mac.UserAssets = make([]*MarginCrossAccountUserAsset, 0, 4)
	for _, v := range recv.UserAssets {
		if v.Borrowed.IsZero() && v.Free.IsZero() &&
			v.Locked.IsZero() && v.Interest.IsZero() &&
			v.NetAsset.IsZero() {
			continue
		}
		as := MarginCrossAccountUserAsset{
			Symbol:   v.Symbol,
			Borrowed: v.Borrowed,
			Free:     v.Free,
			Interest: v.Interest,
			Locked:   v.Locked,
			NetAsset: v.NetAsset,
		}
		mac.UserAssets = append(mac.UserAssets, &as)
	}
	return &mac, nil
}
func (bn *Binance) MarginGetMaxBorrowable(symbol string) (MarginMaxBorrowable, error) {
	params := fmt.Sprintf("&asset=%s", symbol)
	url := bnMarginEndpoint + "/sapi/v1/margin/maxBorrowable?" + bn.httpQuerySign(params)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return MarginMaxBorrowable{}, errors.New(bn.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Amount      decimal.Decimal `json:"amount"`
		BorrowLimit decimal.Decimal `json:"borrowLimit"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return MarginMaxBorrowable{}, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 || len(recv.Msg) != 0 {
		//{"code":-3045,"msg":"The system does not have enough asset now."}
		if recv.Code == -3045 {
			return MarginMaxBorrowable{}, nil
		}
		return MarginMaxBorrowable{}, errors.New(bn.Name() + " api err! " + recv.Msg)
	}

	return MarginMaxBorrowable{
		Amount:      recv.Amount,
		BorrowLimit: recv.BorrowLimit,
	}, nil
}
func (bn *Binance) MarginPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType, sideEffectType string, isIsolated bool) (string, error) {
	isIsolateds := "FALSE"
	if isIsolated {
		isIsolateds = "TRUE"
	}
	params := fmt.Sprintf("&newOrderRespType=ACK&symbol=%s&side=%s&type=%s&isIsolated=%s",
		symbol, side, orderType, isIsolateds)
	if cltId != "" {
		params += "&newClientOrderId=" + cltId
	}
	if sideEffectType != "" {
		params += "&sideEffectType=" + sideEffectType
	}
	if orderType == "LIMIT" {
		params += "&timeInForce=" + timeInForce + "&price=" + price.String()
		params += "&quantity=" + qty.String()
	} else if orderType == "MARKET" {
		if amt.IsPositive() {
			params += "&quoteOrderQty=" + amt.String()
		} else if qty.IsPositive() {
			params += "&quantity=" + qty.String()
		}
	} else {
		return "", errors.New("not support order type:" + orderType)
	}
	url := bnMarginEndpoint + "/sapi/v1/margin/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Post(url, nil, bnApiDeadline, headers)
	if err != nil {
		return "", errors.New(bn.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Symbol   string `json:"symbol,omitempty"` // BTCUSDT
		OrderId  int64  `json:"orderId,omitempty"`
		ClientId string `json:"clientOrderId,omitempty"`
		Time     int64  `json:"transactTime,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return "", errors.New(bn.Name() + " api err! " + ret.Msg)
	}

	return strconv.FormatInt(ret.OrderId, 10), nil
}
func (bn *Binance) MarginCancelOrder(symbol string, /*BTCUSDT*/
	orderId, cltId string, isIsolated bool) error {
	isIsolateds := "FALSE"
	if isIsolated {
		isIsolateds = "TRUE"
	}
	params := fmt.Sprintf("&symbol=%s&isIsolated=%s", symbol, isIsolateds)
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return errors.New(bn.Name() + " orderId or cltId empty!")
	}
	url := bnMarginEndpoint + "/sapi/v1/margin/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Delete(url, bnApiDeadline, headers)
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Status string `json:"status,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(ret.Msg)
	}
	if ret.Status != "CANCELED" {
		return errors.New(bn.Name() + " cancel failed! status now: " + ret.Status)
	}
	return nil
}
func (bn *Binance) MarginGetOrder(symbol, orderId, cltId string, isIsolated bool) (*MarginOrder, error) {
	isIsolateds := "FALSE"
	if isIsolated {
		isIsolateds = "TRUE"
	}
	params := fmt.Sprintf("&symbol=%s&isIsolated=%s", symbol, isIsolateds)
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return nil, errors.New(bn.Name() + " orderId or cltId empty!")
	}
	url := bnMarginEndpoint + "/sapi/v1/margin/order?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Get(url, bnApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}

	order := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`

		Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId      int64           `json:"orderId,omitempty"`
		ClientId     string          `json:"clientOrderId,omitempty"` // BTCUSDT
		Price        decimal.Decimal `json:"price"`
		Quantity     decimal.Decimal `json:"origQty"`             // 用户设置的原始订单数量
		ExecutedQty  decimal.Decimal `json:"executedQty"`         // 交易的订单数量
		CummQuoteQty decimal.Decimal `json:"cummulativeQuoteQty"` // 累计交易的金额
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`        // LIMIT/MARKET
		TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		Time         int64           `json:"time,omitempty"`
		UTime        int64           `json:"updateTime,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &order); err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if order.Code != 0 {
		return nil, errors.New(order.Msg)
	}
	return &MarginOrder{
		Symbol:      order.Symbol,
		OrderId:     strconv.FormatInt(order.OrderId, 10),
		ClientId:    order.ClientId,
		Price:       order.Price,
		Qty:         order.Quantity,
		FilledQty:   order.ExecutedQty,
		FilledAmt:   order.CummQuoteQty,
		Status:      order.Status,
		Type:        order.Type,
		TimeInForce: order.TimeInForce,
		Side:        order.Side,
		CTime:       order.Time,
		UTime:       order.UTime,
	}, nil
}
func (bn *Binance) MarginGetTrades(symbol, orderId string, isIsolated bool) ([]*MarginTrade, error) {
	isIsolateds := "FALSE"
	if isIsolated {
		isIsolateds = "TRUE"
	}
	params := fmt.Sprintf("&symbol=%s&isIsolated=%s", symbol, isIsolateds)
	params += "&orderId=" + orderId
	url := bnMarginEndpoint + "/sapi/v1/margin/myTrades?" + bn.httpQuerySign(params)
	headers := map[string]string{"X-MBX-APIKEY": bn.apikey}
	_, resp, err := ihttp.Get(url, bnApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("MarginGetTrades", resp)
	}

	trades := []struct {
		Symbol   string          `json:"symbol,omitempty"` // BTCUSDT
		OrderId  int64           `json:"orderId,omitempty"`
		Fee      decimal.Decimal `json:"commission"`
		FeeAsset string          `json:"commissionAsset"` // BTC
		Time     int64           `json:"time,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &trades); err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	ret := make([]*MarginTrade, 0, len(trades))
	for i := range trades {
		ret = append(ret, &MarginTrade{
			Symbol:   trades[i].Symbol,
			OrderId:  orderId,
			Fee:      trades[i].Fee,
			FeeAsset: trades[i].FeeAsset,
			Time:     trades[i].Time,
		})
	}
	return ret, nil
}
