package cex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ilog"
)

// Aster的公共频道: bbo=<symbol>@bookTicker orderbook5=<symbol>@depth5@100ms
// ticker=<symbol>@miniTicker trades=<symbol>@aggTrade
var (
	asSpotWsPubMsgPool          sync.Pool
	asSpotWsPrivMsgPool         sync.Pool
	asSpotWsPublicOrderBookPool sync.Pool
)

func init() {
	asSpotWsPubMsgPool = sync.Pool{
		New: func() any {
			return &AsterSpotWsPubMsg{}
		},
	}
	asSpotWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &AsterSpotWsPrivMsg{}
		},
	}
	asSpotWsPublicOrderBookPool = sync.Pool{
		New: func() any {
			return &AsterSpotOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
}
func (as *Aster) newWsDialer() websocket.Dialer {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if as.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(as.localIP),
			Port: 0, // 0 表示随机可用端口
		}
		dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{
				LocalAddr: localAddr,
				Timeout:   2 * time.Second,
			}
			return d.DialContext(ctx, network, addr)
		}
	}
	return dialer
}
func (as *Aster) SpotWsPublicOpen() error {
	dialer := as.newWsDialer()
	conn, _, err := dialer.Dial("wss://sstream.asterdex.com/stream", nil)
	if err != nil {
		return errors.New(as.Name() + " spot.ws.public con failed! " + err.Error())
	}
	as.spotWsPublicConn = conn

	as.spotWsPublicClosedMtx.Lock()
	as.spotWsPublicClosed = false
	as.spotWsPublicClosedMtx.Unlock()
	return nil
}

