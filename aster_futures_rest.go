package cex

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// AsterFuturesOrder 合约订单
type AsterFuturesOrder struct {
	Code         int             `json:"code,omitempty"`
	Msg          string          `json:"msg,omitempty"`
	Symbol       string          `json:"symbol,omitempty"`
	OrderId      json.Number     `json:"orderId,omitempty"`
	ClientId     string          `json:"clientOrderId,omitempty"`
	Price        decimal.Decimal `json:"price"`
	AvgPrice     decimal.Decimal `json:"avgPrice"`
	OrigQty      decimal.Decimal `json:"origQty"`
	ExecutedQty  decimal.Decimal `json:"executedQty"`
	CummQuoteQty decimal.Decimal `json:"cumQuote"`
	Status       string          `json:"status,omitempty"`
	Type         string          `json:"type,omitempty"`
	TimeInForce  string          `json:"timeInForce,omitempty"`
	Side         string          `json:"side,omitempty"`
	Time         int64           `json:"time,omitempty"`
	UpdateTime   int64           `json:"updateTime,omitempty"`
}

func (as *Aster) FuturesSupported(typ string) bool {
	if typ == "UM" { // 只支持U本位
		return true
	}
	return false
}
func (as *Aster) FuturesServerTime(typ string) (int64, error) {
	link := asFuturesEndpoint + "/fapi/v3/time"
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return 0, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Time int64 `json:"serverTime,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return 0, errors.New(as.Name() + " unmarshal error! " + err.Error())
	}
	return recv.Time, nil
}
func (as *Aster) FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal {
	return size
}
func (as *Aster) FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal {
	return qty
}
func (as *Aster) FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error) {
	link := asFuturesEndpoint + "/fapi/v3/exchangeInfo"
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Symbols []struct {
			Symbol       string `json:"symbol,omitempty"`
			Base         string `json:"baseAsset,omitempty"`
			Quote        string `json:"quoteAsset,omitempty"`
			Status       string `json:"status,omitempty"`
			ContractType string `json:"contractType,omitempty"`
			Filters      []struct {
				FilterType  string          `json:"filterType,omitempty"`
				MaxPrice    decimal.Decimal `json:"maxPrice"`
				MinPrice    decimal.Decimal `json:"minPrice"`
				MaxQty      decimal.Decimal `json:"maxQty"`
				MinQty      decimal.Decimal `json:"minQty"`
				TickSize    decimal.Decimal `json:"tickSize"`
				StepSize    decimal.Decimal `json:"stepSize"`
				MinNotional decimal.Decimal `json:"notional"`
			} `json:"filters,omitempty"`
		} `json:"symbols,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Symbols) == 0 {
		return nil, as.handleExceptionResp("FuturesLoadAllPairRule", resp)
	}
	all := make(map[string]*FuturesExchangePairRule)
	now := time.Now().Unix()
	for _, pair := range recv.Symbols {
		if pair.Status != "TRADING" || pair.ContractType != "PERPETUAL" {
			continue
		}
		ep := &FuturesExchangePairRule{
			Typ:    typ,
			Symbol: pair.Symbol,
			Base:   pair.Base,
			Quote:  pair.Quote,
			Time:   now,
		}
		for _, flt := range pair.Filters {
			if flt.FilterType == "PRICE_FILTER" {
				ep.MaxPrice = flt.MaxPrice
				ep.MinPrice = flt.MinPrice
				ep.PriceTickSize = flt.TickSize
			} else if flt.FilterType == "LOT_SIZE" {
				ep.MinOrderQty = flt.MinQty
				ep.MaxOrderQty = flt.MaxQty
				ep.QtyStep = flt.StepSize
			} else if flt.FilterType == "MIN_NOTIONAL" {
				ep.MinNotional = flt.MinNotional
			}
		}
		all[ep.Symbol] = ep
	}
	return all, nil
}
func (as *Aster) FuturesGetBBO(typ, symbol string) (BestBidAsk, error) {
	path := "/fapi/v3/ticker/bookTicker"
	link := asFuturesEndpoint + path + "?symbol=" + symbol
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return BestBidAsk{}, errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Symbol   string          `json:"symbol,omitempty"`
		BidPrice decimal.Decimal `json:"bidPrice"`
		BidQty   decimal.Decimal `json:"bidQty"`
		AskPrice decimal.Decimal `json:"askPrice"`
		AskQty   decimal.Decimal `json:"askQty"`
		Time     int64           `json:"time,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return BestBidAsk{}, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Symbol) == 0 {
		return BestBidAsk{}, as.handleExceptionResp("FuturesGetBBO", resp)
	}
	return BestBidAsk{
		Symbol:   ret.Symbol,
		Time:     ret.Time, // msec
		BidPrice: ret.BidPrice,
		BidQty:   ret.BidQty,
		AskPrice: ret.AskPrice,
		AskQty:   ret.AskQty,
	}, nil
}
func (as *Aster) FuturesGetAllAssets(typ string) (map[string]*FuturesAsset, error) {
	path := "/fapi/v3/balance"
	link := asFuturesEndpoint + path + "?" + as.buildQuery("")
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("FuturesGetAllAssets", resp)
	}
	recv := []struct {
		Symbol            string          `json:"asset,omitempty"`
		Total             decimal.Decimal `json:"balance"`
		Avail             decimal.Decimal `json:"availableBalance"`
		MaxWithdrawAmount decimal.Decimal `json:"maxWithdrawAmount"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	assetsMap := make(map[string]*FuturesAsset, len(recv))
	for _, v := range recv {
		if v.Total.IsZero() && v.Avail.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &FuturesAsset{
			Symbol:            v.Symbol,
			Avail:             v.Avail,
			MaxWithdrawAmount: v.MaxWithdrawAmount,
			Total:             v.Total,
		}
	}
	return assetsMap, nil
}
func (as *Aster) FuturesPlaceOrder(typ, symbol, clientId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, orderType, timeInForce, positionMode string,
	tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error) {
	if !qty.IsPositive() {
		return "", errors.New("qty too small! qty=" + qty.String())
	}
	if orderType != "LIMIT" && orderType != "MARKET" {
		return "", errors.New(as.Name() + " not support order type! " + orderType)
	}
	params := "symbol=" + symbol + "&side=" + as.toStdSide(side) + "&type=" + orderType +
		"&quantity=" + qty.String() + "&positionSide=" + positionMode + "&newOrderRespType=ACK"
	params += "&stpMode=EXPIRE_BOTH" // 自成交保护: 自成交时买卖两单都撤(调用方不需要传)
	if clientId != "" {
		params += "&newClientOrderId=" + clientId
	}
	if orderType == "LIMIT" {
		if len(timeInForce) == 0 {
			timeInForce = "GTC"
		}
		params += "&timeInForce=" + as.toStdTimeInForce(timeInForce) + "&price=" + price.String()
	}
	if reduceOnly == 1 && positionMode == "BOTH" { // 双向持仓模式下不接受此参数
		params += "&reduceOnly=true"
	}
	path := "/fapi/v3/order"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link, as.buildBody(params), asApiDeadline, as.buildHeaders())
	if err != nil {
		return "", errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return "", as.handleExceptionResp("FuturesPlaceOrder", resp)
	}
	return ret.OrderId.String(), nil
}

