package cex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"

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

func (sa *Safetrade) SpotWsPublicOpen_() error {
	sa.spotWsPublicClosedMtx.Lock()
	sa.spotWsPublicClosed = false
	sa.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (sa *Safetrade) SpotWsPublicOpen() error {
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
func (sa *Safetrade) stWsPublicEvent_(event string, channels []string) {
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
func (sa *Safetrade) SpotWsPublicLoop_(ch chan<- any) {
	defer close(ch)

	exitChan := make(chan struct{})
	var bboWg sync.WaitGroup // 等所有BBO协程退出后再close(ch), 否则会往已关闭的channel发送

	streams := make(map[string]bool, 4)
	for {
		sa.spotWsPublicConnMtx.Lock()
		for sym, _ := range sa.spotWsPublicBBOStreams {
			if streams[sym] == false {
				streams[sym] = true
				bboWg.Add(1)
				go func(sym string) {
					defer bboWg.Done()
					sa.spotWsBBOLoop(sym, ch, exitChan)

					sa.spotWsPublicConnMtx.Lock()
					delete(streams, sym)
					sa.spotWsPublicConnMtx.Unlock()
				}(sym)
			}
		}
		sa.spotWsPublicConnMtx.Unlock()

		time.Sleep(1 * time.Second)
		if sa.SpotWsPublicIsClosed() {
			break
		}
	}
	close(exitChan)
	bboWg.Wait()
}

func (sa *Safetrade) spotWsBBOLoop(symbol string, ch chan<- any, exitChan chan struct{}) {
	var bba BestBidAsk
	var interval time.Duration
	// SafeTrade的BBO接口限频1次/秒, 这里加点随机延迟避免同时请求
	interval = time.Duration(4500+rand.Int64()%200) * time.Millisecond
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
				time.Sleep(5000 * time.Millisecond)
				break // jump out of select
			}
			obd := wsPublicBBOPool.Get().(*BestBidAsk)
			obd.Symbol = symbol
			obd.Time = 0 // SafeTrade不提供
			obd.BidPrice = bba.BidPrice
			obd.BidQty = bba.BidQty
			obd.AskPrice = bba.AskPrice
			obd.AskQty = bba.AskQty
			select {
			case ch <- obd:
			case <-exitChan: // 调用方已不再读ch, 退出避免阻塞
				return
			}
		}
	}
}
func (sa *Safetrade) SpotWsPublicLoop(ch chan<- any) {
	defer sa.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 33 * time.Second
	pongWait := pingInterval + 2*time.Second
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

// = priv channel
func (st *Safetrade) SpotWsPrivateSupported() bool {
	return true
}

// SpotWsPrivateOpen 私有wss: 鉴权走X-Auth-*请求头, 订阅order/balance频道后交易所主动推送
func (st *Safetrade) SpotWsPrivateOpen() error {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if st.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(st.localIP),
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
	for k, v := range st.buildHeaders() { // 含X-Auth-Apikey/X-Auth-Nonce/X-Auth-Signature
		header.Set(k, v)
	}
	header.Set("Origin", stWsOrigin) // 服务端校验Origin
	header.Set("User-Agent", stUserAgent)

	conn, _, err := dialer.Dial("wss://safe.trade/api/v2/websocket/private", header)
	if err != nil {
		return errors.New(st.Name() + " spot.ws.priv con failed! " + err.Error())
	}
	st.spotWsPrivateConn = conn

	st.spotWsPrivateClosedMtx.Lock()
	st.spotWsPrivateClosed = false
	st.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (st *Safetrade) SpotWsPrivateSubscribe(channels []string) {
	streams := make([]string, 0, len(channels))
	for _, c := range channels {
		if c == "orders" {
			streams = append(streams, "order")
		} else if c == "trades" {
			streams = append(streams, "trade")
		} else if c == "balance" {
			streams = append(streams, "balance")
		}
	}
	if len(streams) == 0 {
		return
	}
	req, _ := json.Marshal(struct {
		Event   string   `json:"event"`
		Streams []string `json:"streams"`
	}{Event: "subscribe", Streams: streams})
	st.spotWsPrivateConnMtx.Lock()
	st.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	st.spotWsPrivateConnMtx.Unlock()
}
func (st *Safetrade) SpotWsPrivateIsClosed() bool {
	st.spotWsPrivateClosedMtx.RLock()
	defer st.spotWsPrivateClosedMtx.RUnlock()
	return st.spotWsPrivateClosed
}
func (st *Safetrade) SpotWsPrivateClose() {
	st.spotWsPrivateClosedMtx.Lock()
	defer st.spotWsPrivateClosedMtx.Unlock()
	if st.spotWsPrivateClosed {
		return
	}
	st.spotWsPrivateClosed = true
	if st.spotWsPrivateConn != nil {
		st.spotWsPrivateConn.Close()
	}
}
func (st *Safetrade) SpotWsPrivateLoop(ch chan<- any) {
	defer st.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 34 * time.Second
	pongWait := pingInterval + 2*time.Second
	st.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	st.spotWsPrivateConn.SetPongHandler(func(string) error {
		st.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if st.SpotWsPrivateIsClosed() {
					return
				}
				st.spotWsPrivateConnMtx.Lock()
				st.spotWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
				st.spotWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	msg := make(map[string]WsRawJSON, 3) // 循环外分配一次, 之后复用
	for {
		_, recv, err := st.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !st.SpotWsPrivateIsClosed() {
				ilog.Warning(st.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		for k := range msg { // 清空后复用, 避免每条消息都make一次map
			delete(msg, k)
		}
		if st.debug {
			ilog.Rinfo(st.Name() + " spot.ws.priv recv: " + string(recv))
		}
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(st.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			continue
		}
		for k, v := range msg {
			if k == "order" {
				st.spotWsHandleOrder(v, ch)
			} else if k == "balance" {
				st.spotWsHandleBalance(v, ch)
			} else if k == "trade" { // 私有成交, 标准结构体里没有对应的类型
				ilog.Debug(st.Name() + " spot.ws.priv trade: " + string(v))
			} else if k == "success" { // 订阅/取消订阅的回执
				if !bytes.Contains(v, []byte("subscribe")) {
					ilog.Error(st.Name() + " spot.ws.priv recv: " + string(v))
				}
			} else {
				ilog.Error(st.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		}
	}
}
func (st *Safetrade) spotWsHandleOrder(data []byte, ch chan<- any) {
	recv := SafetradeWsOrder{}
	if err := easyjson.Unmarshal(data, &recv); err != nil || recv.Id == 0 {
		return
	}
	ordType := recv.OrdType
	if ordType == "" {
		ordType = recv.Type
	}
	ctime, _ := time.Parse(time.RFC3339, recv.CreatedAt)
	utime, _ := time.Parse(time.RFC3339, recv.UpdatedAt)
	status := st.toStdOrderState(recv.State)
	if status == "NEW" && recv.FilledAmount.IsPositive() {
		status = "PARTIALLY_FILLED"
	}
	ch <- &SpotOrder{
		Symbol:    strings.ToUpper(recv.Market),
		OrderId:   strconv.FormatInt(recv.Id, 10),
		Price:     recv.Price,
		Qty:       recv.OriginAmount,
		FilledQty: recv.FilledAmount,
		FilledAmt: recv.FilledAmount.Mul(recv.AvgPrice),
		AvgPrice:  recv.AvgPrice,
		Status:    status,
		Type:      st.toStdOrderType(ordType),
		Side:      st.toStdSide(recv.Side),
		CTime:     ctime.UnixMilli(),
		UTime:     utime.UnixMilli(),
	}
}
func (st *Safetrade) spotWsHandleBalance(data []byte, ch chan<- any) {
	recv := SafetradeWsBalance{}
	if err := easyjson.Unmarshal(data, &recv); err != nil || recv.Currency == "" {
		return
	}
	ch <- &SpotAsset{
		Symbol: strings.ToUpper(recv.Currency),
		Avail:  recv.Balance,
		Locked: recv.Locked,
		Total:  recv.Balance.Add(recv.Locked),
	}
}
