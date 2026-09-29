package cex

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func (sa *Safetrade) SpotWsPublicOpen() error {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	header := http.Header{}
	header.Set("Origin", stWsOrigin) // 服务端校验Origin
	header.Set("User-Agent", stUserAgent)

	//= 调试用: 走本机代理, 上线请删除以下2行
	proxyURL, _ := url.Parse("http://127.0.0.1:7890")
	dialer.Proxy = http.ProxyURL(proxyURL)
	//= 调试用 end

	url := "wss://safetrade.com/api/v2/websocket/public"
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
func (sa *Safetrade) stWsPublicEvent(event string, channels []string) {
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

	for {
		_, recv, err := sa.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !sa.SpotWsPublicIsClosed() {
				ilog.Warning(sa.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}

		msg := make(map[string]json.RawMessage, 2)
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(sa.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			continue
		}
		for k, v := range msg {
			if strings.HasSuffix(k, ".depth") { // 如 ethbtc.depth
				sa.spotWsHandleDepth(strings.ToUpper(strings.TrimSuffix(k, ".depth")), v, ch)
			} else if k == "success" { // 订阅/取消订阅的回执
				ret := struct {
					Message string `json:"message"`
				}{}
				if json.Unmarshal(v, &ret) == nil && strings.Index(ret.Message, "subscribe") == -1 {
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
	sa.spotWsPublicConn.Close()
}

// spotWsLoadDepth 取一次盘口快照作为本地底仓(depth频道只推增量)
func (sa *Safetrade) spotWsLoadDepth(symbol, id string) {
	_, resp, err := sa.Get(stEndpoint+"/trade/public/markets/"+id+"/depth?limit=5",
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
func (sa *Safetrade) spotWsHandleDepth(symbol string, data json.RawMessage, ch chan<- any) {
	recv := struct {
		Asks [][2]decimal.Decimal `json:"asks"`
		Bids [][2]decimal.Decimal `json:"bids"`
	}{}
	if err := json.Unmarshal(data, &recv); err != nil {
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
