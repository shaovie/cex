package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shaovie/gutils/ihttp"
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
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
	if err != nil {
		return 0, errors.New(bb.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Result struct {
			Time string `json:"timeNano,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	t, _ := strconv.ParseInt(recv.Result.Time, 10, 64)
	return t / 1000000, nil
}
func (bb *Bybit) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := bbUniEndpoint + "/v5/market/instruments-info?category=spot"
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol        string `json:"symbol,omitempty"`
				Base          string `json:"baseCoin,omitempty"`
				Quote         string `json:"quoteCoin,omitempty"`
				Status        string `json:"status,omitempty"`
				LotSizeFilter struct {
					MaxQty      decimal.Decimal `json:"maxOrderQty,omitempty"`
					MinQty      decimal.Decimal `json:"minOrderQty,omitempty"`
					MinAmt      decimal.Decimal `json:"minOrderAmt,omitempty"`
					MaxAmt      decimal.Decimal `json:"maxOrderAmt,omitempty"`
					StepSize    decimal.Decimal `json:"basePrecision,omitempty"`
					AmtStepSize decimal.Decimal `json:"quotePrecision,omitempty"`
				} `json:"lotSizeFilter,omitempty"`
				PriceFilter struct {
					TickSize decimal.Decimal `json:"tickSize,omitempty"`
				} `json:"priceFilter,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Result.List {
		if pair.Status != "Trading" {
			continue
		}

		ep := &SpotExchangePairRule{
			Symbol:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
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
	return all, nil
}
func (bb *Bybit) SpotGetBBO(symbol string) (BestBidAsk, error) {
	url := bbUniEndpoint + "/v5/market/tickers?category=spot&symbol=" + symbol
	_, resp, err := ihttp.Get(url, bbApiDeadline, nil)
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
		} `json:"result,omitempty"`
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
func (bb *Bybit) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	query := "accountType=UNIFIED"
	url := bbUniEndpoint + "/v5/account/wallet-balance?" + query
	_, resp, err := ihttp.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
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
					Avail  decimal.Decimal `json:"walletBalance,omitempty"`
					Symbol string          `json:"coin,omitempty"`
				} `json:"coin,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
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
	side, timeInForce, orderType string) (string, error) {

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
	}
	body, _ := json.Marshal(params)
	url := bbUniEndpoint + "/v5/order/create"
	_, resp, err := ihttp.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			OrderId string `json:"orderId"`
		} `json:"result,omitempty"`
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
	_, resp, err := ihttp.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			OrderId string `json:"orderId"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return errors.New(recv.Msg)
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
	_, resp, err := ihttp.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
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
				Price        decimal.Decimal   `json:"price,omitempty"`
				Quantity     decimal.Decimal   `json:"qty,omitempty"`         // 用户设置的原始订单数量
				Type         string            `json:"orderType,omitempty"`   // LIMIT/MARKET
				TimeInForce  string            `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string            `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal   `json:"cumExecQty,omitempty"`   // 交易的订单数量
				CummQuoteQty decimal.Decimal   `json:"cumExecValue,omitempty"` // 累计交易的金额
				FeeQty       decimal.Decimal   `json:"cumExecFee,omitempty"`
				Status       string            `json:"orderStatus,omitempty"`
				Time         string            `json:"createdTime,omitempty"`
				UTime        string            `json:"updatedTime,omitempty"`
				FeeDetail    map[string]string `json:"cumFeeDetail,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
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
	_, resp, err := ihttp.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
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
				Price        decimal.Decimal   `json:"price,omitempty"`
				Quantity     decimal.Decimal   `json:"qty,omitempty"`         // 用户设置的原始订单数量
				Type         string            `json:"orderType,omitempty"`   // LIMIT/MARKET
				TimeInForce  string            `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string            `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal   `json:"cumExecQty,omitempty"`   // 交易的订单数量
				CummQuoteQty decimal.Decimal   `json:"cumExecValue,omitempty"` // 累计交易的金额
				FeeQty       decimal.Decimal   `json:"cumExecFee,omitempty"`
				Status       string            `json:"orderStatus,omitempty"`
				Time         string            `json:"createdTime,omitempty"`
				UTime        string            `json:"updatedTime,omitempty"`
				FeeDetail    map[string]string `json:"cumFeeDetail,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
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
