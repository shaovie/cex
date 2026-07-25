package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shaovie/gutils/ihttp"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

var (
	kkWsMsgPool                      sync.Pool
	kkSpotWsPublicBBOInnerPool       sync.Pool
	kkSpotWsPublicOrderBookInnerPool sync.Pool
)

func init() {
	kkWsMsgPool = sync.Pool{
		New: func() any {
			return &KrakenWsMsg{}
		},
	}
	kkSpotWsPublicBBOInnerPool = sync.Pool{
		New: func() any {
			return make([]KrakenSpotTicker, 0, 4)
		},
	}
	kkSpotWsPublicOrderBookInnerPool = sync.Pool{
		New: func() any {
			return make([]KrakenSpotOrderBook, 0, 4)
		},
	}
}

func (kk *Kraken) SpotWsPublicOpen() error {
	url := "wss://ws.kraken.com/v2"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	kk.spotWsPublicConn, _, err = dialer.Dial(url, http.Header{
		"Sec-WebSocket-Protocol": []string{"json"},
	})
	if err != nil {
		return errors.New(kk.Name() + " spot.ws.public con failed! " + err.Error())
	}
	kk.spotWsPublicClosedMtx.Lock()
	kk.spotWsPublicClosed = false
	kk.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (kk *Kraken) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	ob5Symbols := make([]string, 0, 4)
	bboSymbols := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kk.getSpotWssSymbol(v); sym != "" {
					ob5Symbols = append(ob5Symbols, sym)
				}
			}
		} else if arr[0] == "bbo" { // 用ticker实现
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kk.getSpotWssSymbol(v); sym != "" {
					bboSymbols = append(bboSymbols, sym)
				}
			}
		} else if arr[0] == "trades" {
		}
	}
	if len(ob5Symbols) > 0 {
		jv, _ := json.Marshal(ob5Symbols)
		req := fmt.Sprintf(`{"method":"subscribe","params":{"channel":"book","depth":10,"symbol":%s}}`,
			string(jv))
		kk.spotWsPublicConnMtx.Lock()
		kk.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		kk.spotWsPublicConnMtx.Unlock()
	}
	if len(bboSymbols) > 0 {
		jv, _ := json.Marshal(bboSymbols)
		req := fmt.Sprintf(`{"method":"subscribe","params":{"channel":"ticker","event_trigger":"bbo","symbol":%s}}`,
			string(jv))
		kk.spotWsPublicConnMtx.Lock()
		kk.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		kk.spotWsPublicConnMtx.Unlock()
	}
}
func (kk *Kraken) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	ob5Symbols := make([]string, 0, 4)
	bboSymbols := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kk.getSpotWssSymbol(v); sym != "" {
					ob5Symbols = append(ob5Symbols, sym)
				}
			}
		} else if arr[0] == "bbo" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kk.getSpotWssSymbol(v); sym != "" {
					bboSymbols = append(bboSymbols, sym)
				}
			}
		} else if arr[0] == "trades" {
		}
	}
	if len(ob5Symbols) > 0 {
		jv, _ := json.Marshal(ob5Symbols)
		req := fmt.Sprintf(`{"method":"unsubscribe","params":{"channel":"book","depth":10,"symbol":%s}}`,
			string(jv))
		kk.spotWsPublicConnMtx.Lock()
		kk.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		kk.spotWsPublicConnMtx.Unlock()
	}
	if len(bboSymbols) > 0 {
		jv, _ := json.Marshal(bboSymbols)
		req := fmt.Sprintf(`{"method":"unsubscribe","params":{"channel":"ticker","event_trigger":"bbo","symbol":%s}}`,
			string(jv))
		kk.spotWsPublicConnMtx.Lock()
		kk.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		kk.spotWsPublicConnMtx.Unlock()
	}
}
func (kk *Kraken) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (kk *Kraken) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (kk *Kraken) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (kk *Kraken) SpotWsPublicTradePoolPut(v any) {
	wsPublicTradePool.Put(v)
}
func (kk *Kraken) SpotWsPublicLoop(ch chan<- any) {
	defer kk.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 24 * time.Second
	pongWait := pingInterval + 2*time.Second
	kk.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		var pingMsg = []byte(`{"method":"ping"}`)
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if kk.SpotWsPublicIsClosed() {
					break
				}
				kk.spotWsPublicConnMtx.Lock()
				kk.spotWsPublicConn.WriteMessage(websocket.TextMessage, pingMsg)
				kk.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := kk.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !kk.SpotWsPublicIsClosed() {
				ilog.Warning(kk.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := kkWsMsgPool.Get().(*KrakenWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(kk.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}

		if msg.Channel == "book" {
			if msg.Type == "snapshot" {
				if symbol, ok := kk.spotWsHandleOrderBookSnap(msg.Data); ok {
					kk.spotWsHandleOrderBook1(symbol, ch)
				}
			} else if msg.Type == "update" {
				if symbol, ok := kk.spotWsHandleOrderBookUpdate(msg.Data); ok {
					kk.spotWsHandleOrderBook1(symbol, ch)
				}
			}
		} else if msg.Channel == "ticker" {
			kk.spotWsHandleBBO(msg.Data, ch) // snap or update are same
		} else if msg.Channel == "heartbeat" || msg.Channel == "status" {
		} else if msg.Method == "pong" {
			kk.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		} else if msg.Method == "subscribe" {
		} else if msg.Method == "unsubscribe" {
		} else {
			ilog.Error(kk.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		kkWsMsgPool.Put(msg)
	}
}
func (kk *Kraken) SpotWsPublicIsClosed() bool {
	kk.spotWsPublicClosedMtx.RLock()
	defer kk.spotWsPublicClosedMtx.RUnlock()
	return kk.spotWsPublicClosed
}
func (kk *Kraken) SpotWsPublicClose() {
	kk.spotWsPublicClosedMtx.Lock()
	defer kk.spotWsPublicClosedMtx.Unlock()
	if kk.spotWsPublicClosed {
		return
	}
	kk.spotWsPublicClosed = true
	kk.spotWsPublicConn.Close()
}

type KrakenSpotOrderBook struct {
	Symbol string `json:"symbol"`
	Bids   []struct {
		Price decimal.Decimal `json:"price"`
		Qty   decimal.Decimal `json:"qty"`
	} `json:"bids"`
	Asks []struct {
		Price decimal.Decimal `json:"price"`
		Qty   decimal.Decimal `json:"qty"`
	} `json:"asks"`
	Checksum int64 `json:"checksum"`
}

func (kk *Kraken) spotWsHandleOrderBookSnap(data json.RawMessage) (string, bool) {
	obs := kkSpotWsPublicOrderBookInnerPool.Get().([]KrakenSpotOrderBook)
	defer func() {
		kkSpotWsPublicOrderBookInnerPool.Put(obs)
	}()
	for i := range obs {
		obs[i].Symbol = ""
		obs[i].Checksum = 0
		obs[i].Bids = obs[i].Bids[:0]
		obs[i].Asks = obs[i].Asks[:0]
	}
	obs = obs[:0]
	if err := json.Unmarshal(data, &obs); err == nil && len(obs) > 0 {
		for i := range obs {
			before, after, ok0 := strings.Cut(obs[i].Symbol, "/")
			if !ok0 {
				continue
			}
			symbol := before + after
			kk.spotWsOrderBookSeqId[symbol] = obs[i].Checksum
			bids := treemap.NewWith[decimal.Decimal, decimal.Decimal](func(a, b decimal.Decimal) int {
				return b.Compare(a) // desc
			})
			asks := treemap.NewWith[decimal.Decimal, decimal.Decimal](func(a, b decimal.Decimal) int {
				return a.Compare(b) // asc
			})
			for _, item := range obs[i].Bids {
				if item.Qty.IsPositive() {
					bids.Put(item.Price, item.Qty)
				}
			}
			kk.spotWsOrderBookBids[symbol] = bids
			for _, item := range obs[i].Asks {
				if item.Qty.IsPositive() {
					asks.Put(item.Price, item.Qty)
				}
			}
			kk.spotWsOrderBookAsks[symbol] = asks
			return symbol, true
		}
	}
	return "", false
}
func (kk *Kraken) spotWsHandleOrderBookUpdate(data json.RawMessage) (string, bool) {
	obs := kkSpotWsPublicOrderBookInnerPool.Get().([]KrakenSpotOrderBook)
	defer func() {
		kkSpotWsPublicOrderBookInnerPool.Put(obs)
	}()
	obs = obs[:0]
	if err := json.Unmarshal(data, &obs); err == nil && len(obs) > 0 {
		for i := range obs {
			base, quote, ok0 := strings.Cut(obs[i].Symbol, "/")
			if !ok0 {
				continue
			}
			symbol := base + quote
			bids := kk.spotWsOrderBookBids[symbol]
			asks := kk.spotWsOrderBookAsks[symbol]
			if bids == nil || asks == nil {
				return "", false
			}
			for _, item := range obs[i].Bids {
				if item.Qty.IsZero() {
					bids.Remove(item.Price)
				} else {
					bids.Put(item.Price, item.Qty)
				}
			}
			kk.spotWsOrderBookBids[symbol] = bids
			for _, item := range obs[i].Asks {
				if item.Qty.IsZero() {
					asks.Remove(item.Price)
				} else {
					asks.Put(item.Price, item.Qty)
				}
			}
			kk.spotWsOrderBookAsks[symbol] = asks
			return symbol, true
		}
	}
	return "", false
}

// 旧代码，本来是用它实现BBO，没用了
func (kk *Kraken) spotWsHandleOrderBook1(symbol string, ch chan<- any) {
	bids := kk.spotWsOrderBookBids[symbol]
	asks := kk.spotWsOrderBookAsks[symbol]
	if bids.Size() < 1 || asks.Size() < 1 {
		return
	}
	obd := BestBidAsk{}
	it := bids.Iterator()
	for range 1 {
		it.Next()
		obd.BidPrice = it.Key()
		obd.BidQty = it.Value()
	}
	it = asks.Iterator()
	for range 1 {
		it.Next()
		obd.AskPrice = it.Key()
		obd.AskQty = it.Value()
	}
	if kk.spotWsOrderBookBid1Ask1Cache.BidPrice.Equals(obd.BidPrice) &&
		kk.spotWsOrderBookBid1Ask1Cache.BidQty.Equals(obd.BidQty) &&
		kk.spotWsOrderBookBid1Ask1Cache.AskPrice.Equals(obd.AskPrice) &&
		kk.spotWsOrderBookBid1Ask1Cache.AskQty.Equals(obd.AskQty) {
		return
	}
	one := wsPublicBBOPool.Get().(*BestBidAsk)
	kk.spotWsOrderBookBid1Ask1Cache = obd
	*one = obd
	one.Symbol = symbol
	ch <- one // 这个SB交易所返回的价格和数量精度跟交易规则里边不一致，使用的时候小心
}

type KrakenSpotTicker struct {
	Symbol   string          `json:"symbol"`
	BidPrice decimal.Decimal `json:"bid"`
	BidQty   decimal.Decimal `json:"bid_qty"`
	AskPrice decimal.Decimal `json:"ask"`
	AskQty   decimal.Decimal `json:"ask_qty"`
}

func (kk *Kraken) spotWsHandleBBO(data json.RawMessage, ch chan<- any) {
	bbo := kkSpotWsPublicBBOInnerPool.Get().([]KrakenSpotTicker)
	defer func() {
		kkSpotWsPublicBBOInnerPool.Put(bbo)
	}()
	bbo = bbo[:0]
	if err := json.Unmarshal(data, &bbo); err == nil && len(bbo) > 0 {
		for i := range bbo {
			before, after, ok0 := strings.Cut(bbo[i].Symbol, "/")
			if !ok0 {
				continue
			}
			obd := wsPublicBBOPool.Get().(*BestBidAsk)
			obd.Symbol = before + after
			obd.BidPrice = bbo[i].BidPrice
			obd.BidQty = bbo[i].BidQty
			obd.AskPrice = bbo[i].AskPrice
			obd.AskQty = bbo[i].AskQty
			ch <- obd
		}
	}
}

// = priv channel
func (kk *Kraken) SpotWsPrivateSupported() bool {
	return true
}
func (kk *Kraken) getWsToken() (string, error) {
	path := "/0/private/GetWebSocketsToken"
	link := kkSpotEndpoint + path
	values := url.Values{}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return "", errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error,omitempty"`
		Result struct {
			Token string `json:"token"`
		} `json:"result,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return "", errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return "", errors.New(kk.Name() + " get wss token fail! " + recv.Error[0])
	}
	if recv.Result.Token == "" {
		return "", errors.New(kk.Name() + " get wss token fail!")
	}
	return recv.Result.Token, nil
}
func (kk *Kraken) SpotWsPrivateOpen() error {
	url := "wss://ws-auth.kraken.com/v2"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	kk.spotWsPrivateConn, _, err = dialer.Dial(url, http.Header{
		"Sec-WebSocket-Protocol": []string{"json"},
	})
	if err != nil {
		return errors.New(kk.Name() + " spot.ws.priv connect failed! " + err.Error())
	}

	token, err := kk.getWsToken()
	if err != nil {
		kk.SpotWsPrivateClose()
		return errors.New(kk.Name() + " spot.ws.priv auth failed! " + err.Error())
	}

	kk.spotWsPrivateConnMtx.Lock()
	kk.spotWsPrivateToken = token
	kk.spotWsPrivatePongTime = 0
	kk.spotWsPrivateExpectedPongTime = 0
	kk.spotWsPrivateConnMtx.Unlock()

	kk.spotWsPrivateClosedMtx.Lock()
	kk.spotWsPrivateClosed = false
	kk.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (kk *Kraken) SpotWsPrivateSubscribe(channels []string) {
	for _, c := range channels {
		if c == "orders" {
			req := fmt.Sprintf(`{"method":"subscribe","params":{"channel":"executions",`+
				`"token":"%s","snap_trades":false,"snap_orders":false,"order_status":true}}`,
				kk.spotWsPrivateToken)
			kk.spotWsPrivateConnMtx.Lock()
			if err := kk.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req)); err != nil {
				ilog.Error("%s spot.ws.priv send sub req: %s", kk.Name(), err.Error())
			}
			kk.spotWsPrivateConnMtx.Unlock()
		} else if c == "balance" {
			req := fmt.Sprintf(`{"method":"subscribe","params":{"channel":"balances",`+
				`"token":"%s","snapshot":true}}`,
				kk.spotWsPrivateToken)
			kk.spotWsPrivateConnMtx.Lock()
			kk.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req))
			kk.spotWsPrivateConnMtx.Unlock()
		}
	}
}
func (kk *Kraken) SpotWsPrivateIsClosed() bool {
	kk.spotWsPrivateClosedMtx.RLock()
	defer kk.spotWsPrivateClosedMtx.RUnlock()
	return kk.spotWsPrivateClosed
}
func (kk *Kraken) SpotWsPrivateClose() {
	kk.spotWsPrivateClosedMtx.Lock()
	defer kk.spotWsPrivateClosedMtx.Unlock()
	if kk.spotWsPrivateClosed {
		return
	}
	kk.spotWsPrivateClosed = true
	kk.spotWsPrivateConn.Close()
}
func (kk *Kraken) SpotWsPrivateLastPong() (int64, int64, int64) {
	kk.spotWsPrivateConnMtx.Lock()
	defer kk.spotWsPrivateConnMtx.Unlock()
	return kk.spotWsPrivatePongTime, kk.spotWsPrivateExpectedPongTime, kk.spotWsPrivatePingInterval
}
func (kk *Kraken) SpotWsPrivateLoop(ch chan<- any) {
	defer kk.SpotWsPrivateClose()
	defer close(ch)

	pingV := int64(25)
	kk.spotWsPrivatePingInterval = pingV
	pingInterval := time.Duration(pingV) * time.Second
	pongWait := pingInterval + 2*time.Second
	kk.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		var pingMsg = []byte(`{"method":"ping"}`)
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if kk.SpotWsPrivateIsClosed() {
					break
				}
				kk.spotWsPrivateConnMtx.Lock()
				kk.spotWsPrivateConn.WriteMessage(websocket.TextMessage, pingMsg)
				kk.spotWsPrivateExpectedPongTime = time.Now().Unix() + pingV
				kk.spotWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := kk.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !kk.SpotWsPrivateIsClosed() {
				ilog.Warning(kk.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if kk.debug {
			ilog.Rinfo(kk.Name() + " spot priv ws: " + string(recv))
		}
		msg := kkWsMsgPool.Get().(*KrakenWsMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(kk.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Channel == "balances" {
			if msg.Type == "snapshot" {
				kk.spotWsHandleAccountSnap(msg.Data, ch)
			} else if msg.Type == "update" {
				kk.spotWsHandleAccountUpdate(msg.Data, ch)
			}
		} else if msg.Channel == "executions" {
			kk.spotWsHandleOrder(msg.Data, ch)
		} else if msg.Channel == "heartbeat" || msg.Channel == "status" {
		} else if msg.Method == "pong" {
			kk.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			kk.spotWsPrivateConnMtx.Lock()
			kk.spotWsPrivatePongTime = time.Now().Unix()
			kk.spotWsPrivateConnMtx.Unlock()
		} else if msg.Method == "subscribe" {
		} else if msg.Method == "unsubscribe" {
		} else {
			ilog.Error(kk.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		kkWsMsgPool.Put(msg)
	}
}

type KrakenCachedOrder struct {
	Time   int64
	Symbol string
}

func (kk *Kraken) clearKrakenOrderCache(now int64) {
	keys := make([]string, 0, 4)
	for k, v := range kk.spotWsOrderCachedInfo {
		if now-v.Time > 600*1000 {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		delete(kk.spotWsOrderCachedInfo, k)
	}
}
func (kk *Kraken) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
	type Fee struct {
		Asset string          `json:"asset"` // 币种
		Qty   decimal.Decimal `json:"qty"`   // 数量
	}
	orders := []struct {
		Symbol    string          `json:"symbol"`
		OrderId   string          `json:"order_id"`
		ClientId  string          `json:"cl_ord_id"`
		Price     decimal.Decimal `json:"limit_price"`
		Qty       decimal.Decimal `json:"order_qty"`
		FilledQty decimal.Decimal `json:"cum_qty"`
		FilledAmt decimal.Decimal `json:"cum_cost"`
		AvgPrice  decimal.Decimal `json:"avg_price"`
		Status    string          `json:"order_status"`
		Type      string          `json:"order_type"`
		Side      string          `json:"side"`
		FeeDetail []Fee           `json:"fees"`
		Time      string          `json:"timestamp"`
	}{}
	if err := json.Unmarshal(data, &orders); err == nil && len(orders) > 0 {
		now := time.Now().UnixMilli()
		for i := range orders {
			ts, _ := time.Parse(time.RFC3339, orders[i].Time)
			symbol := strings.ReplaceAll(orders[i].Symbol, "/", "")
			so := &SpotOrder{
				Symbol:    symbol,
				OrderId:   orders[i].OrderId,
				ClientId:  orders[i].ClientId,
				Price:     orders[i].Price,
				Qty:       orders[i].Qty,
				FilledQty: orders[i].FilledQty,
				FilledAmt: orders[i].FilledAmt,
				AvgPrice:  orders[i].AvgPrice,
				Status:    kk.toStdOrderStatus(orders[i].Status),
				Type:      kk.toStdOrderType(orders[i].Type),
				Side:      kk.toStdSide(orders[i].Side),
				CTime:     ts.UnixMilli(),
				UTime:     ts.UnixMilli(),
			}
			for _, f := range orders[i].FeeDetail {
				so.FeeAsset = f.Asset
				so.FeeQty = f.Qty.Neg()
				break
			}
			if so.Status == "NEW" && so.Type == "" {
				// "new" 状态屁用没有
				continue
			}
			if so.Status == "NEW" && so.Symbol != "" {
				if ci := kk.spotWsOrderCachedInfo[so.OrderId]; ci == nil {
					kk.spotWsOrderCachedInfo[so.OrderId] = &KrakenCachedOrder{
						Symbol: so.Symbol,
						Time:   now,
					}
				}
			}
			if so.Status == "FILLED" && so.Symbol == "" {
				if ci := kk.spotWsOrderCachedInfo[so.OrderId]; ci != nil {
					so.Symbol = ci.Symbol
				}
			}
			// 这个SB交易所，FILLED只是一个状态，里边不带任何数据
			ch <- so
		}
		kk.clearKrakenOrderCache(now)
	}
}
func (kk *Kraken) spotWsHandleAccountUpdate(data json.RawMessage, ch chan<- any) {
	soL := []struct {
		Symbol  string          `json:"asset"`
		Balance decimal.Decimal `json:"balance"`
		Typ     string          `json:"wallet_type"`
	}{}
	if err := json.Unmarshal(data, &soL); err == nil && len(soL) > 0 {
		for i := range soL {
			if soL[i].Typ == "spot" {
				ch <- &SpotAsset{
					Symbol: soL[i].Symbol,
					Avail:  soL[i].Balance,
					Total:  soL[i].Balance,
				}
			}
		}
	}
}
func (kk *Kraken) spotWsHandleAccountSnap(data json.RawMessage, ch chan<- any) {
	soL := []struct {
		Symbol  string          `json:"asset"`
		Balance decimal.Decimal `json:"balance"`
		Wallets []struct {
			Typ     string          `json:"type"`
			Balance decimal.Decimal `json:"balance"`
		} `json:"wallets"`
	}{}
	if err := json.Unmarshal(data, &soL); err == nil && len(soL) > 0 {
		for i := range soL {
			if soL[i].Balance.IsZero() {
				continue
			}
			for _, sv := range soL[i].Wallets {
				ch <- &SpotAsset{
					Symbol: soL[i].Symbol,
					Avail:  sv.Balance,
					Total:  sv.Balance,
				}
			}
		}
	}
}
