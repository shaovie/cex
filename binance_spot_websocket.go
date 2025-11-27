package cex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func (bn *Binance) SpotWsPublicOpen() error {
	url := "wss://stream.binance.com:443/stream"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " spot.ws.public con failed! " + err.Error())
	}
	bn.spotWsPublicClosedMtx.Lock()
	bn.spotWsPublicClosed = false
	bn.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (bn *Binance) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BnSubscribeArg{Method: "SUBSCRIBE"}
	arg.Id = "sub-" + gutils.RandomStr(12)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@depth5@100ms")
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@miniTicker")
				}
			}
		}
	}
	if len(arg.Params) > 0 {
		req, _ := json.Marshal(&arg)
		bn.spotWsPublicConnMtx.Lock()
		bn.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bn.spotWsPublicConnMtx.Unlock()
	}
}
func (bn *Binance) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BnSubscribeArg{Method: "UNSUBSCRIBE"}
	arg.Id = "unsub-" + gutils.RandomStr(12)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@depth5@100ms")
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@miniTicker")
				}
			}
		}
	}
	if len(arg.Params) > 0 {
		req, _ := json.Marshal(&arg)
		bn.spotWsPublicConnMtx.Lock()
		bn.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bn.spotWsPublicConnMtx.Unlock()
	}
}
func (bn *Binance) SpotWsPublicSetTickerPool(p *sync.Pool) {
	bn.spotWsPublicTickerPool = p
}
func (bn *Binance) SpotWsPublicSetOrderBookPool(p *sync.Pool) {
	bn.spotWsPublicOrderBookPool = p
}
func (bn *Binance) SpotWsPublicLoop(ch chan<- any) {
	defer bn.SpotWsPublicClose()

	msgPool := sync.Pool{
		New: func() any {
			return &BinanceWsSpotPubMsg{}
		},
	}
	l := 0
	for {
		_, recv, err := bn.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bn.SpotWsPublicIsClosed() { // 并非主动断开
				ilog.Warning(bn.Name() + " spot.ws.public read: " + err.Error())
			}
			break
		}
		msg := msgPool.Get().(*BinanceWsSpotPubMsg)
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(bn.Name() + " spot.ws.public invalid msg:" + string(recv))
			goto END
		}
		if msg.Code != 0 { // 订阅的响应
			ilog.Error(bn.Name() + " spot.ws.public recv subscribe err:" + string(recv))
			goto END
		}
		l = len(msg.Stream)
		if l > 13 && msg.Stream[l-13:l] == "@depth5@100ms" {
			bn.spotWsHandleOrderBook5(strings.ToUpper(msg.Stream[0:l-13]), msg.Data, ch)
		} else if l > 11 && msg.Stream[l-11:l] == "@miniTicker" {
			bn.spotWsHandle24hTickers(msg.Data, ch)
		} else {
			if strings.Index(string(recv), `"result":null`) == -1 {
				ilog.Error(bn.Name() + " spot.ws.public recv unknown msg: " + string(recv))
			}
		}
	END:
		msg.Data = nil
		msgPool.Put(msg)
	}
}
func (bn *Binance) SpotWsPublicIsClosed() bool {
	bn.spotWsPublicClosedMtx.RLock()
	defer bn.spotWsPublicClosedMtx.RUnlock()
	return bn.spotWsPublicClosed
}
func (bn *Binance) SpotWsPublicClose() {
	bn.spotWsPublicClosedMtx.Lock()
	defer bn.spotWsPublicClosedMtx.Unlock()
	if bn.spotWsPublicClosed {
		return
	}
	bn.spotWsPublicClosed = true
	bn.spotWsPublicConn.Close()
}
func (bn *Binance) spotWsHandleOrderBook5(symbol string, data json.RawMessage, ch chan<- any) {
	depth := bn.spotWsPublicOrderBookInnerPool.Get().(*BinanceSpotOrderBook)
	defer bn.spotWsPublicOrderBookInnerPool.Put(depth)
	depth.Bids = depth.Bids[:0]
	depth.Asks = depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		if len(depth.Bids) != len(depth.Asks) {
			ilog.Error(bn.Name() + " spot.ws.public " + symbol + " orderbook5 exception")
			return
		}
		obd := bn.spotWsPublicOrderBookPool.Get().(*OrderBookDepth)
		obd.Symbol = symbol
		obd.Level = len(depth.Bids)
		obd.Time = 0 // 币安不提供
		obd.Bids = obd.Bids[:0]
		obd.Asks = obd.Asks[:0]
		for i, v := range depth.Bids {
			bTk := Ticker{Price: v[0], Quantity: v[1]}
			obd.Bids = append(obd.Bids, bTk)

			v2 := depth.Asks[i]
			aTk := Ticker{Price: v2[0], Quantity: v2[1]}
			obd.Asks = append(obd.Asks, aTk)
		}
		ch <- obd
	}
}
func (bn *Binance) spotWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	ticker := bn.spotWsPublicTickerInnerPool.Get().(*BinanceSpot24hTicker)
	defer bn.spotWsPublicTickerInnerPool.Put(ticker)
	if err := json.Unmarshal(data, ticker); err == nil {
		tk := bn.spotWsPublicTickerPool.Get().(*Spot24hTicker)
		tk.Symbol = ticker.Symbol
		tk.LastPrice = ticker.Last
		tk.Volume = ticker.Volume
		tk.QuoteVolume = ticker.QuoteVolume
		ch <- tk
	}
}

