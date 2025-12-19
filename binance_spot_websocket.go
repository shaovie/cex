package cex

import (
	"encoding/json"
	"errors"
	"fmt"
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

var (
	bnSpotWsPrivMsgPool              sync.Pool
	bnSpotWsPublicOrderBookInnerPool sync.Pool
	bnSpotWsPublicBBOInnerPool       sync.Pool
	bnSpotWsPublicTickerInnerPool    sync.Pool
)

func init() {
	bnSpotWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &BnSpotWsPrivMsg{}
		},
	}
	bnSpotWsPublicOrderBookInnerPool = sync.Pool{
		New: func() any {
			return &BinanceSpotOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
	bnSpotWsPublicBBOInnerPool = sync.Pool{
		New: func() any {
			return &BinanceSpotBBO{}
		},
	}
	bnSpotWsPublicTickerInnerPool = sync.Pool{
		New: func() any {
			return &BinanceSpot24hTicker{}
		},
	}
}
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
		} else if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@bookTicker")
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
		} else if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@bookTicker")
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
func (bn *Binance) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (bn *Binance) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (bn *Binance) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (bn *Binance) SpotWsPublicLoop(ch chan<- any) {
	defer bn.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 26 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.spotWsPublicConn.SetPongHandler(func(string) error {
		bn.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.SpotWsPublicIsClosed() {
				break
			}
			bn.spotWsPublicConnMtx.Lock()
			bn.spotWsPublicConn.WriteMessage(websocket.PingMessage, nil)
			bn.spotWsPublicConnMtx.Unlock()
		}
	}()

	l := 0
	for {
		_, recv, err := bn.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bn.SpotWsPublicIsClosed() { // 并非主动断开
				ilog.Warning(bn.Name() + " spot.ws.public read: " + err.Error())
			}
			break
		}
		msg := bnWsPubMsgPool.Get().(*BinanceWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
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
		} else if l > 11 && msg.Stream[l-11:l] == "@bookTicker" {
			bn.spotWsHandleBBO(msg.Data, ch)
		} else if l > 11 && msg.Stream[l-11:l] == "@miniTicker" {
			bn.spotWsHandle24hTickers(msg.Data, ch)
		} else {
			if strings.Index(string(recv), `"result":null`) == -1 {
				ilog.Error(bn.Name() + " spot.ws.public recv unknown msg: " + string(recv))
			}
		}
	END:
		bnWsPubMsgPool.Put(msg)
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
	depth := bnSpotWsPublicOrderBookInnerPool.Get().(*BinanceSpotOrderBook)
	defer bnSpotWsPublicOrderBookInnerPool.Put(depth)
	depth.Bids = depth.Bids[:0]
	depth.Asks = depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		if len(depth.Bids) != len(depth.Asks) {
			ilog.Error(bn.Name() + " spot.ws.public " + symbol + " orderbook5 exception")
			return
		}
		obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
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
func (bn *Binance) spotWsHandleBBO(data json.RawMessage, ch chan<- any) {
	bbo := bnSpotWsPublicBBOInnerPool.Get().(*BinanceSpotBBO)
	defer bnSpotWsPublicBBOInnerPool.Put(bbo)
	if err := easyjson.Unmarshal(data, bbo); err == nil {
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		obd.Symbol = bbo.Symbol
		obd.Time = 0 // 币安不提供
		obd.BidPrice = bbo.BidPrice
		obd.BidQty = bbo.BidQty
		obd.AskPrice = bbo.AskPrice
		obd.AskQty = bbo.AskQty
		ch <- obd
	}
}
func (bn *Binance) spotWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	ticker := bnSpotWsPublicTickerInnerPool.Get().(*BinanceSpot24hTicker)
	defer bnSpotWsPublicTickerInnerPool.Put(ticker)
	if err := json.Unmarshal(data, ticker); err == nil {
		tk := wsPublicTickerPool.Get().(*Pub24hTicker)
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

type BnSpotWsPrivMsg struct {
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

func (v *BnSpotWsPrivMsg) reset() {
	v.Id = ""
	v.Status = 0
	v.Err.Code = 0
	v.Err.Msg = ""
	v.Result = nil
	v.Data.Event = ""
	v.Data.Time = 0
}
func (bn *Binance) SpotWsPrivateLoop(ch chan<- any) {
	defer bn.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 24 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.spotWsPrivateConn.SetPongHandler(func(string) error {
		bn.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.SpotWsPrivateIsClosed() {
				break
			}
			bn.spotWsPrivateConnMtx.Lock()
			bn.spotWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
			bn.spotWsPrivateConnMtx.Unlock()
		}
	}()

	for {
		_, recv, err := bn.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !bn.SpotWsPrivateIsClosed() {
				ilog.Warning(bn.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		ilog.Rinfo("spot.ws.priv " + string(recv))
		msg := bnSpotWsPrivMsgPool.Get().(*BnSpotWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(bn.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Status == 0 {
			if msg.Data.Event == "executionReport" { // data
				bn.spotWsHandleOrder(recv, ch)
			} else if msg.Data.Event == "balanceUpdate" { // data
			} else if msg.Data.Event == "outboundAccountPosition" {
				bn.spotWsHandleBalanceUpdate(recv, ch)
			} else if msg.Data.Event == "eventStreamTerminated" { // will be closed
			} else {
				ilog.Error(bn.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		} else { // ws api
			if len(msg.Id) > 5 && msg.Id[0:5] == "sord-" {
				bn.spotWsHandlePlaceOrderResp(msg.Id, msg.Err.Msg, msg.Result, ch)
			} else if len(msg.Id) > 5 && msg.Id[0:5] == "scle-" {
				bn.spotWsHandleCancelOrderResp(msg.Err.Msg)
			} else if len(msg.Id) == 0 {
				ilog.Error(bn.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		}
	END:
		bnSpotWsPrivMsgPool.Put(msg)
	}
}
func (bn *Binance) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
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
func (bn *Binance) spotWsHandleBalanceUpdate(data json.RawMessage, ch chan<- any) {
	msg := struct {
		Event struct {
			B []struct {
				Symbol  string          `json:"a"` // symbol
				Balance decimal.Decimal `json:"f"`
				Locked  decimal.Decimal `json:"l"`
			} `json:"B,omitempty"`
		} `json:"event,omitempty"`
	}{}
	if err := json.Unmarshal(data, &msg); err == nil && len(msg.Event.B) > 0 {
		for _, as := range msg.Event.B {
			ch <- &SpotAsset{
				Symbol: as.Symbol,
				Avail:  as.Balance,
				Locked: as.Locked,
				Total:  as.Balance.Add(as.Locked),
			}
		}
	}
}
func (bn *Binance) spotWsHandlePlaceOrderResp(reqId, errS string,
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
func (bn *Binance) spotWsHandleCancelOrderResp(errS string) {
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
	params["signature"] = bn.wsSign(params)
	req := BnWsApiArg{Id: "sord-" + gutils.RandomStr(16), Method: "order.place", Params: params}
	reqJson, _ := json.Marshal(req)

	bn.spotWsPrivateConnMtx.Lock()
	defer bn.spotWsPrivateConnMtx.Unlock()
	if err := bn.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(bn.Name() + " spot.ws.priv send fail: " + err.Error())
	}
	return req.Id, nil
}
func (bn *Binance) SpotWsCancelOrder(symbol, orderId, cltId string) (string, error) {
	if bn.SpotWsPrivateIsClosed() {
		return "", errors.New(bn.Name() + " spot.ws.priv closed")
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
		return "", errors.New("orderId or clientId is empty")
	}
	params["signature"] = bn.wsSign(params)
	req := BnWsApiArg{Id: "scle-" + gutils.RandomStr(16), Method: "order.cancel", Params: params}
	reqJson, _ := json.Marshal(req)

	bn.spotWsPrivateConnMtx.Lock()
	defer bn.spotWsPrivateConnMtx.Unlock()
	if err := bn.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(bn.Name() + " spot.ws.priv send fail: " + err.Error())
	}
	return req.Id, nil
}
