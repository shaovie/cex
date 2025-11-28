package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shaovie/gutils/ilog"
)

func (gt *Gate) SpotSupported() bool {
	return true
}
func (gt *Gate) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	path := "/api/v4/spot/currency_pairs"
	url := gtUniEndpoint + path
	retCode, resp, err := ihttp.Get(url, gtApiDeadline, nil)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(gt.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}

	recv := []struct {
		Id           string          `json:"id"`
		Base         string          `json:"base,omitempty"`
		Quote        string          `json:"quote,omitempty"`
		Status       string          `json:"trade_status"`
		MinNotional  decimal.Decimal `json:"min_quote_amount,omitempty"`
		QtyPrecision int32           `json:"amount_precision,omitempty"`
		MinBaseQty   decimal.Decimal `json:"min_base_amount,omitempty"`
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
func (gt *Gate) SpotGetAll24hTicker() (map[string]Spot24hTicker, error) {
	path := "/api/v4/spot/tickers"
	url := gtUniEndpoint + path
	retCode, resp, err := ihttp.Get(url, gtApiDeadline, nil)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(gt.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	if resp[0] == '{' {
		return nil, gt.handleExceptionResp("SpotGetAll24hTicker", resp)
	}

	tickers := []GateSpot24hTicker{}
	if err = json.Unmarshal(resp, &tickers); err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	if len(tickers) == 0 {
		return nil, errors.New(gt.Name() + " resp empty")
	}
	allTk := make(map[string]Spot24hTicker, len(tickers))
	for _, tk := range tickers {
		v := Spot24hTicker{
			Symbol:      strings.ReplaceAll(tk.Symbol, "_", ""),
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (gt *Gate) SpotPlaceOrder(symbol, clientId string, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {

	path := "/api/v4/spot/orders"
	url := gtUniEndpoint + path
	symbolS := gt.getSpotSymbol(symbol)
	payload := `{"currency_pair":"` + symbolS + `"`
	if clientId != "" {
		payload += `,"text":"t-` + clientId + `"` // len(clientId) LessOrEqual than 28
	}
	if orderType == "LIMIT" {
		payload += `,"price":"` + price.String() + `"`
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
	retCode, resp, err := ihttp.Post(url, []byte(payload), gtApiDeadline, headers)
	if err != nil {
		return "", errors.New(gt.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return "", errors.New(gt.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Label   string `json:"label,omitempty"`
		Msg     string `json:"message,omitempty"`
		OrderId string `json:"id,omitempty"`
		Status  string `json:"status,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Label != "" {
		return "", errors.New(gt.Name() + " request fail! msg=" + ret.Msg)
	}

	if len(ret.OrderId) == 0 {
		return "", errors.New(gt.Name() + " orderid is empty!")
	}

	return ret.OrderId, nil
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
	retCode, resp, err := ihttp.Delete(url, gtApiDeadline, headers)
	if err != nil {
		return errors.New(gt.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return errors.New(gt.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}

	ret := struct {
		Label string `json:"label,omitempty"`
		Msg   string `json:"message,omitempty"`

		Status string `json:"status,omitempty"`
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
func (gt *Gate) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	symbolS := gt.getSpotSymbol(symbol)
	if orderId == "" && cltId != "" {
		orderId = "t-" + cltId
	}
	path := "/api/v4/spot/orders/" + orderId
	params := "currency_pair=" + symbolS
	headers := gt.buildHeaders("GET", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	retCode, resp, err := ihttp.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(gt.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	order := struct {
		Label string `json:"label,omitempty"`
		Msg   string `json:"message,omitempty"`

		Symbol       string          `json:"currency_pair,omitempty"`
		OrderId      string          `json:"id,omitempty"`
		ClientId     string          `json:"text,omitempty"`
		Price        decimal.Decimal `json:"price,omitempty"`
		Qty          decimal.Decimal `json:"amount,omitempty"`
		ExecutedQty  decimal.Decimal `json:"filled_amount,omitempty"`
		CummQuoteQty decimal.Decimal `json:"filled_total,omitempty"`
		Left         decimal.Decimal `json:"left,omitempty"`
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`
		TimeInForce  string          `json:"time_in_force,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		FeeCoin      string          `json:"fee_currency,omitempty"`
		FeeQty       decimal.Decimal `json:"fee,omitempty"`
		GtQty        decimal.Decimal `json:"gt_fee,omitempty"`    //
		Event        string          `json:"event,omitempty"`     //
		FinishAs     string          `json:"finish_as,omitempty"` //
		Time         int64           `json:"create_time_ms,omitempty"`
		UTime        int64           `json:"update_time_ms,omitempty"`
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
