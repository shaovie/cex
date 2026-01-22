package cex

import (
	"encoding/json"
	"errors"
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
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Args = append(arg.Args, "orderbook.1."+strings.ToLower(sym))
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
				symbolArr := strings.Split(arr[1], ",")
				for _, sym := range symbolArr {
					arg.Args = append(arg.Args, "orderbook.1."+strings.ToLower(sym))
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

	pingInterval := 26 * time.Second
	pongWait := pingInterval + 2*time.Second
	bb.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	bb.spotWsPublicConn.SetPongHandler(func(string) error {
		bb.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		ping := `{"op":"ping"}`
		for range ticker.C {
			if bb.SpotWsPublicIsClosed() {
				break
			}
			bb.spotWsPublicConnMtx.Lock()
			bb.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(ping))
			bb.spotWsPublicConnMtx.Unlock()
		}
	}()

	l := 0
	for {
		_, recv, err := bb.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bb.SpotWsPublicIsClosed() { // 并非主动断开
				ilog.Warning(bb.Name() + " spot.ws.public read: " + err.Error())
			}
			break
		}
		msg := bnWsPubMsgPool.Get().(*BybitWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bb.Name() + " spot.ws.public invalid msg:" + string(recv))
			goto END
		}
		l = len(msg.Topic)
		if l > 12 && msg.Topic[:12] == "orderbook.1." {
			bb.spotWsHandleBBO(msg, ch)
		} else {
			if msg.Op == "subscribe" || msg.Op == "unsubscribe" { // 订阅的响应
				if strings.Index(string(recv), "false") != -1 {
					ilog.Error(bb.Name() + " spot.ws.public recv subscribe err:" + string(recv))
				}
			}
		}
	END:
		bnWsPubMsgPool.Put(msg)
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
	if err := easyjson.Unmarshal(msg.Data, bbo); err == nil {
		/*
			obd := wsPublicBBOPool.Get().(*BestBidAsk)
			obd.Symbol = bbo.Symbol
			obd.Time = 0 // 币安不提供
			obd.BidPrice = bbo.BidPrice
			obd.BidQty = bbo.BidQty
			obd.AskPrice = bbo.AskPrice
			obd.AskQty = bbo.AskQty
			ch <- obd
		*/
	}
}
func (bb *Bybit) spotWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	ticker := bnSpotWsPublicTickerInnerPool.Get().(*BybitSpot24hTicker)
	defer bnSpotWsPublicTickerInnerPool.Put(ticker)
	if err := json.Unmarshal(data, ticker); err == nil {
		tk := wsPublicTickerPool.Get().(*Pub24hTicker)
		tk.Symbol = ticker.Symbol
		tk.LastPrice = ticker.Last
		tk.Volume = ticker.Volume
		tk.QuoteVolume = ticker.QuoteVolume
		ch <- tk
	}
}
