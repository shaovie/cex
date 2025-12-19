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
	"github.com/shaovie/gutils/ihttp"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

var (
	bnFuturesWsPrivMsgPool              sync.Pool
	bnFuturesWsPublicOrderBookInnerPool sync.Pool
	bnFuturesWsPublicTickerInnerPool    sync.Pool
	bnFuturesWsPublicBBOInnerPool       sync.Pool
)

func init() {
	bnFuturesWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &BnFuturesWsPrivMsg{}
		},
	}
	bnFuturesWsPublicOrderBookInnerPool = sync.Pool{
		New: func() any {
			return &BinanceFuturesOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
	bnFuturesWsPublicTickerInnerPool = sync.Pool{
		New: func() any {
			return &BinanceFutures24hTicker{}
		},
	}
	bnFuturesWsPublicBBOInnerPool = sync.Pool{
		New: func() any {
			return &BinanceFuturesBBO{}
		},
	}
}

func (bn *Binance) FuturesWsPublicOpen(typ string) error {
	url := "wss://fstream.binance.com/stream"
	if typ == "CM" {
		url = "wss://dstream.binance.com/stream"
	}
	bn.futuresWsPublicTyp = typ
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.futuresWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " futures.ws.public con failed! " + err.Error())
	}
	bn.futuresWsPublicClosedMtx.Lock()
	bn.futuresWsPublicClosed = false
	bn.futuresWsPublicClosedMtx.Unlock()
	return nil
}
func (bn *Binance) FuturesWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BnSubscribeArg{Method: "SUBSCRIBE"}
	arg.Id = "sub-" + gutils.RandomStr(12)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@depth5@100ms")
				}
			}
		} else if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@bookTicker")
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@miniTicker")
				}
			}
		}
	}
	if len(arg.Params) > 0 {
		req, _ := json.Marshal(&arg)
		bn.futuresWsPublicConnMtx.Lock()
		bn.futuresWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bn.futuresWsPublicConnMtx.Unlock()
	}
}
func (bn *Binance) FuturesWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	arg := BnSubscribeArg{Method: "UNSUBSCRIBE"}
	arg.Id = "sub-" + gutils.RandomStr(12)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@depth5@100ms")
				}
			}
		} else if arr[0] == "bbo" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@bookTicker")
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					if bn.futuresWsPublicTyp == "CM" {
						sym += "_PERP"
					}
					arg.Params = append(arg.Params, strings.ToLower(sym)+"@miniTicker")
				}
			}
		}
	}
	if len(arg.Params) > 0 {
		req, _ := json.Marshal(&arg)
		bn.futuresWsPublicConnMtx.Lock()
		bn.futuresWsPublicConn.WriteMessage(websocket.TextMessage, req)
		bn.futuresWsPublicConnMtx.Unlock()
	}
}
func (bn *Binance) FuturesWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (bn *Binance) FuturesWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (bn *Binance) FuturesWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (bn *Binance) FuturesWsPublicLoop(ch chan<- any) {
	defer bn.FuturesWsPublicClose()
	defer close(ch)

	pingInterval := 23 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.futuresWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.futuresWsPublicConn.SetPongHandler(func(string) error {
		bn.futuresWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.FuturesWsPublicIsClosed() {
				break
			}
			bn.futuresWsPublicConnMtx.Lock()
			bn.futuresWsPublicConn.WriteMessage(websocket.PingMessage, nil)
			bn.futuresWsPublicConnMtx.Unlock()
		}
	}()

	l := 0
	for {
		_, recv, err := bn.futuresWsPublicConn.ReadMessage()
		if err != nil {
			if !bn.FuturesWsPublicIsClosed() {
				ilog.Warning(bn.Name() + " futures.ws.public read: " + err.Error())
			}
			break
		}
		msg := bnWsPubMsgPool.Get().(*BinanceWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bn.Name() + " futures.ws.public invalid msg:" + string(recv))
			goto END
		}
		if msg.Code != 0 {
			ilog.Error(bn.Name() + " futures.ws.public recv subscribe err:" + string(recv))
			goto END
		}
		l = len(msg.Stream)
		if l > 13 && msg.Stream[l-13:l] == "@depth5@100ms" {
			bn.futuresWsHandleOrderBook5(msg.Data, ch)
		} else if l > 11 && msg.Stream[l-11:l] == "@bookTicker" {
			bn.futuresWsHandleBBO(msg.Data, ch)
		} else if l > 11 && msg.Stream[l-11:l] == "@miniTicker" {
			bn.futuresWsHandle24hTickers(msg.Data, ch)
		} else {
			if strings.Index(string(recv), `"result":null`) == -1 {
				ilog.Error(bn.Name() + " futures.ws.public recv unknown msg: " + string(recv))
			}
		}
	END:
		bnWsPubMsgPool.Put(msg)
	}
}
func (bn *Binance) FuturesWsPublicIsClosed() bool {
	bn.futuresWsPublicClosedMtx.RLock()
	defer bn.futuresWsPublicClosedMtx.RUnlock()
	return bn.futuresWsPublicClosed
}
func (bn *Binance) FuturesWsPublicClose() {
	bn.futuresWsPublicClosedMtx.Lock()
	defer bn.futuresWsPublicClosedMtx.Unlock()
	if bn.futuresWsPublicClosed {
		return
	}
	bn.futuresWsPublicClosed = true
	bn.futuresWsPublicConn.Close()
}
func (bn *Binance) futuresWsHandleOrderBook5(data json.RawMessage, ch chan<- any) {
	depth := bnFuturesWsPublicOrderBookInnerPool.Get().(*BinanceFuturesOrderBook)
	defer bnFuturesWsPublicOrderBookInnerPool.Put(depth)
	depth.Bids = depth.Bids[:0]
	depth.Asks = depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		if len(depth.Bids) != len(depth.Asks) {
			ilog.Error(bn.Name() + " futures.ws.public " + depth.Symbol + " orderbook5 exception")
			return
		}
		obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
		if bn.futuresWsPublicTyp == "CM" {
			obd.Symbol = strings.ReplaceAll(depth.Symbol, "_PERP", "")
		} else {
			obd.Symbol = depth.Symbol
		}
		obd.Level = len(depth.Bids)
		obd.Time = depth.Time
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
func (bn *Binance) futuresWsHandleBBO(data json.RawMessage, ch chan<- any) {
	bbo := bnFuturesWsPublicBBOInnerPool.Get().(*BinanceFuturesBBO)
	defer bnFuturesWsPublicBBOInnerPool.Put(bbo)
	if err := easyjson.Unmarshal(data, bbo); err == nil {
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		if bn.futuresWsPublicTyp == "CM" {
			obd.Symbol = strings.ReplaceAll(bbo.Symbol, "_PERP", "")
		} else {
			obd.Symbol = bbo.Symbol
		}
		obd.Time = bbo.Time
		obd.BidPrice = bbo.BidPrice
		obd.BidQty = bbo.BidQty
		obd.AskPrice = bbo.AskPrice
		obd.AskQty = bbo.AskQty
		ch <- obd
	}
}
func (bn *Binance) futuresWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	ticker := bnFuturesWsPublicTickerInnerPool.Get().(*BinanceFutures24hTicker)
	defer bnFuturesWsPublicTickerInnerPool.Put(ticker)
	if err := json.Unmarshal(data, ticker); err == nil {
		tk := wsPublicTickerPool.Get().(*Pub24hTicker)
		if bn.futuresWsPublicTyp == "CM" {
			tk.Symbol = strings.ReplaceAll(ticker.Symbol, "_PERP", "")
		} else {
			tk.Symbol = ticker.Symbol
		}
		tk.LastPrice = ticker.Last
		tk.Volume = ticker.Volume
		tk.QuoteVolume = ticker.QuoteVolume
		ch <- tk
	}
}

