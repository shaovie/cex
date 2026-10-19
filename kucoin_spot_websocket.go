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
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

var (
	kcWsPubMsgPool  sync.Pool
	kcWsPrivMsgPool sync.Pool
)

func init() {
	kcWsPubMsgPool = sync.Pool{
		New: func() any {
			return &KucoinWsMsg{}
		},
	}
	kcWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &KucoinWsMsg{}
		},
	}
}
func (kc *Kucoin) newWsDialer() websocket.Dialer {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if kc.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(kc.localIP),
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
func (kc *Kucoin) SpotWsPublicOpen() error {
	dialer := kc.newWsDialer()
	conn, _, err := dialer.Dial("wss://x-push-spot.kucoin.com", nil)
	if err != nil {
		return errors.New(kc.Name() + " spot.ws.public con failed! " + err.Error())
	}
	kc.spotWsPublicConn = conn

	kc.spotWsPublicClosedMtx.Lock()
	kc.spotWsPublicClosed = false
	kc.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (kc *Kucoin) spotWsPublicEvent(action string, channels []string) {
	if len(channels) == 0 {
		return
	}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if len(arr) < 2 || len(arr[1]) == 0 {
			continue
		}
		channel := ""
		if arr[0] == "bbo" { // 用obu实现
			channel = "obu"
		} else if arr[0] == "ticker" {
			channel = "ticker"
		} else {
			continue
		}
		for _, v := range strings.Split(arr[1], ",") {
			if sym := kc.getSpotSymbol(v); sym != "" {
				id := strconv.FormatInt(time.Now().UnixNano(), 10)
				req := `{"id":"` + id + `","action":"` + action + `","channel":"` + channel +
					`","tradeType":"SPOT"`
				if channel == "obu" {
					req += `,"depth":"1"`
				}
				req += `,"symbol":"` + sym + `"}`
				kc.spotWsPublicConnMtx.Lock()
				kc.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
				kc.spotWsPublicConnMtx.Unlock()
			}
		}
	}
}
func (kc *Kucoin) SpotWsPublicSubscribe(channels []string) {
	kc.spotWsPublicEvent("subscribe", channels)
}
func (kc *Kucoin) SpotWsPublicUnsubscribe(channels []string) {
	kc.spotWsPublicEvent("unsubscribe", channels)
}
func (kc *Kucoin) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (kc *Kucoin) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (kc *Kucoin) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (kc *Kucoin) SpotWsPublicLoop(ch chan<- any) {
	defer kc.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 10*time.Second
	kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		pingMsg := []byte(`{"type":"ping"}`)
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if kc.SpotWsPublicIsClosed() {
					return
				}
				kc.spotWsPublicConnMtx.Lock()
				kc.spotWsPublicConn.WriteMessage(websocket.TextMessage, pingMsg)
				kc.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := kc.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !kc.SpotWsPublicIsClosed() {
				ilog.Warning(kc.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := kcWsPubMsgPool.Get().(*KucoinWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(kc.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Channel == "obu.SPOT" {
			kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			if msg.DataType == "snapshot" {
				kc.spotWsHandleBBO(msg.Data, ch)
			}
		} else if msg.Channel == "ticker.SPOT" {
			kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			kc.spotWsHandleTicker(msg.Data, ch)
		} else if msg.Type == "pong" {
			kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		}
	END:
		kcWsPubMsgPool.Put(msg)
	}
}
func (kc *Kucoin) SpotWsPublicIsClosed() bool {
	kc.spotWsPublicClosedMtx.RLock()
	defer kc.spotWsPublicClosedMtx.RUnlock()
	return kc.spotWsPublicClosed
}
func (kc *Kucoin) SpotWsPublicClose() {
	kc.spotWsPublicClosedMtx.Lock()
	defer kc.spotWsPublicClosedMtx.Unlock()
	if kc.spotWsPublicClosed {
		return
	}
	kc.spotWsPublicClosed = true
	if kc.spotWsPublicConn != nil {
		kc.spotWsPublicConn.Close()
	}
}
func (kc *Kucoin) spotWsHandleBBO(data []byte, ch chan<- any) {
	bbo := KucoinTicker{}
	if err := easyjson.Unmarshal(data, &bbo); err == nil && len(bbo.Bids) > 0 && len(bbo.Asks) > 0 {
		symbol := strings.ReplaceAll(bbo.Symbol, "-", "")
		if !kc.spotWsBboAllow(symbol) { // n毫秒内的BBO丢弃
			return
		}
		bidPrice, err := decimal.NewFromString(bbo.Bids[0][0])
		if err != nil {
			return
		}
		bidQty, err := decimal.NewFromString(bbo.Bids[0][1])
		if err != nil {
			return
		}
		askPrice, err := decimal.NewFromString(bbo.Asks[0][0])
		if err != nil {
			return
		}
		askQty, err := decimal.NewFromString(bbo.Asks[0][1])
		if err != nil {
			return
		}
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		obd.Symbol = symbol
		obd.Time = bbo.Time
		obd.BidPrice = bidPrice
		obd.BidQty = bidQty
		obd.AskPrice = askPrice
		obd.AskQty = askQty
		ch <- obd
	}
}
func (kc *Kucoin) spotWsHandleTicker(data []byte, ch chan<- any) {
	tk := KucoinWsTicker{}
	if err := easyjson.Unmarshal(data, &tk); err != nil {
		return
	}
	pub := wsPublicTickerPool.Get().(*Pub24hTicker)
	pub.Cex = kc.Name()
	pub.Symbol = strings.ReplaceAll(tk.Symbol, "-", "")
	pub.LastPrice = tk.Last
	pub.Volume = tk.LastQty
	ch <- pub
}

// = priv channel
func (kc *Kucoin) SpotWsPrivateSupported() bool {
	return true
}
func (kc *Kucoin) SpotWsPrivateOpen() error {
	// 取ws token
	path := "/api/v2/bullet-private"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Post(link, nil, kcApiDeadline, kc.buildHeaders("POST", path, "", ""))
	if err != nil {
		return errors.New(kc.Name() + " spot.ws.priv get token net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(kc.Name() + " spot.ws.priv token unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" || len(ret.Data.Token) == 0 {
		return errors.New(kc.Name() + " spot.ws.priv get token fail! " + ret.Code + " " + ret.Msg)
	}

	wsUrl := "wss://wsapi-push.kucoin.com?token=" + ret.Data.Token +
		"&connectId=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	dialer := kc.newWsDialer()
	conn, _, err := dialer.Dial(wsUrl, nil)
	if err != nil {
		return errors.New(kc.Name() + " spot.ws.priv con failed! " + err.Error())
	}
	kc.spotWsPrivateConn = conn

	kc.spotWsPrivateClosedMtx.Lock()
	kc.spotWsPrivateClosed = false
	kc.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (kc *Kucoin) SpotWsPrivateSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	for _, c := range channels {
		id := strconv.FormatInt(time.Now().UnixNano(), 10)
		var req string
		if c == "orders" {
			req = `{"id":"` + id + `","action":"subscribe","channel":"orderAll","tradeType":"UNIFIED"}`
		} else if c == "balance" {
			req = `{"id":"` + id + `","action":"subscribe","channel":"balance","accountType":"UNIFIED"}`
		} else {
			continue
		}
		kc.spotWsPrivateConnMtx.Lock()
		if err := kc.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req)); err != nil {
			ilog.Warning(kc.Name() + " spot.ws.priv subscribe net error! " + err.Error())
		}
		kc.spotWsPrivateConnMtx.Unlock()
	}
}
func (kc *Kucoin) SpotWsPrivateIsClosed() bool {
	kc.spotWsPrivateClosedMtx.RLock()
	defer kc.spotWsPrivateClosedMtx.RUnlock()
	return kc.spotWsPrivateClosed
}
func (kc *Kucoin) SpotWsPrivateClose() {
	kc.spotWsPrivateClosedMtx.Lock()
	defer kc.spotWsPrivateClosedMtx.Unlock()
	if kc.spotWsPrivateClosed {
		return
	}
	kc.spotWsPrivateClosed = true
	if kc.spotWsPrivateConn != nil {
		kc.spotWsPrivateConn.Close()
	}
}
func (kc *Kucoin) SpotWsPrivateLoop(ch chan<- any) {
	defer kc.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 20 * time.Second
	pongWait := pingInterval + 10*time.Second
	kc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if kc.SpotWsPrivateIsClosed() {
					return
				}
				id := strconv.FormatInt(time.Now().UnixNano(), 10)
				pingMsg := []byte(`{"id":"` + id + `","type":"ping"}`)
				kc.spotWsPrivateConnMtx.Lock()
				kc.spotWsPrivateConn.WriteMessage(websocket.TextMessage, pingMsg)
				kc.spotWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := kc.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !kc.SpotWsPrivateIsClosed() {
				ilog.Warning(kc.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if kc.debug {
			ilog.Rinfo(kc.Name() + " spot priv ws: " + string(recv))
		}
		msg := kcWsPrivMsgPool.Get().(*KucoinWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(kc.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Channel == "orderAll.UNIFIED" || msg.Channel == "order.UNIFIED" {
			kc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			kc.spotWsHandleOrder(msg.Data, ch)
		} else if strings.HasPrefix(msg.Channel, "balance.") {
			kc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			kc.spotWsHandleBalance(msg.Data, ch)
		} else if msg.Type == "pong" {
			kc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		} else if msg.Message == "welcome" { // 连接欢迎消息
			if msg.PingInterval > 0 {
				kc.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			}
		}
	END:
		kcWsPrivMsgPool.Put(msg)
	}
}
func (kc *Kucoin) spotWsHandleOrder(data []byte, ch chan<- any) {
	order := KucoinWsOrder{}
	if err := easyjson.Unmarshal(data, &order); err != nil {
		return
	}
	obd := &SpotOrder{
		Symbol:      strings.ReplaceAll(order.Symbol, "-", ""),
		OrderId:     strconv.FormatInt(order.OrderId, 10),
		ClientId:    order.ClientOid,
		Price:       order.toDecimal(order.Price),
		Qty:         order.toDecimal(order.Size),
		FilledQty:   order.toDecimal(order.FilledSize),
		AvgPrice:    order.toDecimal(order.AvgPrice),
		FilledAmt:   order.toDecimal(order.FilledSize).Mul(order.toDecimal(order.AvgPrice)),
		Status:      kc.toStdOrderStatus(order.Status),
		Type:        kc.toStdOrderType(order.OrderType),
		TimeInForce: kc.toStdTimeInForce(order.TimeInForce),
		Side:        kc.toStdSide(order.Side),
		FeeQty:      order.toDecimal(order.Fee).Neg(),
		FeeAsset:    order.FeeCurrency,
		UTime:       order.UTime / 1000000, // ns -> msec
	}
	ch <- obd
}
func (kc *Kucoin) spotWsHandleBalance(data []byte, ch chan<- any) {
	balance := KucoinWsBalance{}
	if err := easyjson.Unmarshal(data, &balance); err != nil {
		return
	}
	ch <- &SpotAsset{
		Symbol: balance.Currency,
		Avail:  balance.Available,
		Locked: balance.Hold,
		Total:  balance.Balance,
	}
}
