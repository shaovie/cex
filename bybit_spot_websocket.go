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
	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

var (
	bbSpotWsPublicOrderBookInnerPool sync.Pool
	bbSpotWsPublicBBOInnerPool       sync.Pool
	bbSpotWsPublicTickerInnerPool    sync.Pool
)

func init() {
	bbSpotWsPublicOrderBookInnerPool = sync.Pool{
		New: func() any {
			return &BybitOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 1),
				Asks: make([][2]decimal.Decimal, 0, 1),
			}
		},
	}
	bbSpotWsPublicBBOInnerPool = sync.Pool{
		New: func() any {
			return &BybitSpotBBO{}
		},
	}
	bbSpotWsPublicTickerInnerPool = sync.Pool{
		New: func() any {
			return &BybitSpot24hTicker{}
		},
	}
}
func (bb *Bybit) SpotWsPublicOpen() error {
	url := "wss://stream.bybit.com/v5/public/spot"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bb.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bb.Name() + " spot.ws.public con failed! " + err.Error())
	}
	bb.spotWsPublicClosedMtx.Lock()
	bb.spotWsPublicClosed = false
	bb.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (bb *Bybit) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BbSubscribeArg{Op: "subscribe"}
	arg.Id = "sub-" + gutils.RandomStr(8)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.SplitSeq(arr[1], ",")
				for sym := range symbolArr {
					arg.Args = append(arg.Args, "orderbook.1."+strings.ToUpper(sym))
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.SplitSeq(arr[1], ",")
				for sym := range symbolArr {
					arg.Args = append(arg.Args, "tickers."+strings.ToUpper(sym))
				}
			}
		}
	}
	if len(arg.Args) > 0 {
		req, _ := json.Marshal(&arg)
		bb.spotWsPublicConnMtx.Lock()
		bb.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bb.spotWsPublicConnMtx.Unlock()
	}
}
func (bb *Bybit) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BbSubscribeArg{Op: "unsubscribe"}
	arg.Id = "sub-" + gutils.RandomStr(8)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.SplitSeq(arr[1], ",")
				for sym := range symbolArr {
					arg.Args = append(arg.Args, "orderbook.1."+strings.ToUpper(sym))
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.SplitSeq(arr[1], ",")
				for sym := range symbolArr {
					arg.Args = append(arg.Args, "tickers."+strings.ToUpper(sym))
				}
			}
		}
	}
	if len(arg.Args) > 0 {
		req, _ := json.Marshal(&arg)
		bb.spotWsPublicConnMtx.Lock()
		bb.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bb.spotWsPublicConnMtx.Unlock()
	}
}
func (bb *Bybit) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (bb *Bybit) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (bb *Bybit) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (bb *Bybit) SpotWsPublicLoop(ch chan<- any) {
	defer bb.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 27 * time.Second
	pongWait := pingInterval + 2*time.Second
	bb.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		ping := `{"op":"ping"}`
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if bb.SpotWsPublicIsClosed() {
					break
				}
				bb.spotWsPublicConnMtx.Lock()
				bb.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(ping))
				bb.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	l := 0
	for {
		_, recv, err := bb.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bb.SpotWsPublicIsClosed() { // 并非主动断开
				ilog.Warning(bb.Name() + " spot.ws.public read: " + err.Error())
			}
			break
		}
		msg := bbWsPubMsgPool.Get().(*BybitWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bb.Name() + " spot.ws.public invalid msg:" + string(recv))
			goto END
		}
		l = len(msg.Topic)
		if l > 12 && msg.Topic[:12] == "orderbook.1." {
			bb.spotWsHandleBBO(msg, ch)
		} else if l > 8 && msg.Topic[:8] == "tickers." {
			bb.spotWsHandle24hTickers(msg, ch)
		} else {
			if msg.Op == "ping" {
				bb.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			} else if msg.Op == "subscribe" || msg.Op == "unsubscribe" { // 订阅的响应
				if strings.Index(string(recv), "false") != -1 {
					ilog.Error(bb.Name() + " spot.ws.public recv subscribe err:" + string(recv))
				}
			}
		}
	END:
		bbWsPubMsgPool.Put(msg)
	}
}
func (bb *Bybit) SpotWsPublicIsClosed() bool {
	bb.spotWsPublicClosedMtx.RLock()
	defer bb.spotWsPublicClosedMtx.RUnlock()
	return bb.spotWsPublicClosed
}
func (bb *Bybit) SpotWsPublicClose() {
	bb.spotWsPublicClosedMtx.Lock()
	defer bb.spotWsPublicClosedMtx.Unlock()
	if bb.spotWsPublicClosed {
		return
	}
	bb.spotWsPublicClosed = true
	bb.spotWsPublicConn.Close()
}
func (bb *Bybit) spotWsHandleOrderBook5(msg *BybitWsPubMsg, ch chan<- any) {
	depth := bbSpotWsPublicOrderBookInnerPool.Get().(*BybitOrderBook)
	defer bbSpotWsPublicOrderBookInnerPool.Put(depth)
	depth.Bids = depth.Bids[:0]
	depth.Asks = depth.Asks[:0]
	if err := easyjson.Unmarshal(msg.Data, depth); err == nil {
		if len(depth.Bids) != len(depth.Asks) {
			ilog.Error(bb.Name() + " spot.ws.public " + msg.Topic + " orderbook5 exception")
			return
		}
		obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
		obd.Symbol = depth.Symbol
		obd.Level = len(depth.Bids)
		obd.Time = msg.Time
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
func (bb *Bybit) spotWsHandleBBO(msg *BybitWsPubMsg, ch chan<- any) {
	bbo := bbSpotWsPublicBBOInnerPool.Get().(*BybitSpotBBO)
	defer bbSpotWsPublicBBOInnerPool.Put(bbo)
	bbo.Bids = bbo.Bids[:0]
	bbo.Asks = bbo.Asks[:0]
	if err := easyjson.Unmarshal(msg.Data, bbo); err == nil &&
		len(bbo.Bids) == 1 && len(bbo.Bids) == len(bbo.Asks) {
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		obd.Symbol = bbo.Symbol
		obd.Time = msg.Time
		obd.BidPrice = bbo.Bids[0][0]
		obd.BidQty = bbo.Bids[0][1]
		obd.AskPrice = bbo.Asks[0][0]
		obd.AskQty = bbo.Asks[0][1]
		ch <- obd
	}
}
func (bb *Bybit) spotWsHandle24hTickers(msg *BybitWsPubMsg, ch chan<- any) {
	ticker := bbSpotWsPublicTickerInnerPool.Get().(*BybitSpot24hTicker)
	defer bbSpotWsPublicTickerInnerPool.Put(ticker)
	if err := easyjson.Unmarshal(msg.Data, ticker); err == nil {
		tk := wsPublicTickerPool.Get().(*Pub24hTicker)
		tk.Symbol = ticker.Symbol
		tk.LastPrice = ticker.Last
		tk.Volume = ticker.Volume
		tk.QuoteVolume = ticker.QuoteVolume
		ch <- tk
	}
}

// = priv channel
func (bb *Bybit) SpotWsPrivateSupported() bool {
	return true
}
func (bb *Bybit) SpotWsPrivateOpen() error {
	url := "wss://stream.bybit.com/v5/private"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bb.spotWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bb.Name() + " connect failed! " + err.Error())
	}
	expires := time.Now().UnixMilli() + 3000
	authMessage := map[string]interface{}{
		"op":   "auth",
		"args": []any{bb.apikey, expires, bb.wsSign(expires)},
	}
	req, _ := json.Marshal(authMessage)
	bb.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	_, msg, err := bb.spotWsPrivateConn.ReadMessage()
	if err != nil {
		bb.SpotWsPrivateClose()
		return errors.New(bb.Name() + " spot.ws.priv recv auth resp err:" + err.Error())
	}
	resp := struct {
		Result bool   `json:"success"`
		Err    string `json:"ret_msg"`
		Op     string `json:"op"`
	}{}
	if err = json.Unmarshal(msg, &resp); err != nil {
		bb.SpotWsPrivateClose()
		return errors.New(bb.Name() + " spot.ws.priv auth resp err:" + err.Error())
	}
	if resp.Op != "auth" || resp.Result != true {
		bb.SpotWsPrivateClose()
		return errors.New(bb.Name() + " spot.ws.priv auth fail:" + string(msg))
	}

	bb.spotWsPrivateClosedMtx.Lock()
	bb.spotWsPrivateClosed = false
	bb.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (bb *Bybit) SpotWsPrivateSubscribe(channels []string) {
	arg := BbSubscribeArg{Op: "subscribe"}
	for _, c := range channels {
		if c == "orders" {
			arg.Args = append(arg.Args, "order.spot")
		} else if c == "balance" {
			arg.Args = append(arg.Args, "wallet")
		}
	}
	if len(arg.Args) > 0 {
		req, _ := json.Marshal(&arg)
		bb.spotWsPrivateConnMtx.Lock()
		if err := bb.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
			ilog.Warning(bb.Name() + " spot.ws.priv subscribe net error! " + err.Error())
		}
		bb.spotWsPrivateConnMtx.Unlock()
	}
}
func (bb *Bybit) SpotWsPrivateIsClosed() bool {
	bb.spotWsPrivateClosedMtx.RLock()
	defer bb.spotWsPrivateClosedMtx.RUnlock()
	return bb.spotWsPrivateClosed
}
func (bb *Bybit) SpotWsPrivateClose() {
	bb.spotWsPrivateClosedMtx.Lock()
	defer bb.spotWsPrivateClosedMtx.Unlock()
	if bb.spotWsPrivateClosed {
		return
	}
	bb.spotWsPrivateClosed = true
	bb.spotWsPrivateConn.Close()
}

