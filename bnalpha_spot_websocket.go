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

var (
	baWsPubMsgPool          sync.Pool
	baSpotWsPublicDepthPool sync.Pool
)

func init() {
	baWsPubMsgPool = sync.Pool{
		New: func() any {
			return &BnAlphaWsPubMsg{}
		},
	}
	baSpotWsPublicDepthPool = sync.Pool{
		New: func() any {
			return &BnAlphaDepthUpdate{
				Bids: make([][2]decimal.Decimal, 0, 16),
				Asks: make([][2]decimal.Decimal, 0, 16),
			}
		},
	}
}
func (ba *BnAlpha) newWsDialer() websocket.Dialer {
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if ba.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(ba.localIP),
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
func (ba *BnAlpha) SpotWsPublicOpen() error {
	dialer := ba.newWsDialer()
	conn, _, err := dialer.Dial("wss://nbstream.binance.com/w3w/wsa/stream", nil)
	if err != nil {
		return errors.New(ba.Name() + " spot.ws.public con failed! " + err.Error())
	}
	ba.spotWsPublicConn = conn

	ba.spotWsPublicClosedMtx.Lock()
	ba.spotWsPublicClosed = false
	ba.spotWsPublicClosedMtx.Unlock()
	return nil
}

// spotWsStreams 转换频道参数: bbo@ZKJUSDT -> alpha_173usdt@fulldepth@100ms
// bookTicker只推UI挂单(会跟成交价差很远), 这里改用增量深度流本地维护盘口
func (ba *BnAlpha) spotWsStreams(channels []string, subscribe bool) []string {
	params := make([]string, 0, 4)
	streamMap := make(map[string]bool, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if len(arr) < 2 || len(arr[1]) == 0 {
			continue
		}
		if arr[0] != "bbo" {
			continue
		}
		for _, sym := range strings.Split(arr[1], ",") {
			if len(sym) == 0 {
				continue
			}
			if subscribe {
				ba.spotWsPublicBBOSet[sym] = true
			} else {
				delete(ba.spotWsPublicBBOSet, sym)
			}
			symbolS := ba.getSpotSymbol(sym) // 标准symbol转交易所symbol
			if len(symbolS) == 0 {
				symbolS = sym
			}
			if subscribe {
				ba.spotWsGetDepthSnapshot(sym, symbolS) // 订阅时同步取快照
			}
			stream := strings.ToLower(symbolS) + "@fulldepth@100ms"
			if !streamMap[stream] { // 同一个symbol订阅了bbo和盘口时只发一次
				streamMap[stream] = true
				params = append(params, stream)
			}
		}
	}
	return params
}
func (ba *BnAlpha) spotWsPublicEvent(method string, channels []string) {
	params := ba.spotWsStreams(channels, method == "SUBSCRIBE")
	if len(params) == 0 {
		return
	}
	arg := BaSubscribeArg{Id: time.Now().UnixNano(), Method: method, Params: params}
	req, _ := json.Marshal(&arg)
	ba.spotWsPublicConnMtx.Lock()
	ba.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
	ba.spotWsPublicConnMtx.Unlock()
}
func (ba *BnAlpha) SpotWsPublicSubscribe(channels []string) {
	ba.spotWsPublicEvent("SUBSCRIBE", channels)
}
func (ba *BnAlpha) SpotWsPublicUnsubscribe(channels []string) {
	ba.spotWsPublicEvent("UNSUBSCRIBE", channels)
}
func (ba *BnAlpha) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (ba *BnAlpha) SpotWsPublicLoop(ch chan<- any) {
	defer ba.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 2*time.Second
	ba.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	ba.spotWsPublicConn.SetPingHandler(func(message string) error { // 交易所会主动发ping
		ba.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		err := ba.spotWsPublicConn.WriteControl(websocket.PongMessage,
			[]byte(message), time.Now().Add(2*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})
	ba.spotWsPublicConn.SetPongHandler(func(string) error {
		ba.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if ba.SpotWsPublicIsClosed() {
					return
				}
				ba.spotWsPublicConnMtx.Lock()
				ba.spotWsPublicConn.WriteMessage(websocket.PingMessage, nil)
				ba.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	l := 0
	for {
		_, recv, err := ba.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !ba.SpotWsPublicIsClosed() {
				ilog.Warning(ba.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := baWsPubMsgPool.Get().(*BnAlphaWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(ba.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Code != 0 { // 订阅出错
			ilog.Error(ba.Name() + " spot.ws.public recv err:" + msg.Msg + " " + string(recv))
			goto END
		}
		if l = len(msg.Stream); l > 0 { // 订阅/退订响应没有stream
			if l > 16 && msg.Stream[l-16:l] == "@fulldepth@100ms" {
				ba.spotWsHandleFullDepth(msg.Data, ch)
			} else {
				ilog.Error(ba.Name() + " spot.ws.public recv unknown stream: " + msg.Stream)
			}
		}
	END:
		baWsPubMsgPool.Put(msg)
	}
}
func (ba *BnAlpha) SpotWsPublicIsClosed() bool {
	ba.spotWsPublicClosedMtx.RLock()
	defer ba.spotWsPublicClosedMtx.RUnlock()
	return ba.spotWsPublicClosed
}
func (ba *BnAlpha) SpotWsPublicClose() {
	ba.spotWsPublicClosedMtx.Lock()
	defer ba.spotWsPublicClosedMtx.Unlock()
	if ba.spotWsPublicClosed {
		return
	}
	ba.spotWsPublicClosed = true
	if ba.spotWsPublicConn != nil {
		ba.spotWsPublicConn.Close()
	}
}

// spotWsHandleFullDepth 用增量深度维护本地盘口, 买一卖一有变化才推送
func (ba *BnAlpha) spotWsHandleFullDepth(data []byte, ch chan<- any) {
	depth := baSpotWsPublicDepthPool.Get().(*BnAlphaDepthUpdate)
	defer baSpotWsPublicDepthPool.Put(depth)
	depth.reset()
	if err := easyjson.Unmarshal(data, depth); err != nil || len(depth.Symbol) == 0 {
		return
	}
	symbol := ba.getStdSymbol(depth.Symbol) // 交易所symbol(ALPHA_1185USDT)转标准symbol(BTCUSDT)
	bids := ba.spotWsOrderBookBids[symbol]
	if bids == nil { // 首次(或序号断裂后): 取fullDepth快照重建本地盘口
		if !ba.spotWsInitOrderBook(symbol, depth.Symbol) {
			return
		}
		bids = ba.spotWsOrderBookBids[symbol]
	}
	asks := ba.spotWsOrderBookAsks[symbol]
	snapId := ba.spotWsOrderBookSnapId[symbol]
	prevId := ba.spotWsOrderBookPrevId[symbol]
	if prevId == 0 { // 快照之后的第一个可用事件: pu <= lastUpdateId 且 u >= lastUpdateId
		if depth.LastId < snapId { // 比快照还旧, 丢弃
			return
		}
		if depth.PrevId > snapId { // 快照与事件之间断档, 重新初始化
			ba.spotWsDropOrderBook(symbol)
			return
		}
	} else if depth.PrevId != prevId { // 序号断裂, 重新初始化
		ilog.Warning(ba.Name() + " spot.ws.public " + symbol + " depth seq error! " +
			strconv.FormatInt(depth.PrevId, 10) + " != " + strconv.FormatInt(prevId, 10))
		ba.spotWsDropOrderBook(symbol)
		return
	}
	for _, v := range depth.Bids {
		if v[1].IsZero() {
			bids.Remove(v[0])
		} else {
			bids.Put(v[0], v[1])
		}
	}
	for _, v := range depth.Asks {
		if v[1].IsZero() {
			asks.Remove(v[0])
		} else {
			asks.Put(v[0], v[1])
		}
	}
	ba.spotWsOrderBookPrevId[symbol] = depth.LastId
	if bids.Empty() || asks.Empty() { // 盘口还没收敛
		return
	}
	if !ba.spotWsPublicBBOSet[symbol] {
		return
	}
	// 原始盘口可能因为UI挂单出现买卖交叉, 先虚拟撮合去交叉
	bidPrice, bidQty, askPrice, askQty, ok := ba.virtualMatch(bids, asks)
	if !ok { // 某一侧被吃空, 本次数据无效
		return
	}

	cached := ba.spotWsBBOCache[symbol]
	if cached == nil {
		cached = &BestBidAsk{Symbol: symbol}
		ba.spotWsBBOCache[symbol] = cached
	} else if cached.BidPrice.Equal(bidPrice) && cached.BidQty.Equal(bidQty) &&
		cached.AskPrice.Equal(askPrice) && cached.AskQty.Equal(askQty) {
		return // 买一卖一没变化
	}
	cached.Time = depth.Time
	cached.BidPrice, cached.BidQty = bidPrice, bidQty
	cached.AskPrice, cached.AskQty = askPrice, askQty
	if !ba.spotWsBboAllow(symbol) { // n毫秒内的BBO丢弃
		return
	}
	bbo := wsPublicBBOPool.Get().(*BestBidAsk)
	*bbo = *cached
	ch <- bbo
}

// spotWsGetDepthSnapshot 订阅时同步取fullDepth快照并缓存
// 快照和fulldepth流都包含UI和API订单, 共享同一套updateId
func (ba *BnAlpha) spotWsGetDepthSnapshot(symbol, symbolS string) {
	if snap := ba.spotWsFetchDepthSnapshot(symbol, symbolS); snap != nil {
		ba.spotWsSnapMtx.Lock()
		ba.spotWsSnapCache[symbol] = snap
		ba.spotWsSnapMtx.Unlock()
	}
}
func (ba *BnAlpha) spotWsFetchDepthSnapshot(symbol, symbolS string) *BnAlphaDepthSnapshot {
	link := baEndpoint + "/bapi/defi/v1/public/alpha-trade/fullDepth?symbol=" + symbolS + "&limit=1000"
	_, resp, err := ba.Get(link, baApiDeadline, nil)
	if err != nil {
		ilog.Warning(ba.Name() + " spot.ws.public " + symbol + " depth snapshot net error! " + err.Error())
		return nil
	}
	recv := struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Data    struct {
			LastUpdateId int64                `json:"lastUpdateId,omitempty"`
			Bids         [][2]decimal.Decimal `json:"bids,omitempty"`
			Asks         [][2]decimal.Decimal `json:"asks,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		ilog.Warning(ba.Name() + " spot.ws.public " + symbol + " depth snapshot unmarshal fail! " + err.Error())
		return nil
	}
	if recv.Code != "000000" || recv.Data.LastUpdateId == 0 ||
		len(recv.Data.Bids) == 0 || len(recv.Data.Asks) == 0 {
		ilog.Warning(ba.Name() + " spot.ws.public " + symbol + " depth snapshot err! " + string(resp))
		return nil
	}
	return &BnAlphaDepthSnapshot{
		LastUpdateId: recv.Data.LastUpdateId,
		Bids:         recv.Data.Bids,
		Asks:         recv.Data.Asks,
	}
}

// spotWsInitOrderBook 用快照(优先用订阅时缓存的)建本地盘口
func (ba *BnAlpha) spotWsInitOrderBook(symbol, symbolS string) bool {
	ba.spotWsSnapMtx.Lock()
	snap := ba.spotWsSnapCache[symbol]
	delete(ba.spotWsSnapCache, symbol)
	ba.spotWsSnapMtx.Unlock()
	if snap == nil { // 没有缓存(比如序号断裂后重建)就现取一份
		snap = ba.spotWsFetchDepthSnapshot(symbol, symbolS)
	}
	if snap == nil {
		return false
	}
	bids, asks := ba.newOrderBooks(snap.Bids, snap.Asks)
	ba.spotWsOrderBookBids[symbol] = bids
	ba.spotWsOrderBookAsks[symbol] = asks
	ba.spotWsOrderBookSnapId[symbol] = snap.LastUpdateId
	ba.spotWsOrderBookPrevId[symbol] = 0
	return true
}

// spotWsDropOrderBook 序号断裂时丢掉本地盘口, 下一个事件会重新取快照
func (ba *BnAlpha) spotWsDropOrderBook(symbol string) {
	ba.spotWsSnapMtx.Lock()
	delete(ba.spotWsSnapCache, symbol)
	ba.spotWsSnapMtx.Unlock()
	delete(ba.spotWsOrderBookBids, symbol)
	delete(ba.spotWsOrderBookAsks, symbol)
	delete(ba.spotWsOrderBookSnapId, symbol)
	delete(ba.spotWsOrderBookPrevId, symbol)
	delete(ba.spotWsBBOCache, symbol)
}