// spotWsPublicEvent 转换频道参数, 一个链接最多订阅1024个频道
func (as *Aster) spotWsPublicEvent(method string, channels []string) {
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
			} else if arr[0] == "trades" {
				params = append(params, sym+"@aggTrade")
			}
		}
	}
	if len(params) == 0 {
		return
	}
	arg := AsSubscribeArg{Id: time.Now().UnixNano(), Method: method, Params: params}
	req, _ := json.Marshal(&arg)
	as.spotWsPublicConnMtx.Lock()
	as.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
	as.spotWsPublicConnMtx.Unlock()
}
func (as *Aster) SpotWsPublicSubscribe(channels []string) {
	as.spotWsPublicEvent("SUBSCRIBE", channels)
}
func (as *Aster) SpotWsPublicUnsubscribe(channels []string) {
	as.spotWsPublicEvent("UNSUBSCRIBE", channels)
}
func (as *Aster) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (as *Aster) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (as *Aster) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (as *Aster) SpotWsPublicTradePoolPut(v any) {
	wsPublicTradePool.Put(v)
}
func (as *Aster) SpotWsPublicLoop(ch chan<- any) {
	defer as.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 20 * time.Second
	pongWait := pingInterval + 10*time.Second
	as.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	as.spotWsPublicConn.SetPingHandler(func(message string) error { // 交易所每3分钟发ping
		as.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		err := as.spotWsPublicConn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(2*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
	as.spotWsPublicConn.SetPongHandler(func(string) error {
		as.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if as.SpotWsPublicIsClosed() {
					return
				}
				as.spotWsPublicConnMtx.Lock()
				as.spotWsPublicConn.WriteMessage(websocket.PingMessage, nil)
				as.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := as.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !as.SpotWsPublicIsClosed() {
				ilog.Warning(as.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := asSpotWsPubMsgPool.Get().(*AsterSpotWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(as.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Code != 0 { // 订阅出错
			ilog.Error(as.Name() + " spot.ws.public recv err:" + strconv.Itoa(msg.Code) +
				" " + msg.Msg + " " + string(recv))
			goto END
		}
		if len(msg.Stream) > 0 { // 订阅/退订响应没有stream
			as.spotWsHandlePublic(msg.Stream, msg.Data, ch)
		}
	END:
		asSpotWsPubMsgPool.Put(msg)
	}
}
func (as *Aster) SpotWsPublicIsClosed() bool {
	as.spotWsPublicClosedMtx.RLock()
	defer as.spotWsPublicClosedMtx.RUnlock()
	return as.spotWsPublicClosed
}
func (as *Aster) SpotWsPublicClose() {
	as.spotWsPublicClosedMtx.Lock()
	defer as.spotWsPublicClosedMtx.Unlock()
	if as.spotWsPublicClosed {
		return
	}
	as.spotWsPublicClosed = true
	if as.spotWsPublicConn != nil {
		as.spotWsPublicConn.Close()
	}
}
func (as *Aster) spotWsHandlePublic(stream string, data []byte, ch chan<- any) {
	l := len(stream)
	if l > 11 && stream[l-11:l] == "@bookTicker" {
		as.spotWsHandleBBO(data, ch)
	} else if l > 13 && stream[l-13:l] == "@depth5@100ms" {
		as.spotWsHandleOrderBook5(data, ch)
	} else if l > 11 && stream[l-11:l] == "@miniTicker" {
		as.spotWsHandleTicker(data, ch)
	} else if l > 9 && stream[l-9:l] == "@aggTrade" {
		as.spotWsHandleTrade(data, ch)
	} else {
		ilog.Error(as.Name() + " spot.ws.public recv unknown stream: " + stream)
	}
}
func (as *Aster) spotWsHandleBBO(data []byte, ch chan<- any) {
	bboRecv := AsterSpotBookTicker{}
	if err := easyjson.Unmarshal(data, &bboRecv); err != nil || len(bboRecv.Symbol) == 0 {
		return
	}
	if !as.spotWsBboAllow(bboRecv.Symbol) { // n毫秒内的BBO丢弃
		return
	}
	bidPrice, err := decimal.NewFromString(bboRecv.BidPrice)
	if err != nil {
		return
	}
	bidQty, err := decimal.NewFromString(bboRecv.BidQty)
	if err != nil {
		return
	}
	askPrice, err := decimal.NewFromString(bboRecv.AskPrice)
	if err != nil {
		return
	}
	askQty, err := decimal.NewFromString(bboRecv.AskQty)
	if err != nil {
		return
	}
	bbo := wsPublicBBOPool.Get().(*BestBidAsk)
	bbo.Symbol = bboRecv.Symbol
	bbo.Time = bboRecv.Time
	bbo.BidPrice = bidPrice
	bbo.BidQty = bidQty
	bbo.AskPrice = askPrice
	bbo.AskQty = askQty
	ch <- bbo
}
func (as *Aster) spotWsHandleOrderBook5(data []byte, ch chan<- any) {
	depth := asSpotWsPublicOrderBookPool.Get().(*AsterSpotOrderBook)
	defer asSpotWsPublicOrderBookPool.Put(depth)
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
func (as *Aster) spotWsHandleTicker(data []byte, ch chan<- any) {
	tkRecv := AsterSpot24hTicker{}
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
func (as *Aster) spotWsHandleTrade(data []byte, ch chan<- any) {
	tradeRecv := AsterSpotPublicTrade{}
	if err := easyjson.Unmarshal(data, &tradeRecv); err != nil || len(tradeRecv.Symbol) == 0 {
		return
	}
	trade := wsPublicTradePool.Get().(*PublicTrade)
	trade.Symbol = tradeRecv.Symbol
	trade.Time = tradeRecv.Time
	trade.Price = tradeRecv.Price
	trade.Qty = tradeRecv.Qty
	ch <- trade
}

// = priv channel
func (as *Aster) SpotWsPrivateSupported() bool {
	return true
}
func (as *Aster) SpotWsPrivateOpen() error {
	listenKey, err := as.getSpotWsPrivateListenKey()
	if err != nil {
		return err
	}
	dialer := as.newWsDialer()
	conn, _, err := dialer.Dial("wss://sstream.asterdex.com/ws/"+listenKey, nil)
	if err != nil {
		return errors.New(as.Name() + " spot.ws.priv con failed! " + err.Error())
	}
	as.spotWsPrivateConn = conn
	as.spotWsPrivateListenKey = listenKey

	as.spotWsPrivateClosedMtx.Lock()
	as.spotWsPrivateClosed = false
	as.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (as *Aster) getSpotWsPrivateListenKey() (string, error) {
	path := "/api/v3/listenKey"
	link := asSpotEndpoint + path
	_, resp, err := as.Post(link, as.buildBody(""), asApiDeadline, as.buildHeaders())
	if err != nil {
		return "", errors.New(as.Name() + " spot.ws.priv get listenKey net error! " + err.Error())
	}
	ret := struct {
		Code      int    `json:"code,omitempty"`
		Msg       string `json:"msg,omitempty"`
		ListenKey string `json:"listenKey,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(as.Name() + " spot.ws.priv listenKey unmarshal fail! " + err.Error())
	}
	if len(ret.ListenKey) == 0 {
		return "", as.handleExceptionResp("SpotWsPrivateOpen", resp)
	}
	return ret.ListenKey, nil
}

// extendSpotWsPrivateListenKey 延长listenKey有效期到60分钟, 建议每30分钟调用一次
func (as *Aster) extendSpotWsPrivateListenKey(listenKey string) error {
	path := "/api/v3/listenKey"
	link := asSpotEndpoint + path + "?" + as.buildQuery("listenKey="+listenKey)
	_, resp, err := as.Put(link, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " spot.ws.priv extend listenKey net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " spot.ws.priv extend listenKey unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(as.Name() + " spot.ws.priv extend listenKey fail! " + ret.Msg)
	}
	return nil
}
func (as *Aster) SpotWsPrivateSubscribe(channels []string) {
	// 带上listenKey的连接会自动推送订单和资产变更, 不需要订阅
}
func (as *Aster) SpotWsPrivateIsClosed() bool {
	as.spotWsPrivateClosedMtx.RLock()
	defer as.spotWsPrivateClosedMtx.RUnlock()
	return as.spotWsPrivateClosed
}
func (as *Aster) SpotWsPrivateClose() {
	as.spotWsPrivateClosedMtx.Lock()
	defer as.spotWsPrivateClosedMtx.Unlock()
	if as.spotWsPrivateClosed {
		return
	}
	as.spotWsPrivateClosed = true
	if as.spotWsPrivateConn != nil {
		as.spotWsPrivateConn.Close()
	}
}
func (as *Aster) SpotWsPrivateLoop(ch chan<- any) {
	defer as.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 23 * time.Second
	pongWait := pingInterval + 10*time.Second
	as.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	as.spotWsPrivateConn.SetPingHandler(func(message string) error {
		as.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		err := as.spotWsPrivateConn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(2*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
	as.spotWsPrivateConn.SetPongHandler(func(string) error {
		as.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if as.SpotWsPrivateIsClosed() {
					return
				}
				as.spotWsPrivateConnMtx.Lock()
				as.spotWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
				as.spotWsPrivateConnMtx.Unlock()
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
				if as.SpotWsPrivateIsClosed() {
					return
				}
				if err := as.extendSpotWsPrivateListenKey(as.spotWsPrivateListenKey); err != nil {
					ilog.Warning(as.Name() + " spot.ws.priv extend listenKey fail: " + err.Error())
				}
			}
		}
	}(listenKeyExit)

	for {
		_, recv, err := as.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !as.SpotWsPrivateIsClosed() {
				ilog.Warning(as.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if as.debug {
			ilog.Rinfo(as.Name() + " spot priv ws: " + string(recv))
		}
		msg := asSpotWsPrivMsgPool.Get().(*AsterSpotWsPrivMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(as.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "executionReport" {
			as.spotWsHandleOrder(recv, ch)
		} else if msg.Event == "outboundAccountPosition" {
			as.spotWsHandleBalance(recv, ch)
		} else if msg.Event == "balanceUpdate" || msg.Event == "eventStreamTerminated" {
		} else {
			ilog.Error(as.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		asSpotWsPrivMsgPool.Put(msg)
	}
}
func (as *Aster) spotWsHandleOrder(data []byte, ch chan<- any) {
	orderRecv := AsterSpotWsOrder{}
	if err := easyjson.Unmarshal(data, &orderRecv); err != nil || orderRecv.OrderId <= 0 {
		return
	}
	ch <- &SpotOrder{
		Symbol:      orderRecv.Symbol,
		OrderId:     strconv.FormatInt(orderRecv.OrderId, 10),
		ClientId:    orderRecv.ClientId,
		Price:       orderRecv.Price,
		Qty:         orderRecv.Qty,
		FilledQty:   orderRecv.ExecutedQty,
		FilledAmt:   orderRecv.CummQuoteQty,
		AvgPrice:    orderRecv.AvgPrice,
		Status:      as.toStdOrderStatus(orderRecv.Status),
		Type:        as.toStdOrderType(orderRecv.OrderType),
		TimeInForce: as.toStdTimeInForce(orderRecv.TimeInForce),
		Side:        as.toStdSide(orderRecv.Side),
		FeeQty:      orderRecv.FeeQty.Neg(),
		FeeAsset:    orderRecv.FeeAsset,
		CTime:       orderRecv.CreateTime,
		UTime:       orderRecv.TradeTime,
	}
}
func (as *Aster) spotWsHandleBalance(data []byte, ch chan<- any) {
	balance := AsterSpotWsBalance{}
	if err := easyjson.Unmarshal(data, &balance); err != nil {
		return
	}
	for _, v := range balance.Balances {
		ch <- &SpotAsset{
			Symbol: v.Symbol,
			Avail:  v.Free,
			Locked: v.Locked,
			Total:  v.Free.Add(v.Locked),
		}
	}
}