type BybitWsPrivMsg struct {
	Op    string          `json:"op"`
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"`
}

func (v *BybitWsPrivMsg) reset() {
	v.Op = ""
	v.Topic = ""
	v.Data = nil
}
func (bb *Bybit) SpotWsPrivateLoop(ch chan<- any) {
	defer bb.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 31 * time.Second
	pongWait := pingInterval + 2*time.Second
	bb.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	bb.spotWsPublicConn.SetPongHandler(func(message string) error {
		bb.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		ping := `{"op":"ping"}`
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if bb.SpotWsPrivateIsClosed() {
					break
				}
				bb.spotWsPrivateConnMtx.Lock()
				bb.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(ping))
				bb.spotWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := bb.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !bb.SpotWsPrivateIsClosed() {
				ilog.Warning(bb.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if bb.debug {
			ilog.Rinfo(bb.Name() + " spot priv ws: " + string(recv))
		}
		msg := bbWsPrivMsgPool.Get().(*BybitWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(bb.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Op == "ping" {
			bb.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		} else if msg.Topic == "wallet" {
			bb.spotWsHandleBalanceUpdate(msg.Data, ch)
		} else if msg.Topic == "order.spot" {
			bb.spotWsHandleOrder(msg.Data, ch)
		} else {
			if msg.Op == "subscribe" { // 订阅的响应
				if strings.Index(string(recv), "false") != -1 {
					ilog.Error(bb.Name() + " spot.ws.priv recv subscribe err:" + string(recv))
				}
			}
		}
	END:
		bbWsPrivMsgPool.Put(msg)
	}
}
func (bb *Bybit) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
	orders := []struct {
		Symbol       string            `json:"symbol"` // BTCUSDT
		OrderId      string            `json:"orderId"`
		ClientId     string            `json:"orderLinkId"`
		Price        decimal.Decimal   `json:"price"`
		Quantity     decimal.Decimal   `json:"qty"`         // 用户设置的原始订单数量
		Type         string            `json:"orderType"`   // LIMIT/MARKET
		TimeInForce  string            `json:"timeInForce"` // GTC/FOK/IOC
		Side         string            `json:"side"`
		ExecutedQty  decimal.Decimal   `json:"cumExecQty"`   // 交易的订单数量
		CummQuoteQty decimal.Decimal   `json:"cumExecValue"` // 累计交易的金额
		FeeQty       decimal.Decimal   `json:"cumExecFee"`
		Status       string            `json:"orderStatus"`
		Time         string            `json:"createdTime"` // msec
		UTime        string            `json:"updatedTime"` // msec
		FeeDetail    map[string]string `json:"cumFeeDetail"`
	}{}
	if err := json.Unmarshal(data, &orders); err == nil && len(orders) > 0 {
		for i := range orders {
			o := &SpotOrder{
				Symbol:      orders[i].Symbol,
				OrderId:     orders[i].OrderId,
				ClientId:    orders[i].ClientId,
				Price:       orders[i].Price,
				Qty:         orders[i].Quantity,
				FilledQty:   orders[i].ExecutedQty,
				FilledAmt:   orders[i].CummQuoteQty,
				Status:      bb.toStdOrderStatus(orders[i].Status),
				Type:        bb.toStdOrderType(orders[i].Type),
				TimeInForce: orders[i].TimeInForce,
				Side:        bb.toStdSide(orders[i].Side),
			}
			o.CTime, _ = strconv.ParseInt(orders[i].Time, 10, 64)
			o.UTime, _ = strconv.ParseInt(orders[i].UTime, 10, 64)
			for k, v := range orders[i].FeeDetail {
				o.FeeAsset = k
				o.FeeQty, _ = decimal.NewFromString(v)
				o.FeeQty = o.FeeQty.Neg() // 换成负数
				break
			}
			ch <- o
		}
	}
}
func (bb *Bybit) spotWsHandleBalanceUpdate(data json.RawMessage, ch chan<- any) {
	bls := []struct {
		Coin struct {
			Symbol string          `json:"coin"`
			Total  decimal.Decimal `json:"equity"`
			Avail  decimal.Decimal `json:"walletBalance"`
			Locked decimal.Decimal `json:"locked"`
		} `json:"coin"`
	}{}
	if err := json.Unmarshal(data, &bls); err == nil && len(bls) > 0 {
		for i := range bls {
			ch <- &SpotAsset{
				Symbol: bls[i].Coin.Symbol,
				Avail:  bls[i].Coin.Avail,
				Locked: bls[i].Coin.Locked,
				Total:  bls[i].Coin.Avail.Add(bls[i].Coin.Locked),
			}
		}
	}
}
