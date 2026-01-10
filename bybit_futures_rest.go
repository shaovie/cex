package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

func (bb *Bybit) FuturesSupported(typ string) bool {
	if typ == "UM" || typ == "CM" {
		return true
	}
	return false
}
func (bb *Bybit) FuturesServerTime(typ string) (int64, error) {
	return bb.serverTime()
}
func (bb *Bybit) FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal {
	return size
}
func (bb *Bybit) FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal {
	return qty
}
func (bb *Bybit) FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error) {
	typ = bb.fromStdCategory(typ)
	url := bbUniEndpoint + "/v5/market/instruments-info?category=" + typ
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
					StepSize    decimal.Decimal `json:"qtyStep,omitempty"`
					MinNotional decimal.Decimal `json:"minNotionalValue,omitempty"`
				} `json:"lotSizeFilter,omitempty"`
				PriceFilter struct {
					TickSize decimal.Decimal `json:"tickSize,omitempty"`
					MinPrice      decimal.Decimal `json:"minPrice,omitempty"`
					MaxPrice      decimal.Decimal `json:"maxPrice,omitempty"`
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
	all := make(map[string]*FuturesExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Result.List {
		if pair.Status != "Trading" {
			continue
		}

		ep := &FuturesExchangePairRule{
			Symbol:        pair.Symbol,
			Base:          pair.Base,
			Quote:         pair.Quote,
			PriceTickSize: pair.PriceFilter.TickSize,
			MaxOrderQty:   pair.LotSizeFilter.MaxQty,
			MinOrderQty:   pair.LotSizeFilter.MinQty,
			MinPrice:      pair.PriceFilter.MinPrice,
			MaxPrice:      pair.PriceFilter.MaxPrice,
			QtyStep:       pair.LotSizeFilter.StepSize,
			MinNotional:   pair.LotSizeFilter.MinNotional,
			Time:          now,
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (bb *Bybit) FuturesGetBBO(typ, symbol string) (BestBidAsk, error) {
	typ = bb.fromStdCategory(typ)
	url := bbUniEndpoint + "/v5/market/tickers?category="+typ+"&symbol=" + symbol
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
func (bb *Bybit) FuturesGetAllAssets(typ string) (map[string]*FuturesAsset, error) {
	query := "accountType=UNIFIED"
	if typ == "UM" {
		query += "&coin=USDT"
	}
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
				Coin []struct{
					Avail decimal.Decimal `json:"walletBalance,omitempty"`
					Symbol string `json:"coin,omitempty"`
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

	assetsMap := make(map[string]*FuturesAsset)
	for _, v := range recv.Result.List {
		if len(v.Coin) == 0 {
			continue
		}
		for _, as := range v.Coin {
			assetsMap[as.Symbol] = &FuturesAsset{
				Symbol: as.Symbol,
				Avail:  as.Avail,
				Total:  as.Avail,
			}
		}
	}
	return assetsMap, nil
}
func (bb *Bybit) FuturesPlaceOrder(typ, symbol, cltId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, orderType, timeInForce string,
	positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error) {
	typ = bb.fromStdCategory(typ)
	params := map[string]any{
		"category":     typ,
		"symbol":  symbol,
		"side": bb.fromStdSide(side),
		"orderType": bb.fromStdOrderType(orderType),
		"orderFilter": "Order",
		"qty": qty.String(),
	}
	if cltId != "" {
		params["orderLinkId"] = cltId
	}
	if orderType == "LIMIT" {
		params["price"] = price.String()
		if timeInForce != "" {
			params["timeInForce"] = timeInForce
		}
	}
	if reduceOnly == 1 {
		params["reduceOnly"] = true
	}
	body, _ := json.Marshal(params)
	url := bbUniEndpoint + "/v5/order/create"
	_, resp, err := ihttp.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			OrderId     string          `json:"orderId"`
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
func (bb *Bybit) FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error) {
	typ = bb.fromStdCategory(typ)
	query := "category=" + typ + "&symbol=" + symbol
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
				Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
				OrderId      string           `json:"orderId,omitempty"`
				ClientId     string          `json:"orderLinkId,omitempty"` 
				Price        decimal.Decimal `json:"price,omitempty"`
				Quantity     decimal.Decimal `json:"qty,omitempty"`             // 用户设置的原始订单数量
				Type         string          `json:"orderType,omitempty"`        // LIMIT/MARKET
				TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string          `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal `json:"cumExecQty,omitempty"`         // 交易的订单数量
				CummQuoteQty decimal.Decimal `json:"cumExecValue,omitempty"` // 累计交易的金额
				AvgPrice       decimal.Decimal `json:"avgPrice,omitempty"`
				FeeQty       decimal.Decimal `json:"cumExecFee,omitempty"`
				Status       string          `json:"orderStatus,omitempty"`
				Time         string           `json:"createdTime,omitempty"`
				UTime        string           `json:"updatedTime,omitempty"`
				FeeDetail map[string]string `json:"cumFeeDetail,omitempty"`
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
	o := &FuturesOrder{
		Symbol:      order.Symbol,
		OrderId:     order.OrderId,
		ClientId:    order.ClientId,
		Price:       order.Price,
		Qty:         order.Quantity,
		FilledQty:   order.ExecutedQty,
		FilledAmt:   order.CummQuoteQty,
		Status:      bb.toStdOrderStatus(order.Status),
		Type:        bb.toStdOrderType(order.Type),
		Side:        bb.toStdSide(order.Side),
	}
	o.CTime, _ = strconv.ParseInt(order.Time, 10, 64)
	o.UTime, _ = strconv.ParseInt(order.UTime, 10, 64)
	if typ == "inverse" {
		o.AvgPrice =  order.AvgPrice
		for k, v := range order.FeeDetail {
			o.FeeAsset = k
			o.FeeQty, _ = decimal.NewFromString(v)
			break
		}
	} else {
		o.FeeAsset = "USDT"
		o.FeeQty = order.FeeQty
	}
	return o, nil
}
func (bb *Bybit) FuturesGetOpenOrders(typ, symbol string) ([]*FuturesOrder, error) {
	typ = bb.fromStdCategory(typ)
	query := "limit=50&category=" + typ
	if symbol != "" {
		query += "&symbol=" + symbol
	} else if typ == "linear" {
		query += "&settleCoin=USDT"
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
				Symbol       string          `json:"symbol,omitempty"` // BTCUSDT
				OrderId      string           `json:"orderId,omitempty"`
				ClientId     string          `json:"orderLinkId,omitempty"` 
				Price        decimal.Decimal `json:"price,omitempty"`
				Quantity     decimal.Decimal `json:"qty,omitempty"`             // 用户设置的原始订单数量
				Type         string          `json:"orderType,omitempty"`        // LIMIT/MARKET
				TimeInForce  string          `json:"timeInForce,omitempty"` // GTC/FOK/IOC
				Side         string          `json:"side,omitempty"`
				ExecutedQty  decimal.Decimal `json:"cumExecQty,omitempty"`         // 交易的订单数量
				CummQuoteQty decimal.Decimal `json:"cumExecValue,omitempty"` // 累计交易的金额
				AvgPrice       decimal.Decimal `json:"avgPrice,omitempty"`
				FeeQty       decimal.Decimal `json:"cumExecFee,omitempty"`
				Status       string          `json:"orderStatus,omitempty"`
				Time         string           `json:"createdTime,omitempty"`
				UTime        string           `json:"updatedTime,omitempty"`
				FeeDetail map[string]string `json:"cumFeeDetail,omitempty"`
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	oL := make([]*FuturesOrder, 0, len(recv.Result.List))
	for _, order := range recv.Result.List {
		o := &FuturesOrder{
			Symbol:      order.Symbol,
			OrderId:     order.OrderId,
			ClientId:    order.ClientId,
			Price:       order.Price,
			Qty:         order.Quantity,
			FilledQty:   order.ExecutedQty,
			FilledAmt:   order.CummQuoteQty,
			Status:      bb.toStdOrderStatus(order.Status),
			Type:        bb.toStdOrderType(order.Type),
			Side:        bb.toStdSide(order.Side),
		}
		o.CTime, _ = strconv.ParseInt(order.Time, 10, 64)
		o.UTime, _ = strconv.ParseInt(order.UTime, 10, 64)
		if typ == "inverse" {
			o.AvgPrice =  order.AvgPrice
			for k, v := range order.FeeDetail {
				o.FeeAsset = k
				o.FeeQty, _ = decimal.NewFromString(v)
				break
			}
		} else {
			o.FeeAsset = "USDT"
			o.FeeQty = order.FeeQty
		}
		oL = append(oL, o)
	}
	return oL, nil
}
func (bb *Bybit) FuturesCancelOrder(typ, symbol, orderId, cltId string) error {
	typ = bb.fromStdCategory(typ)
	params := map[string]any{
		"category":     typ,
		"symbol":  symbol,
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
			OrderId     string          `json:"orderId"`
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
func (bb *Bybit) FuturesSwitchTradeMode(typ, symbol string, mode, leverage int) error {
	typ = bb.fromStdCategory(typ)
	leverageS := fmt.Sprintf("%d", leverage)
	params := map[string]any{
		"category":     typ,
		"symbol":  symbol,
		"buyLeverage": leverageS,
		"sellLeverage": leverageS,
	}
	body, _ := json.Marshal(params)
	url := bbUniEndpoint + "/v5/position/set-leverage"
	_, resp, err := ihttp.Post(url, body, bbApiDeadline, bb.buildHeaders("", string(body)))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return errors.New(bb.Name() + " Unmarshal err! " + err.Error())
	}
	if recv.Code != 0 {
		return errors.New(recv.Msg)
	}
	return nil
}
func (bb *Bybit) FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error) {
	typ = bb.fromStdCategory(typ)
	query := "limit=200&category=" + typ
	if typ == "linear" {
		query += "&settleCoin=USDT"
	}
	url := bbUniEndpoint + "/v5/position/list?" + query
	_, resp, err := ihttp.Get(url, bbApiDeadline, bb.buildHeaders(query, ""))
	recv := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			List []struct {
				Symbol     string          `json:"symbol"`
				Side     string          `json:"side"`
				EntryPrice decimal.Decimal `json:"avgPrice,omitempty"`
				Leverage   decimal.Decimal `json:"leverage,omitempty"`
				LiqPrice   decimal.Decimal `json:"liqPrice,omitempty"`
				//MarkPrice decimal.Decimal `json:"markPrice,omitempty"`
				PositionQty   decimal.Decimal `json:"size,omitempty"`
				NotionalVal   decimal.Decimal `json:"positionValue,omitempty"`
				UnrealisedPnl decimal.Decimal `json:"unrealisedPnl,omitempty"`
				Time int64 `json:"updateTime,omitempty"` // msec
			} `json:"list,omitempty"`
		} `json:"result,omitempty"`
	}{}

	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(bb.Name() + " api err! " + recv.Msg)
	}
	positionM := make(map[string]*FuturesPosition)
	for _, v := range recv.Result.List {
		side := bb.toStdSide(v.Side)
		cp := FuturesPosition{
			Symbol:           v.Symbol,
			Side:             side,
			PositionQty:      v.PositionQty,
			EntryPrice:       v.EntryPrice,
			UnRealizedProfit: v.UnrealisedPnl,
			LiqPrice:         v.LiqPrice,
			Leverage:         v.Leverage,
			UTime:            v.Time,
		}
		positionM[cp.Symbol] = &cp
	}

	return positionM, nil
}