func (as *Aster) FuturesPlaceOrderMultipleSupported() bool {
	return true
}

// FuturesPlaceOrderMultiple 批量下单: aster一次最多5单, 超过单次上限自动分批
// 返回数组里每单可能成功或失败, 只要有成功的就算成功; 某一批失败立刻返回错误
func (as *Aster) FuturesPlaceOrderMultiple(typ string, orders []FuturesPostOrder) error {
	if len(orders) == 0 {
		return errors.New(as.Name() + " orders empty!")
	}
	for i := 0; i < len(orders); i += 5 { // 单次最多5单
		end := i + 5
		if end > len(orders) {
			end = len(orders)
		}
		if err := as.futuresPlaceOrderMultiple(orders[i:end]); err != nil {
			return err
		}
	}
	return nil
}
func (as *Aster) futuresPlaceOrderMultiple(orders []FuturesPostOrder) error {
	type BatchOrder struct {
		Symbol           string `json:"symbol"`
		Side             string `json:"side"`
		Type             string `json:"type"`
		PositionSide     string `json:"positionSide,omitempty"`
		TimeInForce      string `json:"timeInForce,omitempty"`
		Price            string `json:"price,omitempty"`
		Quantity         string `json:"quantity"`
		ReduceOnly       string `json:"reduceOnly,omitempty"`
		NewClientOrderId string `json:"newClientOrderId,omitempty"`
		StpMode          string `json:"stpMode"` // 自成交保护: 自成交时买卖两单都撤
	}
	orderL := make([]BatchOrder, 0, len(orders))
	for i := range orders {
		if orders[i].Type != "LIMIT" && orders[i].Type != "MARKET" {
			return errors.New(as.Name() + " not support order type! " + orders[i].Type)
		}
		po := BatchOrder{
			Symbol:           orders[i].Symbol,
			Side:             as.toStdSide(orders[i].Side),
			Type:             orders[i].Type,
			PositionSide:     orders[i].PositionMode,
			Quantity:         orders[i].Qty.String(),
			NewClientOrderId: orders[i].ClientId,
			StpMode:          "EXPIRE_BOTH",
		}
		if orders[i].Type == "LIMIT" {
			if len(orders[i].TimeInForce) == 0 {
				po.TimeInForce = "GTC"
			} else {
				po.TimeInForce = as.toStdTimeInForce(orders[i].TimeInForce)
			}
			po.Price = orders[i].Price.String()
		}
		if orders[i].ReduceOnly == 1 && orders[i].PositionMode == "BOTH" { // 双向持仓模式下不接受此参数
			po.ReduceOnly = "true"
		}
		orderL = append(orderL, po)
	}
	payload, _ := json.Marshal(orderL)
	path := "/fapi/v3/batchOrders"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link, as.buildBody("batchOrders="+url.QueryEscape(string(payload))),
		asApiDeadline, as.buildHeaders())
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return as.handleExceptionResp("FuturesPlaceOrderMultiple", resp)
	}
	ret := []struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	success, failMsg := 0, ""
	for _, v := range ret {
		if len(v.OrderId) > 0 {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.Msg
		}
	}
	if success == 0 {
		return errors.New(as.Name() + " batch order all fail! " + failMsg)
	}
	return nil
}
func (as *Aster) FuturesCancelOrder(typ, symbol, orderId, cltId string) error {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return errors.New(as.Name() + " orderId or cltId empty!")
	}
	path := "/fapi/v3/order"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Delete(link, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := AsterFuturesOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 || ret.Status != "CANCELED" {
		return as.handleExceptionResp("FuturesCancelOrder", resp)
	}
	return nil
}

