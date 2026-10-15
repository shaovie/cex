package cex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ilog"
)

// MEXC行情推送的频道: 只有.pb频道可以订阅成功, 报文是protobuf
// bbo:       spot@public.aggre.bookTicker.v3.api.pb@100ms@BTCUSDT
// orderbook5: spot@public.limit.depth.v3.api.pb@BTCUSDT@5
// trades:    spot@public.aggre.deals.v3.api.pb@100ms@BTCUSDT
var (
	mcWsPingMsg = []byte(`{"method":"PING"}`)
	mcWsPong    = []byte("PONG")
	mcWsBlocked = []byte("Blocked")
)

func (mc *Mexc) newWsDialer() websocket.Dialer {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if mc.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(mc.localIP),
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
func (mc *Mexc) SpotWsPublicOpen() error {
	dialer := mc.newWsDialer()
	conn, _, err := dialer.Dial("wss://wbs-api.mexc.com/ws", nil)
	if err != nil {
		return errors.New(mc.Name() + " spot.ws.public con failed! " + err.Error())
	}
	mc.spotWsPublicConn = conn

	mc.spotWsPublicClosedMtx.Lock()
	mc.spotWsPublicClosed = false
	mc.spotWsPublicClosedMtx.Unlock()
	return nil
}

// spotWsPublicEvent 转换频道参数, 一个链接最多订阅30个频道, 超过会被交易所忽略
func (mc *Mexc) spotWsPublicEvent(method string, channels []string) {
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
			if arr[0] == "bbo" {
				params = append(params, "spot@public.aggre.bookTicker.v3.api.pb@100ms@"+sym)
			} else if arr[0] == "orderbook5" {
				params = append(params, "spot@public.limit.depth.v3.api.pb@"+sym+"@5")
			} else if arr[0] == "trades" {
				params = append(params, "spot@public.aggre.deals.v3.api.pb@100ms@"+sym)
			}
		}
	}
	if len(params) == 0 {
		return
	}
	if len(params) > 30 {
		ilog.Warning(mc.Name() + " spot.ws.public channels > 30, exchange ignore this event!")
	}
	req, _ := json.Marshal(McSubscribeArg{Method: method, Params: params})
	mc.spotWsPublicConnMtx.Lock()
	mc.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
	mc.spotWsPublicConnMtx.Unlock()
}
func (mc *Mexc) SpotWsPublicSubscribe(channels []string) {
	mc.spotWsPublicEvent("SUBSCRIPTION", channels)
}
func (mc *Mexc) SpotWsPublicUnsubscribe(channels []string) {
	mc.spotWsPublicEvent("UNSUBSCRIPTION", channels)
}
func (mc *Mexc) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (mc *Mexc) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (mc *Mexc) SpotWsPublicTradePoolPut(v any) {
	wsPublicTradePool.Put(v)
}
func (mc *Mexc) SpotWsPublicLoop(ch chan<- any) {
	defer mc.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 20 * time.Second
	pongWait := pingInterval + 10*time.Second
	mc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if mc.SpotWsPublicIsClosed() {
					return
				}
				mc.spotWsPublicConnMtx.Lock()
				mc.spotWsPublicConn.WriteMessage(websocket.TextMessage, mcWsPingMsg)
				mc.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := mc.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !mc.SpotWsPublicIsClosed() {
				ilog.Warning(mc.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		if len(recv) == 0 {
			continue
		}
		if recv[0] == '{' { // 订阅响应/PONG
			if bytes.Contains(recv, mcWsPong) {
				mc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			} else if bytes.Contains(recv, mcWsBlocked) {
				ilog.Warning(mc.Name() + " spot.ws.public subscribe fail: " + string(recv))
			}
			continue
		}
		mc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		mc.spotWsHandlePublic(recv, ch)
	}
}
func (mc *Mexc) SpotWsPublicIsClosed() bool {
	mc.spotWsPublicClosedMtx.RLock()
	defer mc.spotWsPublicClosedMtx.RUnlock()
	return mc.spotWsPublicClosed
}
func (mc *Mexc) SpotWsPublicClose() {
	mc.spotWsPublicClosedMtx.Lock()
	defer mc.spotWsPublicClosedMtx.Unlock()
	if mc.spotWsPublicClosed {
		return
	}
	mc.spotWsPublicClosed = true
	if mc.spotWsPublicConn != nil {
		mc.spotWsPublicConn.Close()
	}
}
func (mc *Mexc) spotWsHandlePublic(recv []byte, ch chan<- any) {
	bodyField, symbol, sendTime, body := mcWsDecode(recv)
	if bodyField == 315 { // PublicAggreBookTickerV3Api
		mc.spotWsHandleBBO(symbol, sendTime, body, ch)
	} else if bodyField == 303 { // PublicLimitDepthsV3Api
		mc.spotWsHandleOrderBook5(symbol, sendTime, body, ch)
	} else if bodyField == 314 { // PublicAggreDealsV3Api
		mc.spotWsHandleTrade(symbol, sendTime, body, ch)
	}
}
func (mc *Mexc) spotWsHandleBBO(symbol string, sendTime int64, data []byte, ch chan<- any) {
	var bidPrice, bidQty, askPrice, askQty string
	mcPbEach(data, func(num, wire int, val []byte) {
		if num == 1 {
			bidPrice = string(val)
		} else if num == 2 {
			bidQty = string(val)
		} else if num == 3 {
			askPrice = string(val)
		} else if num == 4 {
			askQty = string(val)
		}
	})
	if len(bidPrice) == 0 || len(askPrice) == 0 {
		return
	}
	bbo := wsPublicBBOPool.Get().(*BestBidAsk)
	bbo.Symbol = symbol
	bbo.Time = sendTime
	bbo.BidPrice = mcToDecimal(bidPrice)
	bbo.BidQty = mcToDecimal(bidQty)
	bbo.AskPrice = mcToDecimal(askPrice)
	bbo.AskQty = mcToDecimal(askQty)
	ch <- bbo
}
func (mc *Mexc) spotWsHandleOrderBook5(symbol string, sendTime int64, data []byte, ch chan<- any) {
	obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
	obd.Symbol = symbol
	obd.Level = 5
	obd.Time = sendTime
	obd.Bids = obd.Bids[:0]
	obd.Asks = obd.Asks[:0]
	mcPbEach(data, func(num, wire int, val []byte) {
		if num != 1 && num != 2 { // 1 asks 2 bids
			return
		}
		price, qty := mcPbPriceQty(val)
		tk := Ticker{Price: mcToDecimal(price), Quantity: mcToDecimal(qty)}
		if num == 1 {
			obd.Asks = append(obd.Asks, tk)
		} else {
			obd.Bids = append(obd.Bids, tk)
		}
	})
	if len(obd.Bids) == 0 || len(obd.Asks) == 0 {
		wsPublicOrderBook5Pool.Put(obd) // 无效数据, 归还池子
		return
	}
	ch <- obd
}
func (mc *Mexc) spotWsHandleTrade(symbol string, sendTime int64, data []byte, ch chan<- any) {
	mcPbEach(data, func(num, wire int, val []byte) {
		if num != 1 { // 1 deals
			return
		}
		price, qty, tm := mcPbDeal(val)
		if len(price) == 0 {
			return
		}
		if tm == 0 {
			tm = sendTime
		}
		trade := wsPublicTradePool.Get().(*PublicTrade)
		trade.Symbol = symbol
		trade.Time = tm
		trade.Price = mcToDecimal(price)
		trade.Qty = mcToDecimal(qty)
		ch <- trade
	})
}

// = priv channel
func (mc *Mexc) SpotWsPrivateSupported() bool {
	return true
}
func (mc *Mexc) SpotWsPrivateOpen() error {
	listenKey, err := mc.getSpotWsPrivateListenKey()
	if err != nil {
		return err
	}
	dialer := mc.newWsDialer()
	conn, _, err := dialer.Dial("wss://wbs-api.mexc.com/ws?listenKey="+listenKey, nil)
	if err != nil {
		return errors.New(mc.Name() + " spot.ws.priv con failed! " + err.Error())
	}
	mc.spotWsPrivateConn = conn
	mc.spotWsPrivateListenKey = listenKey

	mc.spotWsPrivateClosedMtx.Lock()
	mc.spotWsPrivateClosed = false
	mc.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (mc *Mexc) getSpotWsPrivateListenKey() (string, error) {
	path := "/api/v3/userDataStream"
	link := mcUniEndpoint + path + "?" + mc.buildQuery("")
	_, resp, err := mc.Post(link, nil, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return "", errors.New(mc.Name() + " spot.ws.priv get listenKey net error! " + err.Error())
	}
	ret := struct {
		Code      int    `json:"code,omitempty"`
		Msg       string `json:"msg,omitempty"`
		ListenKey string `json:"listenKey,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(mc.Name() + " spot.ws.priv listenKey unmarshal fail! " + err.Error())
	}
	if len(ret.ListenKey) == 0 {
		return "", errors.New(mc.Name() + " spot.ws.priv get listenKey fail! " + string(resp))
	}
	return ret.ListenKey, nil
}

// extendSpotWsPrivateListenKey 延长listenKey有效期到60分钟, 建议每30分钟调用一次
func (mc *Mexc) extendSpotWsPrivateListenKey(listenKey string) error {
	path := "/api/v3/userDataStream"
	link := mcUniEndpoint + path + "?" + mc.buildQuery("listenKey="+listenKey)
	_, resp, err := mc.Put(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return errors.New(mc.Name() + " spot.ws.priv extend listenKey net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(mc.Name() + " spot.ws.priv extend listenKey unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(mc.Name() + " spot.ws.priv extend listenKey fail! " + ret.Msg)
	}
	return nil
}
func (mc *Mexc) SpotWsPrivateSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	params := make([]string, 0, len(channels))
	for _, c := range channels {
		if c == "orders" {
			params = append(params, "spot@private.orders.v3.api.pb")
		} else if c == "balance" {
			params = append(params, "spot@private.account.v3.api.pb")
		}
	}
	if len(params) == 0 {
		return
	}
	req, _ := json.Marshal(McSubscribeArg{Method: "SUBSCRIPTION", Params: params})
	mc.spotWsPrivateConnMtx.Lock()
	if err := mc.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
		ilog.Warning(mc.Name() + " spot.ws.priv subscribe net error! " + err.Error())
	}
	mc.spotWsPrivateConnMtx.Unlock()
}
func (mc *Mexc) SpotWsPrivateIsClosed() bool {
	mc.spotWsPrivateClosedMtx.RLock()
	defer mc.spotWsPrivateClosedMtx.RUnlock()
	return mc.spotWsPrivateClosed
}
func (mc *Mexc) SpotWsPrivateClose() {
	mc.spotWsPrivateClosedMtx.Lock()
	defer mc.spotWsPrivateClosedMtx.Unlock()
	if mc.spotWsPrivateClosed {
		return
	}
	mc.spotWsPrivateClosed = true
	if mc.spotWsPrivateConn != nil {
		mc.spotWsPrivateConn.Close()
	}
}
func (mc *Mexc) SpotWsPrivateLoop(ch chan<- any) {
	defer mc.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 20 * time.Second
	pongWait := pingInterval + 10*time.Second
	mc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if mc.SpotWsPrivateIsClosed() {
					return
				}
				mc.spotWsPrivateConnMtx.Lock()
				mc.spotWsPrivateConn.WriteMessage(websocket.TextMessage, mcWsPingMsg)
				mc.spotWsPrivateConnMtx.Unlock()
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
				if mc.SpotWsPrivateIsClosed() {
					return
				}
				if err := mc.extendSpotWsPrivateListenKey(mc.spotWsPrivateListenKey); err != nil {
					ilog.Warning(mc.Name() + " spot.ws.priv extend listenKey fail: " + err.Error())
				}
			}
		}
	}(listenKeyExit)

	for {
		_, recv, err := mc.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !mc.SpotWsPrivateIsClosed() {
				ilog.Warning(mc.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if len(recv) == 0 {
			continue
		}
		if recv[0] == '{' { // 订阅响应/PONG
			if bytes.Contains(recv, mcWsPong) {
				mc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			} else if bytes.Contains(recv, mcWsBlocked) {
				ilog.Warning(mc.Name() + " spot.ws.priv subscribe fail: " + string(recv))
			}
			continue
		}
		mc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		mc.spotWsHandlePrivate(recv, ch)
	}
}
func (mc *Mexc) spotWsHandlePrivate(recv []byte, ch chan<- any) {
	bodyField, symbol, sendTime, body := mcWsDecode(recv)
	if bodyField == 304 { // PrivateOrdersV3Api
		mc.spotWsHandleOrder(symbol, sendTime, body, ch)
	} else if bodyField == 307 { // PrivateAccountV3Api
		mc.spotWsHandleAsset(body, ch)
	}
}
func (mc *Mexc) spotWsHandleOrder(symbol string, sendTime int64, data []byte, ch chan<- any) {
	var orderId, clientId, price, qty, avgPrice, cumQty, cumAmt string
	var orderType, tradeType, status int
	var cTime int64
	mcPbEach(data, func(num, wire int, val []byte) {
		if num == 1 {
			orderId = string(val)
		} else if num == 2 {
			clientId = string(val)
		} else if num == 3 {
			price = string(val)
		} else if num == 4 {
			qty = string(val)
		} else if num == 6 {
			avgPrice = string(val)
		} else if num == 7 {
			orderType = int(mcPbU64(val))
		} else if num == 8 {
			tradeType = int(mcPbU64(val))
		} else if num == 13 {
			cumQty = string(val)
		} else if num == 14 {
			cumAmt = string(val)
		} else if num == 15 {
			status = int(mcPbU64(val))
		} else if num == 16 {
			cTime = int64(mcPbU64(val))
		}
	})
	if len(orderId) == 0 && len(clientId) == 0 {
		return
	}
	side := ""
	if tradeType == 1 {
		side = "BUY"
	} else if tradeType == 2 {
		side = "SELL"
	}
	ch <- &SpotOrder{
		Symbol:    symbol,
		OrderId:   orderId,
		ClientId:  clientId,
		Price:     mcToDecimal(price),
		Qty:       mcToDecimal(qty),
		FilledQty: mcToDecimal(cumQty),
		FilledAmt: mcToDecimal(cumAmt),
		AvgPrice:  mcToDecimal(avgPrice),
		Status:    mc.toStdPrivateOrderStatus(status),
		Type:      mc.toStdPrivateOrderType(orderType),
		Side:      side,
		CTime:     cTime,
		UTime:     sendTime,
	}
}
func (mc *Mexc) spotWsHandleAsset(data []byte, ch chan<- any) {
	var coin, balance, frozen string
	mcPbEach(data, func(num, wire int, val []byte) {
		if num == 1 {
			coin = string(val)
		} else if num == 3 {
			balance = string(val)
		} else if num == 5 {
			frozen = string(val)
		}
	})
	if len(coin) == 0 {
		return
	}
	avail := mcToDecimal(balance)
	locked := mcToDecimal(frozen)
	ch <- &SpotAsset{
		Symbol: coin,
		Avail:  avail,
		Locked: locked,
		Total:  avail.Add(locked),
	}
}

// mcToDecimal 字符串转decimal, 失败返回0
func mcToDecimal(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero
	}
	return d
}
