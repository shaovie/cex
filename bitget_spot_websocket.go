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
)

var (
	bgWsMsgPool sync.Pool
)

func init() {
	bgWsMsgPool = sync.Pool{
		New: func() any {
			return &BitgetWsMsg{}
		},
	}
}
func (bg *Bitget) newWsDialer() websocket.Dialer {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if bg.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(bg.localIP),
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
func (bg *Bitget) SpotWsPublicOpen() error {
	dialer := bg.newWsDialer()
	conn, _, err := dialer.Dial("wss://ws.bitget.com/v3/ws/public", nil)
	if err != nil {
		return errors.New(bg.Name() + " spot.ws.public con failed! " + err.Error())
	}
	bg.spotWsPublicConn = conn

	bg.spotWsPublicClosedMtx.Lock()
	bg.spotWsPublicClosed = false
	bg.spotWsPublicClosedMtx.Unlock()
	return nil
}

// wsPubArg 统一账户公共频道参数: instType=spot
func (bg *Bitget) wsPubArg(topic, symbols string) []BitgetWsArg {
	symbolArr := strings.Split(symbols, ",")
	argL := make([]BitgetWsArg, 0, len(symbolArr))
	for _, v := range symbolArr {
		if len(v) == 0 {
			continue
		}
		argL = append(argL, BitgetWsArg{
			InstType: "spot",
			Topic:    topic,
			Symbol:   v,
		})
	}
	return argL
}
func (bg *Bitget) spotWsPublicEvent(event string, channels []string) {
	if len(channels) == 0 {
		return
	}
	argL := make([]BitgetWsArg, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if len(arr) < 2 || len(arr[1]) == 0 {
			continue
		}
		if arr[0] == "bbo" {
			argL = append(argL, bg.wsPubArg("books1", arr[1])...)
		} else if arr[0] == "orderbook5" {
			argL = append(argL, bg.wsPubArg("books5", arr[1])...)
		} else if arr[0] == "ticker" {
			argL = append(argL, bg.wsPubArg("ticker", arr[1])...)
		} else if arr[0] == "trades" {
			argL = append(argL, bg.wsPubArg("trade", arr[1])...)
		}
	}
	if len(argL) == 0 {
		return
	}
	req, _ := json.Marshal(struct {
		Op   string        `json:"op"`
		Args []BitgetWsArg `json:"args"`
	}{Op: event, Args: argL})
	bg.spotWsPublicConnMtx.Lock()
	bg.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
	bg.spotWsPublicConnMtx.Unlock()
}
func (bg *Bitget) SpotWsPublicSubscribe(channels []string) {
	bg.spotWsPublicEvent("subscribe", channels)
}
func (bg *Bitget) SpotWsPublicUnsubscribe(channels []string) {
	bg.spotWsPublicEvent("unsubscribe", channels)
}
func (bg *Bitget) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (bg *Bitget) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (bg *Bitget) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (bg *Bitget) SpotWsPublicTradePoolPut(v any) {
	wsPublicTradePool.Put(v)
}
func (bg *Bitget) SpotWsPublicLoop(ch chan<- any) {
	defer bg.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 35 * time.Second
	pongWait := pingInterval + 2*time.Second
	bg.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if bg.SpotWsPublicIsClosed() {
					return
				}
				bg.spotWsPublicConnMtx.Lock()
				bg.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte("ping"))
				bg.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := bg.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bg.SpotWsPublicIsClosed() {
				ilog.Warning(bg.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		if len(recv) == 4 && string(recv) == "pong" { // 心跳响应
			bg.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		msg := bgWsMsgPool.Get().(*BitgetWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bg.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "error" {
			ilog.Error(bg.Name() + " spot.ws.public recv err:" + msg.Code + " " + msg.Msg + " " + string(recv))
		} else if msg.Arg.Topic == "books1" {
			bg.spotWsHandleBBO(msg.Arg.Symbol, msg.Data, ch)
		} else if msg.Arg.Topic == "books5" {
			bg.spotWsHandleOrderBook(msg.Arg.Symbol, msg.Data, ch)
		} else if msg.Arg.Topic == "ticker" {
			bg.spotWsHandle24hTicker(msg.Arg.Symbol, msg.Data, ch)
		} else if msg.Arg.Topic == "trade" || msg.Arg.Topic == "publicTrade" {
			bg.spotWsHandleTrade(msg.Arg.Symbol, msg.Data, ch)
		}
	END:
		bgWsMsgPool.Put(msg)
	}
}
func (bg *Bitget) SpotWsPublicIsClosed() bool {
	bg.spotWsPublicClosedMtx.RLock()
	defer bg.spotWsPublicClosedMtx.RUnlock()
	return bg.spotWsPublicClosed
}
func (bg *Bitget) SpotWsPublicClose() {
	bg.spotWsPublicClosedMtx.Lock()
	defer bg.spotWsPublicClosedMtx.Unlock()
	if bg.spotWsPublicClosed {
		return
	}
	bg.spotWsPublicClosed = true
	if bg.spotWsPublicConn != nil {
		bg.spotWsPublicConn.Close()
	}
}
func (bg *Bitget) spotWsHandleBBO(symbol string, data []byte, ch chan<- any) {
	depth := []BitgetWsOrderBook{}
	if err := json.Unmarshal(data, &depth); err != nil || len(depth) == 0 {
		return
	}
	if len(depth[0].Bids) == 0 || len(depth[0].Asks) == 0 {
		return
	}
	ts, _ := strconv.ParseInt(depth[0].Ts, 10, 64)
	if !bg.spotWsBboAllow(strings.ToUpper(symbol)) { // n毫秒内的BBO丢弃
		return
	}
	obd := wsPublicBBOPool.Get().(*BestBidAsk)
	obd.Symbol = strings.ToUpper(symbol)
	obd.Time = ts
	obd.BidPrice = depth[0].Bids[0][0]
	obd.BidQty = depth[0].Bids[0][1]
	obd.AskPrice = depth[0].Asks[0][0]
	obd.AskQty = depth[0].Asks[0][1]
	ch <- obd
}
func (bg *Bitget) spotWsHandleOrderBook(symbol string, data []byte, ch chan<- any) {
	depth := []BitgetWsOrderBook{}
	if err := json.Unmarshal(data, &depth); err != nil || len(depth) == 0 {
		return
	}
	if len(depth[0].Bids) == 0 || len(depth[0].Asks) == 0 {
		return
	}
	ts, _ := strconv.ParseInt(depth[0].Ts, 10, 64)
	obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
	obd.Symbol = strings.ToUpper(symbol)
	obd.Level = len(depth[0].Bids)
	obd.Time = ts
	obd.Bids = obd.Bids[:0]
	obd.Asks = obd.Asks[:0]
	for _, v := range depth[0].Bids {
		obd.Bids = append(obd.Bids, Ticker{Price: v[0], Quantity: v[1]})
	}
	for _, v := range depth[0].Asks {
		obd.Asks = append(obd.Asks, Ticker{Price: v[0], Quantity: v[1]})
	}
	ch <- obd
}
func (bg *Bitget) spotWsHandle24hTicker(symbol string, data []byte, ch chan<- any) {
	tickers := []BitgetSpotTicker{}
	if err := json.Unmarshal(data, &tickers); err != nil || len(tickers) == 0 {
		return
	}
	tk := wsPublicTickerPool.Get().(*Pub24hTicker)
	tk.Cex = bg.Name()
	tk.Symbol = strings.ToUpper(symbol)
	tk.LastPrice = tickers[0].toDecimal(tickers[0].LastPrice)
	tk.Volume = tickers[0].toDecimal(tickers[0].BaseVolume)
	tk.QuoteVolume = tickers[0].toDecimal(tickers[0].QuoteVolume)
	ch <- tk
}
func (bg *Bitget) spotWsHandleTrade(symbol string, data []byte, ch chan<- any) {
	trades := []BitgetWsTrade{}
	if err := json.Unmarshal(data, &trades); err != nil || len(trades) == 0 {
		return
	}
	for _, v := range trades {
		ts, _ := strconv.ParseInt(v.Ts, 10, 64)
		tr := wsPublicTradePool.Get().(*PublicTrade)
		tr.Symbol = strings.ToUpper(symbol)
		tr.Time = ts
		tr.Price = v.Price
		tr.Qty = v.Size
		ch <- tr
	}
}

// = priv channel
func (bg *Bitget) SpotWsPrivateSupported() bool {
	return true
}
func (bg *Bitget) SpotWsPrivateOpen() error {
	dialer := bg.newWsDialer()
	conn, _, err := dialer.Dial("wss://ws.bitget.com/v3/ws/private", nil)
	if err != nil {
		return errors.New(bg.Name() + " spot.ws.priv con failed! " + err.Error())
	}
	bg.spotWsPrivateConn = conn

	type loginArg struct {
		Apikey     string `json:"apiKey"`
		Passphrase string `json:"passphrase"`
		Timestamp  string `json:"timestamp"`
		Sign       string `json:"sign"`
	}
	req, _ := json.Marshal(struct {
		Op   string     `json:"op"`
		Args []loginArg `json:"args"`
	}{
		Op: "login",
		Args: []loginArg{
			{
				Apikey:     bg.apikey,
				Passphrase: bg.passphrase,
				Timestamp:  strconv.FormatInt(time.Now().Unix(), 10),
				Sign:       bg.wsLoginSign(),
			},
		},
	})
	bg.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	_, msg, err := bg.spotWsPrivateConn.ReadMessage()
	if err != nil {
		bg.SpotWsPrivateClose()
		return errors.New(bg.Name() + " spot.ws.priv recv login resp err:" + err.Error())
	}
	resp := struct {
		Event string `json:"event,omitempty"`
		Code  string `json:"code,omitempty"`
		Msg   string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(msg, &resp); err != nil {
		bg.SpotWsPrivateClose()
		return errors.New(bg.Name() + " spot.ws.priv login resp err:" + err.Error())
	}
	if resp.Event != "login" || (resp.Code != "" && resp.Code != "0") {
		bg.SpotWsPrivateClose()
		return errors.New(bg.Name() + " spot.ws.priv login fail:" + string(msg))
	}

	bg.spotWsPrivateClosedMtx.Lock()
	bg.spotWsPrivateClosed = false
	bg.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (bg *Bitget) SpotWsPrivateSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	argL := make([]BitgetWsArg, 0, len(channels))
	for _, c := range channels {
		if c == "orders" {
			argL = append(argL, BitgetWsArg{InstType: "UTA", Topic: "order"})
		} else if c == "balance" {
			argL = append(argL, BitgetWsArg{InstType: "UTA", Topic: "account"})
		}
	}
	if len(argL) == 0 {
		return
	}
	req, _ := json.Marshal(struct {
		Op   string        `json:"op"`
		Args []BitgetWsArg `json:"args"`
	}{Op: "subscribe", Args: argL})
	bg.spotWsPrivateConnMtx.Lock()
	if err := bg.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
		ilog.Warning(bg.Name() + " spot.ws.priv subscribe net error! " + err.Error())
	}
	bg.spotWsPrivateConnMtx.Unlock()
}
func (bg *Bitget) SpotWsPrivateIsClosed() bool {
	bg.spotWsPrivateClosedMtx.RLock()
	defer bg.spotWsPrivateClosedMtx.RUnlock()
	return bg.spotWsPrivateClosed
}
func (bg *Bitget) SpotWsPrivateClose() {
	bg.spotWsPrivateClosedMtx.Lock()
	defer bg.spotWsPrivateClosedMtx.Unlock()
	if bg.spotWsPrivateClosed {
		return
	}
	bg.spotWsPrivateClosed = true
	if bg.spotWsPrivateConn != nil {
		bg.spotWsPrivateConn.Close()
	}
}
func (bg *Bitget) SpotWsPrivateLoop(ch chan<- any) {
	defer bg.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 38 * time.Second
	pongWait := pingInterval + 2*time.Second
	bg.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if bg.SpotWsPrivateIsClosed() {
					return
				}
				bg.spotWsPrivateConnMtx.Lock()
				bg.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte("ping"))
				bg.spotWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := bg.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !bg.SpotWsPrivateIsClosed() {
				ilog.Warning(bg.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if bg.debug {
			ilog.Rinfo(bg.Name() + " spot priv ws: " + string(recv))
		}
		if len(recv) == 4 && string(recv) == "pong" {
			bg.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		msg := bgWsMsgPool.Get().(*BitgetWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bg.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "error" {
			ilog.Error(bg.Name() + " spot.ws.priv recv err:" + msg.Code + " " + msg.Msg + " " + string(recv))
		} else if msg.Arg.Topic == "order" {
			bg.spotWsHandleOrder(msg.Data, ch)
		} else if msg.Arg.Topic == "account" {
			bg.spotWsHandleAsset(msg.Data, ch)
		}
	END:
		bgWsMsgPool.Put(msg)
	}
}
func (bg *Bitget) spotWsHandleOrder(data []byte, ch chan<- any) {
	orders := []BitgetSpotOrder{}
	if err := json.Unmarshal(data, &orders); err != nil {
		return
	}
	for i := range orders {
		ch <- bg.toStdOrder(&orders[i])
	}
}
func (bg *Bitget) spotWsHandleAsset(data []byte, ch chan<- any) {
	accounts := []BitgetWsAccount{}
	if err := json.Unmarshal(data, &accounts); err != nil {
		return
	}
	for _, ac := range accounts {
		for _, as := range ac.Coin {
			ch <- &SpotAsset{
				Symbol: as.Coin,
				Avail:  as.Available,
				Locked: as.Locked,
				Total:  as.Balance,
			}
		}
	}
}