// = priv channel
func (bn *Binance) SpotWsPrivateOpen() error {
	url := "wss://ws-api.binance.com:443/ws-api/v3?returnRateLimits=false"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.spotWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " spot.ws.priv connect failed! " + err.Error())
	}

	arg := struct {
		Id     string `json:"id"`
		Method string `json:"method"`
		Params struct {
			Apikey    string `json:"apiKey"`
			Sign      string `json:"signature"`
			Timestamp int64  `json:"timestamp"`
		} `json:"params"`
	}{
		Id:     "sub-" + gutils.RandomStr(24),
		Method: "userDataStream.subscribe.signature",
	}
	arg.Params.Apikey = bn.apikey
	arg.Params.Timestamp = time.Now().UnixMilli()
	payload := fmt.Sprintf("apiKey=%s&timestamp=%d", arg.Params.Apikey, arg.Params.Timestamp)
	arg.Params.Sign = bn.sign(payload)
	req, _ := json.Marshal(&arg)
	bn.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	_, recv, err := bn.spotWsPrivateConn.ReadMessage()
	if err != nil {
		bn.SpotWsPrivateClose()
		return errors.New(bn.Name() + " spot.ws.priv subscribe resp fail! " + err.Error())
	}
	resp := struct {
		Status int `json:"status"`
	}{}
	if err = json.Unmarshal(recv, &resp); err != nil || resp.Status != 200 {
		ilog.Error(bn.Name() + " spot.ws.priv userDataStream.subscribe.signature : " + string(recv))
		bn.SpotWsPrivateClose()
		return errors.New(bn.Name() + " spot.ws.priv subscribe userdata fail!")
	}

	bn.spotWsPrivateClosedMtx.Lock()
	bn.spotWsPrivateClosed = false
	bn.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (bn *Binance) SpotWsPrivateSubscribe(channels []string) {
}
func (bn *Binance) SpotWsPrivateIsClosed() bool {
	bn.spotWsPrivateClosedMtx.RLock()
	defer bn.spotWsPrivateClosedMtx.RUnlock()
	return bn.spotWsPrivateClosed
}
func (bn *Binance) SpotWsPrivateClose() {
	bn.spotWsPrivateClosedMtx.Lock()
	defer bn.spotWsPrivateClosedMtx.Unlock()
	if bn.spotWsPrivateClosed {
		return
	}
	bn.spotWsPrivateClosed = true
	bn.spotWsPrivateConn.Close()
}
func (bn *Binance) SpotWsPrivateLoop(ch chan<- any) {
	defer bn.SpotWsPrivateClose()

	type Msg struct {
		Id     string `json:"id,omitempty"`
		Status int    `json:"status,omitempty"`
		Err    struct {
			Code int    `json:"code,omitempty"`
			Msg  string `json:"msg,omitempty"`
		} `json:"error,omitempty"`
		Result json.RawMessage `json:"result,omitempty"`

		Data struct {
			Event string `json:"e,omitempty"`
			Time  int64  `json:"E,omitempty"` // msec
		} `json:"event,omitempty"`
	}
	for {
		_, recv, err := bn.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !bn.SpotWsPrivateIsClosed() {
				ilog.Warning(bn.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		ilog.Rinfo("spot.ws.priv " + string(recv))
		msg := Msg{}
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(bn.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			continue
		}
		if msg.Status == 0 {
			if msg.Data.Event == "executionReport" { // data
				bn.wsSpotHandleOrder(recv, ch)
			} else if msg.Data.Event == "balanceUpdate" { // data
			} else if msg.Data.Event == "outboundAccountPosition" {
			} else if msg.Data.Event == "eventStreamTerminated" { // will be closed
			} else {
				ilog.Error(bn.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		} else { // ws api
			if len(msg.Id) > 5 && msg.Id[0:5] == "sord-" {
				bn.wsSpotHandlePlaceOrderResp(msg.Id, msg.Err.Msg, msg.Result, ch)
			} else if len(msg.Id) > 5 && msg.Id[0:5] == "scle-" {
				bn.wsSpotHandleCancelOrderResp(msg.Err.Msg)
			} else if len(msg.Id) == 0 {
				ilog.Error(bn.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		}
	}
}
func (bn *Binance) wsSpotHandleOrder(data json.RawMessage, ch chan<- any) {
	order := struct {
		Data struct {
			ClientId     string          `json:"c,omitempty"` //
			ClientIdOrig string          `json:"C,omitempty"` //
			OrderId      int64           `json:"i,omitempty"` //
			TradeId      int64           `json:"I,omitempty"` //
			Symbol       string          `json:"s,omitempty"` // BTCUSDT
			Side         string          `json:"S,omitempty"`
			OrderType    string          `json:"o,omitempty"`
			TimeInForce  string          `json:"f,omitempty"` // GTC/FOK/IOC
			IceXX        decimal.Decimal `json:"F,omitempty"` // 冰山订单数量
			CreateTime   int64           `json:"O,omitempty"` // 订单创建时间 msec
			ExecTime     int64           `json:"T,omitempty"` // 成交时间 msec
			Xt           int64           `json:"t,omitempty"` // 未知
			Qty          decimal.Decimal `json:"q,omitempty"` // 原始订单数量
			QtyXX        decimal.Decimal `json:"Q,omitempty"` // Quote Order Quantity
			Price        decimal.Decimal `json:"p,omitempty"` // 原始订单价格
			TrigerPrice  decimal.Decimal `json:"P,omitempty"` // 止盈止损单触发价格
			ExecQty      decimal.Decimal `json:"l,omitempty"` // 末次成交数量
			ExecPrice    decimal.Decimal `json:"L,omitempty"` // 末次成交价格
			ExecutedQty  decimal.Decimal `json:"z,omitempty"` // 订单累计已成交量
			CummQuoteQty decimal.Decimal `json:"Z,omitempty"` // 订单累计已成交金额
			FeeQty       decimal.Decimal `json:"n,omitempty"` // 手续费数量
			FeeAsset     string          `json:"N,omitempty"` // 手续费类型
			EventType    string          `json:"x,omitempty"` // 本次事件的执行类型
			Status       string          `json:"X,omitempty"` // 订单当前状态
		} `json:"event,omitempty"`
	}{}
	if err := json.Unmarshal(data, &order); err == nil && order.Data.OrderId > 0 {
		if order.Data.Status == "CANCELED" {
			order.Data.ClientId = order.Data.ClientIdOrig
		}
		ch <- &SpotOrder{
			Symbol:      order.Data.Symbol,
			OrderId:     strconv.FormatInt(order.Data.OrderId, 10),
			ClientId:    order.Data.ClientId,
			Price:       order.Data.Price,
			Qty:         order.Data.Qty,
			FilledQty:   order.Data.ExecutedQty,
			FilledAmt:   order.Data.CummQuoteQty,
			Status:      order.Data.Status,
			Type:        order.Data.OrderType,
			TimeInForce: order.Data.TimeInForce,
			Side:        order.Data.Side,
			FeeQty:      order.Data.FeeQty.Neg(), // 换成负数
			FeeAsset:    order.Data.FeeAsset,
			CTime:       order.Data.CreateTime,
			UTime:       order.Data.ExecTime,
		}
	}
}
func (bn *Binance) wsSpotHandlePlaceOrderResp(reqId, errS string,
	data json.RawMessage, ch chan<- any) {
	if errS != "" {
		order := &SpotOrder{
			RequestId: reqId,
			Err:       errS,
		}
		ch <- order
		return
	}

	ret := struct {
		Symbol   string `json:"symbol,omitempty"`
		OrderId  int64  `json:"orderId,omitempty"`
		ClientId string `json:"clientOrderId,omitempty"`
	}{}
	if err := json.Unmarshal(data, &ret); err != nil {
		ilog.Error(bn.Name() + " spot.ws.priv handle place order resp: " + err.Error())
		return
	}
	ch <- &SpotOrder{
		RequestId: reqId,
		OrderId:   strconv.FormatInt(ret.OrderId, 10),
		ClientId:  ret.ClientId,
	}
}
func (bn *Binance) wsSpotHandleCancelOrderResp(errS string) {
	if errS != "" {
		ilog.Error(bn.Name() + " spot cancel order fail! " + errS)
	}
}

// priv ws api
func (bn *Binance) SpotWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {
	if bn.SpotWsPrivateIsClosed() {
		return "", errors.New(bn.Name() + " spot.ws.priv ws closed")
	}
	params := map[string]any{
		"apiKey":     bn.apikey,
		"timestamp":  time.Now().UnixMilli(),
		"recvWindow": 3000,

		"symbol":           symbol,
		"newClientOrderId": cltId,
		"newOrderRespType": "ACK",
		"side":             side,
		"type":             orderType,
	}
	if orderType == "LIMIT" {
		params["quantity"] = qty.String()
		params["price"] = price.String()
		params["timeInForce"] = timeInForce
	} else if orderType == "MARKET" {
		if side == "BUY" {
			params["quoteOrderQty"] = qty.String()
		} else {
			params["quantity"] = qty.String()
		}
	}
	params["signature"] = bn.spotWsSign(params)
	req := BnWsApiArg{Id: "sord-" + gutils.RandomStr(16), Method: "order.place", Params: params}
	reqJson, _ := json.Marshal(req)

	bn.spotWsPrivateConnMtx.Lock()
	defer bn.spotWsPrivateConnMtx.Unlock()
	if err := bn.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(bn.Name() + " spot.ws.priv send fail: " + err.Error())
	}
	return req.Id, nil
}
func (bn *Binance) SpotWsCancelOrder(symbol, orderId, cltId string) error {
	if bn.SpotWsPrivateIsClosed() {
		return errors.New(bn.Name() + " spot.ws.priv closed")
	}
	params := map[string]any{
		"apiKey":     bn.apikey,
		"timestamp":  time.Now().UnixMilli(),
		"recvWindow": 3000,

		"symbol": symbol,
	}
	ordId, _ := strconv.ParseInt(orderId, 10, 64)
	if ordId > 0 {
		params["orderId"] = ordId
	} else if cltId != "" {
		params["origClientOrderId"] = cltId
	} else {
		return errors.New("orderId or clientId is empty")
	}
	params["signature"] = bn.spotWsSign(params)
	req := BnWsApiArg{Id: "scle-" + gutils.RandomStr(16), Method: "order.cancel", Params: params}
	reqJson, _ := json.Marshal(req)

	bn.spotWsPrivateConnMtx.Lock()
	defer bn.spotWsPrivateConnMtx.Unlock()
	if err := bn.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return errors.New(bn.Name() + " spot.ws.priv send fail: " + err.Error())
	}
	return nil
}
func (bn *Binance) spotWsSign(kv map[string]any) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for i, key := range keys {
		val := kv[key]
		valStr := ""
		switch v := val.(type) {
		case string:
			valStr = v
		case int:
			valStr = strconv.FormatInt(int64(v), 10)
		case int64:
			valStr = strconv.FormatInt(v, 10)
		default:
			ilog.Error(bn.Name() + " spotWsSign unsupported type")
		}
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(key)
		buf.WriteByte('=')
		buf.WriteString(valStr)
	}
	return bn.sign(buf.String())
}
