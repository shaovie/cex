package cex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	//"net/url"
	"net"
	"strings"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func (sa *Safetrade) SpotWsPublicOpen() error {
	sa.spotWsPublicClosedMtx.Lock()
	sa.spotWsPublicClosed = false
	sa.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (sa *Safetrade) SpotWsPublicOpen_() error {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if sa.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(sa.localIP),
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
	header := http.Header{}
	header.Set("Origin", "https://safe.trade") // 服务端校验Origin
	header.Set("User-Agent", stUserAgent)

	url := "wss://safe.trade/api/v2/websocket/public"
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return errors.New(sa.Name() + " spot.ws.public con failed! " + err.Error())
	}
	sa.spotWsPublicConn = conn

	sa.spotWsPublicClosedMtx.Lock()
	sa.spotWsPublicClosed = false
	sa.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (sa *Safetrade) stWsPublicEvent_(event string, channels []string) {
	if len(channels) == 0 {
		return
	}
	streams := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" { // 用depth实现
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			for _, v := range strings.Split(arr[1], ",") {
				if id := sa.getSpotSymbol(v); id != "" {
					if event == "subscribe" { // 订阅时先取快照打底, 避免在WS读循环里取数把读循环卡住
						sa.spotWsLoadDepth(v, id)
					}
					streams = append(streams, id+".depth")
				}
			}
		}
	}
	if len(streams) == 0 {
		return
	}
	req, _ := json.Marshal(struct {
		Event   string   `json:"event"`
		Streams []string `json:"streams"`
	}{Event: event, Streams: streams})
	sa.spotWsPublicConnMtx.Lock()
	sa.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
	sa.spotWsPublicConnMtx.Unlock()
}
func (sa *Safetrade) stWsPublicEvent(event string, channels []string) {
	if len(channels) == 0 {
		return
	}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" { // 用depth实现
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			for _, v := range strings.Split(arr[1], ",") {
				sa.spotWsPublicConnMtx.Lock()
				if event == "subscribe" {
					sa.spotWsPublicBBOStreams[v] = true
				} else if event == "unsubscribe" {
					delete(sa.spotWsPublicBBOStreams, v)
				}
				sa.spotWsPublicConnMtx.Unlock()
			}
		}
	}
}
func (sa *Safetrade) SpotWsPublicSubscribe(channels []string) {
	sa.stWsPublicEvent("subscribe", channels)
}
func (sa *Safetrade) SpotWsPublicUnsubscribe(channels []string) {
	sa.stWsPublicEvent("unsubscribe", channels)
}
func (sa *Safetrade) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (sa *Safetrade) SpotWsPublicLoop(ch chan<- any) {
	defer close(ch)

	exitChan := make(chan struct{})
	defer close(exitChan)

	streams := make(map[string]bool, 4)
	for {
		sa.spotWsPublicConnMtx.Lock()
		for sym, _ := range sa.spotWsPublicBBOStreams {
			if streams[sym] == false {
				streams[sym] = true
				go sa.spotWsBBOLoop(sym, ch, exitChan)
			}
		}
		sa.spotWsPublicConnMtx.Unlock()

		time.Sleep(1 * time.Second)
		if sa.SpotWsPublicIsClosed() {
			break
		}
	}
}
func (sa *Safetrade) spotWsBBOLoop(symbol string, ch chan<- any, exitChan chan struct{}) {
	var bba BestBidAsk
	var interval time.Duration
	interval = time.Duration(1500 + rand.Int64()%200)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-exitChan:
			return
		case <-ticker.C:
			sa.spotWsPublicConnMtx.Lock()
			if sa.spotWsPublicBBOStreams[symbol] == false {
				sa.spotWsPublicConnMtx.Unlock()
				return
			}
			sa.spotWsPublicConnMtx.Unlock()

			bba, _ = sa.SpotGetBBO(symbol)
			if bba.BidPrice.IsZero() {
				time.Sleep(2000 * time.Millisecond)
				break // jump out of select
			}
			obd := wsPublicBBOPool.Get().(*BestBidAsk)
			obd.Symbol = symbol
			obd.Time = 0 // SafeTrade不提供
			obd.BidPrice = bba.BidPrice
			obd.BidQty = bba.BidQty
			obd.AskPrice = bba.AskPrice
			obd.AskQty = bba.AskQty
			ch <- obd
		}
	}
}
func (sa *Safetrade) SpotWsPublicLoop_(ch chan<- any) {
	defer sa.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 10*time.Second
	sa.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	sa.spotWsPublicConn.SetPongHandler(func(string) error {
		sa.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if sa.SpotWsPublicIsClosed() {
					return
				}
				sa.spotWsPublicConnMtx.Lock()
				sa.spotWsPublicConn.WriteMessage(websocket.PingMessage, nil)
				sa.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	msg := make(map[string]WsRawJSON, 2) // 循环外分配一次, 之后复用
	for {
		_, recv, err := sa.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !sa.SpotWsPublicIsClosed() {
				ilog.Warning(sa.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}

		for k := range msg { // 清空后复用, 避免每条消息都make一次map
			delete(msg, k)
		}
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(sa.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			continue
		}
		for k, v := range msg {
			if strings.HasSuffix(k, ".depth") { // 如 ethbtc.depth
				sa.spotWsHandleDepth(strings.ToUpper(strings.TrimSuffix(k, ".depth")), v, ch)
			} else if k == "success" { // 订阅/取消订阅的回执
				if !bytes.Contains(v, []byte("subscribe")) {
					ilog.Error(sa.Name() + " spot.ws.public recv: " + string(v))
				}
			}
		}
	}
}
func (sa *Safetrade) SpotWsPublicIsClosed() bool {
	sa.spotWsPublicClosedMtx.RLock()
	defer sa.spotWsPublicClosedMtx.RUnlock()
	return sa.spotWsPublicClosed
}
func (sa *Safetrade) SpotWsPublicClose() {
	sa.spotWsPublicClosedMtx.Lock()
	defer sa.spotWsPublicClosedMtx.Unlock()
	if sa.spotWsPublicClosed {
		return
	}
	sa.spotWsPublicClosed = true
	if sa.spotWsPublicConn != nil {
		sa.spotWsPublicConn.Close()
	}
}

// spotWsLoadDepth 取一次盘口快照作为本地底仓(depth频道只推增量)
func (sa *Safetrade) spotWsLoadDepth(symbol, id string) {
	_, resp, err := sa.Get(stEndpoint+"/trade/public/markets/"+id+"/depth?limit=20",
		stApiDeadline, stHttpHeaders)
	if err != nil {
		return
	}
	recv := struct {
		Asks [][2]decimal.Decimal `json:"asks"`
		Bids [][2]decimal.Decimal `json:"bids"`
	}{}
	if json.Unmarshal(resp, &recv) != nil {
		return
	}
	bids := treemap.NewWith[decimal.Decimal, decimal.Decimal](func(a, b decimal.Decimal) int {
		return b.Compare(a) // desc
	})
	asks := treemap.NewWith[decimal.Decimal, decimal.Decimal](func(a, b decimal.Decimal) int {
		return a.Compare(b) // asc
	})
	for _, v := range recv.Bids {
		if !v[1].IsZero() {
			bids.Put(v[0], v[1])
		}
	}
	for _, v := range recv.Asks {
		if !v[1].IsZero() {
			asks.Put(v[0], v[1])
		}
	}
	sa.spotWsPublicConnMtx.Lock()
	sa.spotWsOrderBookBids[symbol] = bids
	sa.spotWsOrderBookAsks[symbol] = asks
	sa.spotWsPublicConnMtx.Unlock()
}
func (sa *Safetrade) spotWsHandleDepth(symbol string, data []byte, ch chan<- any) {
	recv := SafetradeDepth{}
	if err := easyjson.Unmarshal(data, &recv); err != nil {
		ilog.Error(sa.Name() + " spot.ws.public invalid depth msg:" + string(data))
		return
	}
	var bidPrice, bidQty, askPrice, askQty decimal.Decimal
	sa.spotWsPublicConnMtx.Lock()
	bids, asks := sa.spotWsOrderBookBids[symbol], sa.spotWsOrderBookAsks[symbol]
	if bids != nil && asks != nil {
		for _, v := range recv.Bids { // 数量为0表示该价位已撤销
			if v[1].IsZero() {
				bids.Remove(v[0])
			} else {
				bids.Put(v[0], v[1])
			}
		}
		for _, v := range recv.Asks {
			if v[1].IsZero() {
				asks.Remove(v[0])
			} else {
				asks.Put(v[0], v[1])
			}
		}
		if k, v, ok := bids.Min(); ok { // bids降序, Min即最优买价
			bidPrice, bidQty = k, v
		}
		if k, v, ok := asks.Min(); ok { // asks升序, Min即最优卖价
			askPrice, askQty = k, v
		}
	}
	sa.spotWsPublicConnMtx.Unlock()
	if !bidPrice.IsPositive() || !askPrice.IsPositive() {
		return
	}
	obd := wsPublicBBOPool.Get().(*BestBidAsk)
	obd.Symbol = symbol
	obd.Time = 0 // SafeTrade不提供
	obd.BidPrice = bidPrice
	obd.BidQty = bidQty
	obd.AskPrice = askPrice
	obd.AskQty = askQty
	ch <- obd
}