// = priv channel
func (bn *Binance) getListenKey(typ string) (string, error) {
	link := bnUMFuturesEndpoint + "/fapi/v1/listenKey"
	if typ == "CM" {
		link = bnCMFuturesEndpoint + "/dapi/v1/listenKey"
	}
	_, resp, err := ihttp.Post(link, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return "", errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code      int    `json:"code,omitempty"`
		Msg       string `json:"msg,omitempty"`
		ListenKey string `json:"listenKey,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}
	if ret.Code != 0 {
		return "", errors.New(bn.Name() + " api err! " + ret.Msg)
	}
	return ret.ListenKey, nil
}
func (bn *Binance) FuturesWsPrivateOpen(typ string) error {
	bn.futuresWsPrivateTyp = typ
	if err := bn.futuresWsPrivateOpen(typ); err != nil {
		return err
	}
	return bn.futuresWsPrivateApiOpen(typ)
}
func (bn *Binance) futuresWsPrivateOpen(typ string) error {
	listenKey, err := bn.getListenKey(typ)
	if err != nil {
		return errors.New(bn.Name() + " get listenkey fail! " + err.Error())
	}
	url := "wss://fstream.binance.com/ws/" + listenKey
	if typ == "CM" {
		url = "wss://dstream.binance.com/ws/" + listenKey
	}
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.futuresWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " priv c-ws connect failed! " + err.Error())
	}
	bn.futuresWsPrivateClosedMtx.Lock()
	bn.futuresWsPrivateClosed = false
	bn.futuresWsPrivateClosedMtx.Unlock()
	return nil
}
func (bn *Binance) futuresWsPrivateApiOpen(typ string) error {
	url := "wss://ws-fapi.binance.com/ws-fapi/v1?returnRateLimits=false"
	if typ == "CM" {
		url = "wss://ws-dapi.binance.com/ws-dapi/v1?returnRateLimits=false"
	}
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.futuresWsPrivateApiConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " priv c-apiws connect failed! " + err.Error())
	}

	bn.futuresWsPrivateApiClosedMtx.Lock()
	bn.futuresWsPrivateApiClosed = false
	bn.futuresWsPrivateApiClosedMtx.Unlock()
	return nil
}
func (bn *Binance) FuturesWsPrivateSubscribe(channels []string) {
}
func (bn *Binance) FuturesWsPrivateIsClosed() bool {
	return bn.futuresWsPrivateIsClosed() ||
		bn.futuresWsPrivateApiIsClosed()
}
func (bn *Binance) futuresWsPrivateIsClosed() bool {
	bn.futuresWsPrivateClosedMtx.RLock()
	defer bn.futuresWsPrivateClosedMtx.RUnlock()
	return bn.futuresWsPrivateClosed
}
func (bn *Binance) futuresWsPrivateApiIsClosed() bool {
	bn.futuresWsPrivateApiClosedMtx.RLock()
	defer bn.futuresWsPrivateApiClosedMtx.RUnlock()
	return bn.futuresWsPrivateApiClosed
}
func (bn *Binance) FuturesWsPrivateClose() {
	bn.futuresWsPrivateClose()
	bn.futuresWsPrivateApiClose()
}
func (bn *Binance) futuresWsPrivateClose() {
	bn.futuresWsPrivateClosedMtx.Lock()
	defer bn.futuresWsPrivateClosedMtx.Unlock()
	if bn.futuresWsPrivateClosed {
		return
	}
	bn.futuresWsPrivateClosed = true
	bn.futuresWsPrivateConn.Close()
}
func (bn *Binance) futuresWsPrivateApiClose() {
	bn.futuresWsPrivateApiClosedMtx.Lock()
	defer bn.futuresWsPrivateApiClosedMtx.Unlock()
	if bn.futuresWsPrivateApiClosed {
		return
	}
	bn.futuresWsPrivateApiClosed = true
	bn.futuresWsPrivateApiConn.Close()
}
func (bn *Binance) FuturesWsPrivateLoop(ch chan<- any) {
	var wg sync.WaitGroup
	wg.Add(2)
	go bn.futuresWsPrivateLoop(ch, &wg)
	go bn.futuresWsPrivateApiLoop(ch, &wg)
	wg.Wait()
	close(ch)
}

type BnFuturesWsPrivMsg struct {
	Event     string          `json:"e,omitempty"`
	Time      int64           `json:"E,omitempty"` // msec
	Result    json.RawMessage `json:"o,omitempty"`
	ResultPos json.RawMessage `json:"a,omitempty"`
}

func (v *BnFuturesWsPrivMsg) reset() {
	v.Event = ""
	v.Time = 0
	v.Result = nil
	v.ResultPos = nil
}
func (bn *Binance) futuresWsPrivateLoop(ch chan<- any, wg *sync.WaitGroup) {
	defer bn.futuresWsPrivateClose()
	defer wg.Done()

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.futuresWsPrivateConn.SetPongHandler(func(string) error {
		bn.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.futuresWsPrivateIsClosed() {
				break
			}
			bn.futuresWsPrivateConnMtx.Lock()
			bn.futuresWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
			bn.futuresWsPrivateConnMtx.Unlock()
		}
	}()

	go func() {
		ticker := time.NewTicker((3600 - 120) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if bn.futuresWsPrivateIsClosed() {
				break
			}
			bn.getListenKey(bn.futuresWsPrivateTyp)
		}
	}()

	for {
		_, recv, err := bn.futuresWsPrivateConn.ReadMessage()
		if err != nil {
			if !bn.futuresWsPrivateIsClosed() {
				ilog.Warning(bn.Name() + " futures.ws.priv read: " + err.Error())
			}
			break
		}
		ilog.Rinfo("futures.ws.priv: " + string(recv))
		msg := bnFuturesWsPrivMsgPool.Get().(*BnFuturesWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(bn.Name() + " futures.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "ORDER_TRADE_UPDATE" { // order
			bn.futuresWsHandleOrder(msg.Result, ch)
		} else if msg.Event == "ACCOUNT_UPDATE" { // balance and position
			bn.futuresWsHandlePosition(msg.ResultPos, ch, msg.Time)
		} else if msg.Event == "TRADE_LITE" { // trade
		} else if msg.Event == "ACCOUNT_CONFIG_UPDATE" { //
		} else {
			ilog.Error(bn.Name() + " futures.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		bnFuturesWsPrivMsgPool.Put(msg)
	}
}
func (bn *Binance) futuresWsHandleOrder(data json.RawMessage, ch chan<- any) {
	order := struct {
		ClientId      string          `json:"c,omitempty"` //
		OrderId       int64           `json:"i,omitempty"` //
		Symbol        string          `json:"s,omitempty"` // BTCUSDT
		Side          string          `json:"S,omitempty"`
		TimeInForce   string          `json:"f,omitempty"` // GTC/FOK/IOC
		OrderType     string          `json:"o,omitempty"`
		OrderTypeOrig string          `json:"ot,omitempty"` // 原始订单类型
		ExecTime      int64           `json:"T,omitempty"`  // 成交时间 msec
		TradeId       int64           `json:"t,omitempty"`  //
		Qty           decimal.Decimal `json:"q,omitempty"`  // 原始订单数量
		Price         decimal.Decimal `json:"p,omitempty"`  // 原始订单价格
		AvgPrice      decimal.Decimal `json:"ap,omitempty"` // 订单平均价格
		ExecQty       decimal.Decimal `json:"l,omitempty"`  // 末次成交数量
		ExecPrice     decimal.Decimal `json:"L,omitempty"`  // 末次成交价格
		ExecutedQty   decimal.Decimal `json:"z,omitempty"`  // 订单累计已成交量
		CummQuoteQty  decimal.Decimal `json:"Z,omitempty"`  // 订单累计已成交金额 文档不存在
		FeeQty        decimal.Decimal `json:"n,omitempty"`  // 手续费数量
		FeeAsset      string          `json:"N,omitempty"`  // 手续费类型
		EventType     string          `json:"x,omitempty"`  // 本次事件的执行类型
		Status        string          `json:"X,omitempty"`  // 订单当前状态
	}{}
	if err := json.Unmarshal(data, &order); err == nil && order.OrderId > 0 {
		var ctime int64
		if order.Status == "NEW" {
			ctime = order.ExecTime
		}
		if bn.futuresWsPrivateTyp == "CM" {
			order.Symbol = strings.ReplaceAll(order.Symbol, "_PERP", "")
		}
		ch <- &FuturesOrder{
			Symbol:    order.Symbol,
			OrderId:   strconv.FormatInt(order.OrderId, 10),
			ClientId:  order.ClientId,
			Price:     order.Price,
			Qty:       order.Qty,
			FilledQty: order.ExecutedQty,
			FilledAmt: order.ExecutedQty.Mul(order.AvgPrice),
			Status:    order.Status,
			Type:      order.OrderType,
			Side:      order.Side,
			FeeQty:    order.FeeQty.Neg(), // 换成负数
			FeeAsset:  order.FeeAsset,
			CTime:     ctime,
			UTime:     order.ExecTime,
		}
	}
}
func (bn *Binance) futuresWsHandlePosition(data json.RawMessage, ch chan<- any, t int64) {
	pl := struct {
		Event string `json:"m,omitempty"`
		Pos   []struct {
			Symbol       string          `json:"s,omitempty"`
			EntryPrice   decimal.Decimal `json:"ep,omitempty"`
			Qty          decimal.Decimal `json:"pa,omitempty"`
			PositionMode string          `json:"ps,omitempty"`
			UTime        int64           `json:"time_ms,omitempty"`
		} `json:"P,omitempty"`
	}{}
	if err := json.Unmarshal(data, &pl); err == nil && len(pl.Pos) > 0 {
		for _, p := range pl.Pos {
			if p.PositionMode != "BOTH" {
				continue // 只支持单仓模式 目前不支持 双仓模式
			}
			side := "SELL"
			if p.Qty.IsPositive() {
				side = "BUY"
			}
			if bn.futuresWsPrivateTyp == "CM" {
				p.Symbol = strings.ReplaceAll(p.Symbol, "_PERP", "")
			}
			cp := FuturesPosition{
				Symbol:      p.Symbol,
				Side:        side,
				PositionQty: p.Qty.Abs(),
				EntryPrice:  p.EntryPrice,
				UTime:       t,
			}
			ch <- &cp
		}
	}
}
func (bn *Binance) futuresWsPrivateApiLoop(ch chan<- any, wg *sync.WaitGroup) {
	defer bn.futuresWsPrivateApiClose()
	defer wg.Done()

	pingInterval := 32 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.futuresWsPrivateApiConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.futuresWsPrivateApiConn.SetPongHandler(func(string) error {
		bn.futuresWsPrivateApiConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.futuresWsPrivateApiIsClosed() {
				break
			}
			bn.futuresWsPrivateApiConnMtx.Lock()
			bn.futuresWsPrivateApiConn.WriteMessage(websocket.PingMessage, nil)
			bn.futuresWsPrivateApiConnMtx.Unlock()
		}
	}()

	type Msg struct {
		Id     string          `json:"Id,omitempty"`
		Status int             `json:"status,omitempty"`
		Result json.RawMessage `json:"result,omitempty"`
		Err    struct {
			Code int    `json:"code,omitempty"`
			Msg  string `json:"msg,omitempty"`
		} `json:"error,omitempty"`
	}
	for {
		_, recv, err := bn.futuresWsPrivateApiConn.ReadMessage()
		if err != nil {
			if !bn.futuresWsPrivateApiIsClosed() {
				ilog.Warning(bn.Name() + " futures.ws.priv.api channel read: " + err.Error())
			}
			break
		}
		msg := Msg{}
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(bn.Name() + " futures.ws.priv.api recv invalid msg:" + string(recv))
			continue
		}
		if len(msg.Id) > 5 && msg.Id[0:5] == "ford-" {
			bn.futuresWsHandlePlaceOrderResp(msg.Id, msg.Err.Msg, msg.Result, ch)
		} else if len(msg.Id) > 5 && msg.Id[0:5] == "fcle-" {
			bn.futuresWsHandleCancelOrderResp(msg.Err.Msg)
		} else {
			ilog.Error(bn.Name() + " futures.ws.priv.api recv unknown msg: " + string(recv))
		}
	}
}
func (bn *Binance) futuresWsHandlePlaceOrderResp(reqId, errS string,
	data json.RawMessage, ch chan<- any) {
	if errS != "" {
		order := &FuturesOrder{
			RequestId: reqId,
			Err:       errS,
		}
		ch <- order
		return
	}

	ret := struct {
		Symbol   string `json:"symbol,omitempty"`
		OrderId  int64  `json:"orderId,omitempty"`
		ClientId string `json:"clientOrderId,omitempty"`
	}{}
	if err := json.Unmarshal(data, &ret); err != nil {
		ilog.Error(bn.Name() + " futures.ws.priv.api handle place order resp: " + err.Error())
		return
	}
	ch <- &FuturesOrder{
		RequestId: reqId,
		OrderId:   strconv.FormatInt(ret.OrderId, 10),
		ClientId:  ret.ClientId,
	}
}
func (bn *Binance) futuresWsHandleCancelOrderResp(errS string) {
	if errS != "" {
		ilog.Error(bn.Name() + " futures cancel order fail! " + errS)
	}
}

// priv ws api
func (bn *Binance) FuturesWsPlaceOrder(symbol, cltId string,
	price, qty decimal.Decimal, side, orderType, timeInForce string,
	positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error) {
	if bn.futuresWsPrivateApiIsClosed() {
		return "", errors.New(bn.Name() + " futures.ws.priv.api ws closed")
	}

	if !qty.IsPositive() {
		return "", errors.New("qty too small! qty=" + qty.String())
	}
	if bn.futuresWsPrivateTyp == "CM" {
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
	}
	params := map[string]any{
		"apiKey":     bn.apikey,
		"timestamp":  time.Now().UnixMilli(),
		"recvWindow": 3000,

		"symbol":           symbol,
		"newClientOrderId": cltId,
		"newOrderRespType": "ACK",
		"side":             side,
		"type":             orderType,
		"quantity":         qty.String(),
	}
	if orderType == "LIMIT" {
		params["price"] = price.String()
		params["timeInForce"] = timeInForce
	} else if orderType == "MARKET" {
	}
	if positionMode == 0 {
		params["positionSide"] = "BOTH"
	}
	if reduceOnly == 1 {
		params["reduceOnly"] = true
	}
	params["signature"] = bn.wsSign(params)
	req := BnWsApiArg{Id: "ford-" + gutils.RandomStr(14), Method: "order.place", Params: &params}
	reqJson, _ := json.Marshal(req)
	ilog.Rinfo(bn.Name() + " post c order:" + string(reqJson))

	bn.futuresWsPrivateApiConnMtx.Lock()
	defer bn.futuresWsPrivateApiConnMtx.Unlock()
	if err := bn.futuresWsPrivateApiConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(bn.Name() + " send fail: " + err.Error())
	}
	return req.Id, nil
}
func (bn *Binance) FuturesWsCancelOrder(symbol, orderId, cltId string) (string, error) {
	if bn.futuresWsPrivateApiIsClosed() {
		return "", errors.New(bn.Name() + " futures.ws.priv.api ws closed")
	}
	if bn.futuresWsPrivateTyp == "CM" {
		if strings.Index(symbol, "_") == -1 {
			symbol += "_PERP"
		}
	}
	params := map[string]any{
		"apiKey":     bn.apikey,
		"timestamp":  time.Now().UnixMilli(),
		"recvWindow": 3000,

		"symbol": symbol,
	}
	ordId, _ := strconv.ParseInt(orderId, 10, 64)
	if ordId > 0 {
		params["orderId"] = ordId
	} else if cltId != "" {
		params["origClientOrderId"] = cltId
	} else {
		return "", errors.New("orderId or clientId is empty")
	}
	params["signature"] = bn.wsSign(params)
	req := BnWsApiArg{Id: "fcle-" + gutils.RandomStr(14), Method: "order.cancel", Params: params}
	reqJson, _ := json.Marshal(req)

	bn.futuresWsPrivateApiConnMtx.Lock()
	defer bn.futuresWsPrivateApiConnMtx.Unlock()
	if err := bn.futuresWsPrivateApiConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(bn.Name() + " send fail: " + err.Error())
	}
	return req.Id, nil
}
