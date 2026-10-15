package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ilog"
)

// Aster合约的公共频道: bbo=<symbol>@bookTicker orderbook5=<symbol>@depth5@100ms
// ticker=<symbol>@miniTicker
var (
	asFuturesWsPubMsgPool          sync.Pool
	asFuturesWsPrivMsgPool         sync.Pool
	asFuturesWsPublicOrderBookPool sync.Pool
)

func init() {
	asFuturesWsPubMsgPool = sync.Pool{
		New: func() any {
			return &AsterFuturesWsPubMsg{}
		},
	}
	asFuturesWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &AsterFuturesWsPrivMsg{}
		},
	}
	asFuturesWsPublicOrderBookPool = sync.Pool{
		New: func() any {
			return &AsterFuturesOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
}
func (as *Aster) FuturesWsPublicOpen(typ string) error {
	dialer := as.newWsDialer()
	conn, _, err := dialer.Dial("wss://fstream.asterdex.com/stream", nil)
	if err != nil {
		return errors.New(as.Name() + " futures.ws.public con failed! " + err.Error())
	}
	as.futuresWsPublicConn = conn

	as.futuresWsPublicClosedMtx.Lock()
	as.futuresWsPublicClosed = false
	as.futuresWsPublicClosedMtx.Unlock()
	return nil
}

// futuresWsPublicEvent 转换频道参数, 一个链接最多订阅1024个频道
func (as *Aster) futuresWsPublicEvent(method string, channels []string) {
	if len(channels) == 0 {
		return
	}
	params := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if len(arr) < 2 || len(arr[1]) == 0 {
			continue
		}
		for _, sym := range strings.Split(arr[1], ",") {
			if len(sym) == 0 {
				continue
			}
			sym = strings.ToLower(sym)
			if arr[0] == "bbo" {
				params = append(params, sym+"@bookTicker")
			} else if arr[0] == "orderbook5" {
				params = append(params, sym+"@depth5@100ms")
			} else if arr[0] == "ticker" {
				params = append(params, sym+"@miniTicker")
			}
		}
	}
	if len(params) == 0 {
		return
	}
	arg := AsSubscribeArg{Id: time.Now().UnixNano(), Method: method, Params: params}
	req, _ := json.Marshal(&arg)
	as.futuresWsPublicConnMtx.Lock()
	as.futuresWsPublicConn.WriteMessage(websocket.TextMessage, req)
	as.futuresWsPublicConnMtx.Unlock()
}
func (as *Aster) FuturesWsPublicSubscribe(channels []string) {
	as.futuresWsPublicEvent("SUBSCRIBE", channels)
}
func (as *Aster) FuturesWsPublicUnsubscribe(channels []string) {
	as.futuresWsPublicEvent("UNSUBSCRIBE", channels)
}
func (as *Aster) FuturesWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (as *Aster) FuturesWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (as *Aster) FuturesWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (as *Aster) FuturesWsPublicLoop(ch chan<- any) {
	defer as.FuturesWsPublicClose()
	defer close(ch)

	pingInterval := 29 * time.Second
	pongWait := pingInterval + 2*time.Second
	as.futuresWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	as.futuresWsPublicConn.SetPingHandler(func(message string) error { // 交易所每3分钟发ping
		as.futuresWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		err := as.futuresWsPublicConn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(2*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
	as.futuresWsPublicConn.SetPongHandler(func(string) error {
		as.futuresWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if as.FuturesWsPublicIsClosed() {
					return
				}
				as.futuresWsPublicConnMtx.Lock()
				as.futuresWsPublicConn.WriteMessage(websocket.PingMessage, nil)
				as.futuresWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := as.futuresWsPublicConn.ReadMessage()
		if err != nil {
			if !as.FuturesWsPublicIsClosed() {
				ilog.Warning(as.Name() + " futures.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := asFuturesWsPubMsgPool.Get().(*AsterFuturesWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(as.Name() + " futures.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Code != 0 { // 订阅出错
			ilog.Error(as.Name() + " futures.ws.public recv err:" + strconv.Itoa(msg.Code) +
				" " + msg.Msg + " " + string(recv))
			goto END
		}
		if len(msg.Stream) > 0 { // 订阅/退订响应没有stream
			as.futuresWsHandlePublic(msg.Stream, msg.Data, ch)
		}
	END:
		asFuturesWsPubMsgPool.Put(msg)
	}
}
func (as *Aster) FuturesWsPublicIsClosed() bool {
	as.futuresWsPublicClosedMtx.RLock()
	defer as.futuresWsPublicClosedMtx.RUnlock()
	return as.futuresWsPublicClosed
}
func (as *Aster) FuturesWsPublicClose() {
	as.futuresWsPublicClosedMtx.Lock()
	defer as.futuresWsPublicClosedMtx.Unlock()
	if as.futuresWsPublicClosed {
		return
	}
	as.futuresWsPublicClosed = true
	if as.futuresWsPublicConn != nil {
		as.futuresWsPublicConn.Close()
	}
}
func (as *Aster) futuresWsHandlePublic(stream string, data []byte, ch chan<- any) {
	l := len(stream)
	if l > 11 && stream[l-11:l] == "@bookTicker" {
		as.futuresWsHandleBBO(data, ch)
	} else if l > 13 && stream[l-13:l] == "@depth5@100ms" {
		as.futuresWsHandleOrderBook5(data, ch)
	} else if l > 11 && stream[l-11:l] == "@miniTicker" {
		as.futuresWsHandleTicker(data, ch)
	} else {
		ilog.Error(as.Name() + " futures.ws.public recv unknown stream: " + stream)
	}
}
func (as *Aster) futuresWsHandleBBO(data []byte, ch chan<- any) {
	bboRecv := AsterFuturesBBO{}
	if err := easyjson.Unmarshal(data, &bboRecv); err != nil || len(bboRecv.Symbol) == 0 {
		return
	}
	bbo := wsPublicBBOPool.Get().(*BestBidAsk)
	bbo.Symbol = bboRecv.Symbol
	bbo.Time = bboRecv.Time
	bbo.BidPrice = bboRecv.BidPrice
	bbo.BidQty = bboRecv.BidQty
	bbo.AskPrice = bboRecv.AskPrice
	bbo.AskQty = bboRecv.AskQty
	ch <- bbo
}
func (as *Aster) futuresWsHandleOrderBook5(data []byte, ch chan<- any) {
	depth := asFuturesWsPublicOrderBookPool.Get().(*AsterFuturesOrderBook)
	defer asFuturesWsPublicOrderBookPool.Put(depth)
	depth.reset()
	if err := easyjson.Unmarshal(data, depth); err != nil {
		return
	}
	if len(depth.Bids) == 0 || len(depth.Asks) == 0 {
		return
	}
	obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
	obd.Symbol = depth.Symbol
	obd.Level = len(depth.Bids)
	obd.Time = depth.Time
	obd.Bids = obd.Bids[:0]
	obd.Asks = obd.Asks[:0]
	for _, v := range depth.Bids {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range depth.Asks {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	ch <- obd
}
func (as *Aster) futuresWsHandleTicker(data []byte, ch chan<- any) {
	tkRecv := AsterFutures24hTicker{}
	if err := easyjson.Unmarshal(data, &tkRecv); err != nil || len(tkRecv.Symbol) == 0 {
		return
	}
	tk := wsPublicTickerPool.Get().(*Pub24hTicker)
	tk.Cex = as.Name()
	tk.Symbol = tkRecv.Symbol
	tk.LastPrice = tkRecv.Last
	tk.Volume = tkRecv.Volume
	tk.QuoteVolume = tkRecv.QuoteVolume
	ch <- tk
}

// = priv channel
func (as *Aster) FuturesWsPrivateSupported(typ string) bool {
	return true
}
func (as *Aster) FuturesWsPrivateOpen(typ string) error {
	listenKey, err := as.getFuturesWsPrivateListenKey()
	if err != nil {
		return err
	}
	dialer := as.newWsDialer()
	conn, _, err := dialer.Dial("wss://fstream.asterdex.com/ws/"+listenKey, nil)
	if err != nil {
		return errors.New(as.Name() + " futures.ws.priv con failed! " + err.Error())
	}
	as.futuresWsPrivateConn = conn
	as.futuresWsPrivateListenKey = listenKey

	as.futuresWsPrivateClosedMtx.Lock()
	as.futuresWsPrivateClosed = false
	as.futuresWsPrivateClosedMtx.Unlock()
	return nil
}
func (as *Aster) getFuturesWsPrivateListenKey() (string, error) {
	path := "/fapi/v3/listenKey"
	link := asFuturesEndpoint + path
	_, resp, err := as.Post(link, as.buildBody(""), asApiDeadline, as.buildHeaders())
	if err != nil {
		return "", errors.New(as.Name() + " futures.ws.priv get listenKey net error! " + err.Error())
	}
	ret := struct {
		Code      int    `json:"code,omitempty"`
		Msg       string `json:"msg,omitempty"`
		ListenKey string `json:"listenKey,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(as.Name() + " futures.ws.priv listenKey unmarshal fail! " + err.Error())
	}
	if len(ret.ListenKey) == 0 {
		return "", as.handleExceptionResp("FuturesWsPrivateOpen", resp)
	}
	return ret.ListenKey, nil
}

// extendFuturesWsPrivateListenKey 延长listenKey有效期到60分钟, 建议每30分钟调用一次
func (as *Aster) extendFuturesWsPrivateListenKey(listenKey string) error {
	path := "/fapi/v3/listenKey"
	link := asFuturesEndpoint + path + "?" + as.buildQuery("listenKey="+listenKey)
	_, resp, err := as.Put(link, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " futures.ws.priv extend listenKey net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " futures.ws.priv extend listenKey unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(as.Name() + " futures.ws.priv extend listenKey fail! " + ret.Msg)
	}
	return nil
}
func (as *Aster) FuturesWsPrivateSubscribe(channels []string) {
	// 带上listenKey的连接会自动推送订单和仓位变更, 不需要订阅
}
func (as *Aster) FuturesWsPrivateIsClosed() bool {
	as.futuresWsPrivateClosedMtx.RLock()
	defer as.futuresWsPrivateClosedMtx.RUnlock()
	return as.futuresWsPrivateClosed
}
func (as *Aster) FuturesWsPrivateClose() {
	as.futuresWsPrivateClosedMtx.Lock()
	defer as.futuresWsPrivateClosedMtx.Unlock()
	if as.futuresWsPrivateClosed {
		return
	}
	as.futuresWsPrivateClosed = true
	if as.futuresWsPrivateConn != nil {
		as.futuresWsPrivateConn.Close()
	}
}
func (as *Aster) FuturesWsPrivateLoop(ch chan<- any) {
	defer as.FuturesWsPrivateClose()
	defer close(ch)

	pingInterval := 31 * time.Second
	pongWait := pingInterval + 2*time.Second
	as.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	as.futuresWsPrivateConn.SetPingHandler(func(message string) error {
		as.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		err := as.futuresWsPrivateConn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(2*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
	as.futuresWsPrivateConn.SetPongHandler(func(string) error {
		as.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if as.FuturesWsPrivateIsClosed() {
					return
				}
				as.futuresWsPrivateConnMtx.Lock()
				as.futuresWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
				as.futuresWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	// listenKey 60分钟过期, 每30分钟延长一次
	listenKeyExit := make(chan struct{})
	defer close(listenKeyExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if as.FuturesWsPrivateIsClosed() {
					return
				}
				if err := as.extendFuturesWsPrivateListenKey(as.futuresWsPrivateListenKey); err != nil {
					ilog.Warning(as.Name() + " futures.ws.priv extend listenKey fail: " + err.Error())
				}
			}
		}
	}(listenKeyExit)

	for {
		_, recv, err := as.futuresWsPrivateConn.ReadMessage()
		if err != nil {
			if !as.FuturesWsPrivateIsClosed() {
				ilog.Warning(as.Name() + " futures.ws.priv channel read: " + err.Error())
			}
			break
		}
		if as.debug {
			ilog.Rinfo(as.Name() + " futures priv ws: " + string(recv))
		}
		msg := asFuturesWsPrivMsgPool.Get().(*AsterFuturesWsPrivMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(as.Name() + " futures.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "ORDER_TRADE_UPDATE" {
			as.futuresWsHandleOrder(msg.Order, ch)
		} else if msg.Event == "ACCOUNT_UPDATE" {
			as.futuresWsHandlePosition(msg.Account, ch, msg.Time)
		} else if msg.Event == "listenKeyExpired" {
			ilog.Warning(as.Name() + " futures.ws.priv listenKey expired!")
			break
		} else if msg.Event == "MARGIN_CALL" || msg.Event == "ACCOUNT_CONFIG_UPDATE" {
		} else {
			ilog.Error(as.Name() + " futures.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		asFuturesWsPrivMsgPool.Put(msg)
	}
}
func (as *Aster) futuresWsHandleOrder(data []byte, ch chan<- any) {
	orderRecv := AsterFuturesWsOrder{}
	if err := easyjson.Unmarshal(data, &orderRecv); err != nil || orderRecv.OrderId <= 0 {
		return
	}
	var ctime int64 // 只有新订单事件带下单时间
	if orderRecv.Status == "NEW" {
		ctime = orderRecv.TradeTime
	}
	ch <- &FuturesOrder{
		Symbol:         orderRecv.Symbol,
		OrderId:        strconv.FormatInt(orderRecv.OrderId, 10),
		ClientId:       orderRecv.ClientId,
		Price:          orderRecv.Price,
		Qty:            orderRecv.Qty,
		FilledQty:      orderRecv.ExecutedQty,
		FilledAmt:      orderRecv.ExecutedQty.Mul(orderRecv.AvgPrice),
		AvgPrice:       orderRecv.AvgPrice,
		Status:         as.toStdOrderStatus(orderRecv.Status),
		Type:           as.toStdOrderType(orderRecv.OrderType),
		Side:           as.toStdSide(orderRecv.Side),
		FeeQty:         orderRecv.FeeQty.Neg(), // 换成负数
		FeeAsset:       orderRecv.FeeAsset,
		RealizedProfit: orderRecv.Rpnl,
		CTime:          ctime,
		UTime:          orderRecv.TradeTime,
	}
}
func (as *Aster) futuresWsHandlePosition(data []byte, ch chan<- any, t int64) {
	pl := struct {
		Balances []struct {
			Symbol string          `json:"a,omitempty"`
			Free   decimal.Decimal `json:"wb"` // 钱包余额
		} `json:"B,omitempty"`
		Pos []struct {
			Symbol           string          `json:"s,omitempty"`
			EntryPrice       decimal.Decimal `json:"ep"`
			UnRealizedProfit decimal.Decimal `json:"up"`
			Qty              decimal.Decimal `json:"pa"`
			PositionMode     string          `json:"ps,omitempty"`
		} `json:"P,omitempty"`
	}{}
	if err := json.Unmarshal(data, &pl); err != nil {
		return
	}
	for _, p := range pl.Pos {
		side := "SELL"
		if p.PositionMode == "BOTH" { // 单仓模式
			if p.Qty.IsPositive() {
				side = "BUY"
			}
		} else {
			side = as.toStdSide(p.PositionMode)
		}
		ch <- &FuturesPosition{
			Symbol:           p.Symbol,
			Side:             side,
			PositionQty:      p.Qty.Abs(),
			EntryPrice:       p.EntryPrice,
			UnRealizedProfit: p.UnRealizedProfit,
			LiqPrice:         decimal.NewFromFloat(-1), // 不支持
			UTime:            t,
		}
	}
	for _, b := range pl.Balances {
		ch <- &FuturesAsset{
			Symbol:            b.Symbol,
			Total:             b.Free,
			Avail:             decimal.NewFromInt(-999999999),
			MaxWithdrawAmount: decimal.NewFromInt(-999999999),
		}
	}
}