func (as *Aster) FuturesCancelOrderMultipleSupported() bool {
	return true
}

// FuturesCancelOrderMultiple 批量撤单: 一次最多10单, 超过单次上限自动分批
// orderIdList和origClientOrderIdList只能二选一, 所以先撤orderIds再撤cltIds
func (as *Aster) FuturesCancelOrderMultiple(typ, symbol string, orderIds, cltIds []string) error {
	if len(orderIds) == 0 && len(cltIds) == 0 {
		return errors.New(as.Name() + " orderIds or cltIds empty!")
	}
	for i := 0; i < len(orderIds); i += 10 { // 单次最多10单
		end := i + 10
		if end > len(orderIds) {
			end = len(orderIds)
		}
		if err := as.futuresCancelOrderMultiple(symbol, orderIds[i:end], nil); err != nil {
			return err
		}
	}
	for i := 0; i < len(cltIds); i += 10 {
		end := i + 10
		if end > len(cltIds) {
			end = len(cltIds)
		}
		if err := as.futuresCancelOrderMultiple(symbol, nil, cltIds[i:end]); err != nil {
			return err
		}
	}
	return nil
}
func (as *Aster) futuresCancelOrderMultiple(symbol string, orderIds, cltIds []string) error {
	params := "symbol=" + symbol
	if len(orderIds) > 0 {
		payload, _ := json.Marshal(orderIds)
		params += "&orderIdList=" + url.QueryEscape(string(payload))
	} else if len(cltIds) > 0 {
		payload, _ := json.Marshal(cltIds)
		params += "&origClientOrderIdList=" + url.QueryEscape(string(payload))
	} else {
		return errors.New(as.Name() + " orderIds or cltIds empty!")
	}
	path := "/fapi/v3/batchOrders"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Delete(link, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return as.handleExceptionResp("FuturesCancelOrderMultiple", resp)
	}
	ret := []struct {
		Code    int         `json:"code,omitempty"`
		Msg     string      `json:"msg,omitempty"`
		OrderId json.Number `json:"orderId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	success, failMsg := 0, ""
	for _, v := range ret {
		if len(v.OrderId) > 0 {
			success++
		} else if len(failMsg) == 0 {
			failMsg = v.Msg
		}
	}
	if success == 0 {
		return errors.New(as.Name() + " batch cancel all fail! " + failMsg)
	}
	return nil
}
func (as *Aster) FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error) {
	params := "symbol=" + symbol
	if orderId != "" {
		params += "&orderId=" + orderId
	} else if cltId != "" {
		params += "&origClientOrderId=" + cltId
	} else {
		return nil, errors.New(as.Name() + " orderId or cltId empty!")
	}
	path := "/fapi/v3/order"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := AsterFuturesOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.OrderId) == 0 {
		return nil, as.handleExceptionResp("FuturesGetOrder", resp)
	}
	return as.toStdFuturesOrder(&ret), nil
}
func (as *Aster) FuturesGetOpenOrders(typ, symbol string) ([]*FuturesOrder, error) {
	params := ""
	if symbol != "" {
		params = "symbol=" + symbol
	}
	path := "/fapi/v3/openOrders"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("FuturesGetOpenOrders", resp)
	}
	ret := []AsterFuturesOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*FuturesOrder, 0, len(ret))
	for i := range ret {
		dl = append(dl, as.toStdFuturesOrder(&ret[i]))
	}
	return dl, nil
}

// FuturesGetFilledOrders 查已结束的订单(全成/已撤/过期): 不带时间参数时是最近7天
func (as *Aster) FuturesGetFilledOrders(typ, symbol string) ([]*FuturesOrder, error) {
	params := "limit=100"
	if symbol != "" {
		params = "symbol=" + symbol + "&" + params
	}
	path := "/fapi/v3/allOrders"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("FuturesGetFilledOrders", resp)
	}
	ret := []AsterFuturesOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*FuturesOrder, 0, len(ret))
	for i := range ret {
		dl = append(dl, as.toStdFuturesOrder(&ret[i]))
	}
	return dl, nil
}
func (as *Aster) FuturesSwitchPositionMode(typ string, mode int) error {
	m := ""
	if mode == 1 { // 双仓
		m = "true"
	} else if mode == 0 { // 单仓
		m = "false"
	} else {
		return errors.New("params error")
	}
	path := "/fapi/v3/positionSide/dual"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link, as.buildBody("dualSidePosition="+m), asApiDeadline, as.buildHeaders())
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 && ret.Code != 200 && ret.Code != -4059 { // -4059 已经是该持仓模式
		return errors.New(as.Name() + " " + ret.Msg)
	}
	return nil
}
func (as *Aster) FuturesSwitchTradeMode(typ, symbol string, mode, leverage int) error {
	if err := as.futuresSwitchTradeMode(symbol, mode); err != nil {
		return err
	}
	return as.futuresSetLeverage(symbol, leverage)
}
func (as *Aster) futuresSwitchTradeMode(symbol string, mode int) error {
	m := ""
	if mode == 1 { // 逐仓
		m = "ISOLATED"
	} else if mode == 0 { // 全仓
		m = "CROSSED"
	} else {
		return errors.New("params error")
	}
	path := "/fapi/v3/marginType"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link, as.buildBody("symbol="+symbol+"&marginType="+m),
		asApiDeadline, as.buildHeaders())
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 && ret.Code != 200 && ret.Code != -4046 { // -4046 已经是该保证金模式
		return errors.New(as.Name() + " " + ret.Msg)
	}
	return nil
}
func (as *Aster) futuresSetLeverage(symbol string, leverage int) error {
	if leverage < 1 || leverage > 125 {
		return errors.New("leverage error 1~125")
	}
	path := "/fapi/v3/leverage"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link,
		as.buildBody("symbol="+symbol+"&leverage="+strconv.Itoa(leverage)), asApiDeadline, as.buildHeaders())
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code     int             `json:"code,omitempty"`
		Msg      string          `json:"msg,omitempty"`
		Symbol   string          `json:"symbol,omitempty"`
		Leverage decimal.Decimal `json:"leverage"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 && ret.Code != 200 {
		return errors.New(as.Name() + " " + ret.Msg)
	}
	return nil
}
func (as *Aster) FuturesMaintMargin(typ, symbol string) ([]*FuturesLeverageBracket, error) {
	params := ""
	if symbol != "" {
		params = "symbol=" + symbol
	}
	path := "/fapi/v3/leverageBrackets"
	link := asFuturesEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("FuturesMaintMargin", resp)
	}
	ret := []struct {
		Symbol   string `json:"symbol,omitempty"`
		Brackets []struct {
			Bracket          decimal.Decimal `json:"bracket"`
			InitialLeverage  decimal.Decimal `json:"initialLeverage"`
			NotionalCap      decimal.Decimal `json:"notionalCap"`
			NotionalFloor    decimal.Decimal `json:"notionalFloor"`
			MaintMarginRatio decimal.Decimal `json:"maintMarginRatio"`
			Cum              decimal.Decimal `json:"cum"`
		} `json:"brackets,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret) == 0 {
		return nil, errors.New(as.Name() + " resp empty")
	}
	lbs := make([]*FuturesLeverageBracket, 0, len(ret[0].Brackets))
	for _, v := range ret[0].Brackets {
		lbs = append(lbs, &FuturesLeverageBracket{
			Bracket:          v.Bracket.IntPart(),
			InitialLeverage:  v.InitialLeverage.IntPart(),
			NotionalCap:      v.NotionalCap,
			NotionalFloor:    v.NotionalFloor,
			MaintMarginRatio: v.MaintMarginRatio,
			Cum:              v.Cum,
		})
	}
	return lbs, nil
}
func (as *Aster) FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error) {
	path := "/fapi/v3/positionRisk"
	link := asFuturesEndpoint + path + "?" + as.buildQuery("")
	_, resp, err := as.Get(link, asApiDeadline, nil)
	if err != nil {
		return nil, errors.New(as.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, as.handleExceptionResp("FuturesGetAllPositionList", resp)
	}
	recv := []struct {
		Symbol           string          `json:"symbol,omitempty"`
		PositionSide     string          `json:"positionSide,omitempty"`
		PositionQty      decimal.Decimal `json:"positionAmt"`
		EntryPrice       decimal.Decimal `json:"entryPrice"`
		LiqPrice         decimal.Decimal `json:"liquidationPrice"`
		UnRealisedProfit decimal.Decimal `json:"unRealizedProfit"`
		Leverage         decimal.Decimal `json:"leverage"`
		Time             int64           `json:"updateTime,omitempty"` // msec
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(as.Name() + " unmarshal fail! " + err.Error())
	}
	positionM := make(map[string]*FuturesPosition)
	for _, v := range recv {
		if v.PositionSide != "BOTH" { // 只支持单仓模式
			continue
		}
		side := "SELL"
		if v.PositionQty.IsPositive() {
			side = "BUY"
		}
		positionM[v.Symbol] = &FuturesPosition{
			Symbol:           v.Symbol,
			Side:             side,
			PositionQty:      v.PositionQty.Abs(),
			EntryPrice:       v.EntryPrice,
			UnRealizedProfit: v.UnRealisedProfit,
			LiqPrice:         v.LiqPrice,
			Leverage:         v.Leverage,
			UTime:            v.Time,
		}
	}
	return positionM, nil
}

// toStdFuturesOrder 交易所合约订单转标准订单
func (as *Aster) toStdFuturesOrder(v *AsterFuturesOrder) *FuturesOrder {
	return &FuturesOrder{
		Symbol:    v.Symbol,
		OrderId:   v.OrderId.String(),
		ClientId:  v.ClientId,
		Price:     v.Price,
		Qty:       v.OrigQty,
		FilledQty: v.ExecutedQty,
		FilledAmt: v.CummQuoteQty,
		AvgPrice:  v.AvgPrice,
		Status:    as.toStdOrderStatus(v.Status),
		Type:      as.toStdOrderType(v.Type),
		Side:      as.toStdSide(v.Side),
		CTime:     v.Time,
		UTime:     v.UpdateTime,
	}
}
