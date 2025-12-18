package cex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
)

var (
	boSpotWsPubMsgPool  sync.Pool
	boSpotWsPrivMsgPool sync.Pool
)

func init() {
	boSpotWsPubMsgPool = sync.Pool{
		New: func() any {
			return &BigoneSpotWsPubMsg{}
		},
	}
	boSpotWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &BigoneSpotWsPrivMsg{}
		},
	}
}
func (bo *Bigone) SpotWsPublicOpen() error {
	url := "wss://big.one/ws/v2"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bo.spotWsPublicConn, _, err = dialer.Dial(url, http.Header{
		"Sec-WebSocket-Protocol": []string{"json"},
	})
	if err != nil {
		return errors.New(bo.Name() + " spot.ws.public con failed! " + err.Error())
	}
	bo.spotWsPublicClosedMtx.Lock()
	bo.spotWsPublicClosed = false
	bo.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (bo *Bigone) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	tickerSymbols := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbol := bo.getSpotSymbol(sym); symbol != "" {
					req := fmt.Sprintf(`{"requestId": "%s", "subscribeMarketDepthRequest":{"market":"%s"}}`,
						gutils.RandomStr(8), symbol)
					bo.spotWsPublicConnMtx.Lock()
					bo.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
					bo.spotWsPublicConnMtx.Unlock()
				}
			}
		} else if arr[0] == "ticker xx" { // 先不支持
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := bo.getSpotSymbol(v); sym != "" {
					tickerSymbols = append(tickerSymbols, sym)
				}
			}
		}
	}
	if len(tickerSymbols) > 0 {
		jv, _ := json.Marshal(tickerSymbols)
		req := fmt.Sprintf(`{"requestId": "%s", "subscribeMarketsTickerRequest":{"markets":%s}}`,
			gutils.RandomStr(8), string(jv))
		bo.spotWsPublicConnMtx.Lock()
		bo.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		bo.spotWsPublicConnMtx.Unlock()
	}
}
func (bo *Bigone) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	tickerSymbols := make([]string, 0, 4)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbol := bo.getSpotSymbol(sym); symbol != "" {
					req := fmt.Sprintf(`{"requestId": "%s", "unsubscribeMarketDepthRequest":{"market":"%s"}}`,
						gutils.RandomStr(8), symbol)
					bo.spotWsPublicConnMtx.Lock()
					bo.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
					bo.spotWsPublicConnMtx.Unlock()
				}
			}
		} else if arr[0] == "ticker" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := bo.getSpotSymbol(v); sym != "" {
					tickerSymbols = append(tickerSymbols, sym)
				}
			}
		}
	}
	if len(tickerSymbols) > 0 {
		jv, _ := json.Marshal(tickerSymbols)
		req := fmt.Sprintf(`{"requestId": "%s", "unsubscribeMarketsTickerRequest":{"markets":%s}}`,
			gutils.RandomStr(8), string(jv))
		bo.spotWsPublicConnMtx.Lock()
		bo.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		bo.spotWsPublicConnMtx.Unlock()
	}
}
func (bo *Bigone) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (bo *Bigone) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (bo *Bigone) SpotWsPublicLoop(ch chan<- any) {
	defer bo.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 27 * time.Second
	pongWait := pingInterval + 2*time.Second
	bo.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bo.SpotWsPublicIsClosed() {
				break
			}
			bo.spotWsPublicConnMtx.Lock()
			bo.spotWsPublicConn.WriteMessage(websocket.PingMessage, nil)
			bo.spotWsPublicConnMtx.Unlock()
		}
	}()
	bo.spotWsPublicConn.SetPongHandler(func(message string) error {
		bo.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, recv, err := bo.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !bo.SpotWsPublicIsClosed() {
				ilog.Warning(bo.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := boSpotWsPubMsgPool.Get().(*BigoneSpotWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bo.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}

		if msg.DepthSnap != nil {
			if symbol, ok := bo.spotWsHandleOrderBookSnap(msg.DepthSnap); ok {
				bo.spotWsHandleOrderBook5(symbol, ch)
			}
		} else if msg.DepthUpdate != nil {
			if symbol, ok := bo.spotWsHandleOrderBookUpdate(msg.DepthUpdate); ok {
				bo.spotWsHandleOrderBook5(symbol, ch)
			}
		} else if msg.TickerSnap != nil {
		} else if msg.TickerUpdate != nil {
		} else if bytes.Contains(recv, []byte(`"heartbeat":`)) {
		} else if bytes.Contains(recv, []byte(`"success":`)) {
		} else {
			ilog.Error(bo.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		boSpotWsPubMsgPool.Put(msg)
	}
}
func (bo *Bigone) SpotWsPublicIsClosed() bool {
	bo.spotWsPublicClosedMtx.RLock()
	defer bo.spotWsPublicClosedMtx.RUnlock()
	return bo.spotWsPublicClosed
}
func (bo *Bigone) SpotWsPublicClose() {
	bo.spotWsPublicClosedMtx.Lock()
	defer bo.spotWsPublicClosedMtx.Unlock()
	if bo.spotWsPublicClosed {
		return
	}
	bo.spotWsPublicClosed = true
	bo.spotWsPublicConn.Close()
}
func (bo *Bigone) spotWsHandleOrderBookSnap(data json.RawMessage) (string, bool) {
	depth := bo.spotWsPublicOrderBookInnerPool.Get().(*BigoneSpotOrderBook)
	defer bo.spotWsPublicOrderBookInnerPool.Put(depth)
	depth.Depth.Bids = depth.Depth.Bids[:0]
	depth.Depth.Asks = depth.Depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		symbol := strings.ReplaceAll(depth.Depth.Symbol, "-", "")
		bo.spotWsOrderBookSeqId[symbol] = depth.ChangeId
		bids := treemap.NewWith[decimal.Decimal, string](func(a, b decimal.Decimal) int {
			return b.Compare(a) // desc
		})
		asks := treemap.NewWith[decimal.Decimal, string](func(a, b decimal.Decimal) int {
			return a.Compare(b) // asc
		})
		for _, item := range depth.Depth.Bids {
			if item.Qty != "0" {
				bids.Put(item.Price, item.Qty)
			}
		}
		bo.spotWsOrderBookBids[symbol] = bids
		for _, item := range depth.Depth.Asks {
			if item.Qty != "0" {
				asks.Put(item.Price, item.Qty)
			}
		}
		bo.spotWsOrderBookAsks[symbol] = asks
		return symbol, true
	}
	return "", false
}
func (bo *Bigone) spotWsHandleOrderBookUpdate(data json.RawMessage) (string, bool) {
	depth := bo.spotWsPublicOrderBookInnerPool.Get().(*BigoneSpotOrderBook)
	defer bo.spotWsPublicOrderBookInnerPool.Put(depth)
	depth.Depth.Bids = depth.Depth.Bids[:0]
	depth.Depth.Asks = depth.Depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		symbol := strings.ReplaceAll(depth.Depth.Symbol, "-", "")
		if depth.PrevId != bo.spotWsOrderBookSeqId[symbol] {
			ilog.Error(bo.Name() + " spot.ws.public orderbook seq error!")
			return "", false
		}
		bo.spotWsOrderBookSeqId[symbol] = depth.ChangeId
		bids := bo.spotWsOrderBookBids[symbol]
		asks := bo.spotWsOrderBookAsks[symbol]
		if bids == nil || asks == nil {
			return "", false
		}
		for _, item := range depth.Depth.Bids {
			if item.Qty == "0" {
				bids.Remove(item.Price)
			} else {
				bids.Put(item.Price, item.Qty)
			}
		}
		bo.spotWsOrderBookBids[symbol] = bids
		for _, item := range depth.Depth.Asks {
			if item.Qty == "0" {
				asks.Remove(item.Price)
			} else {
				asks.Put(item.Price, item.Qty)
			}
		}
		bo.spotWsOrderBookAsks[symbol] = asks
		return symbol, true
	}
	return "", false
}
func (bo *Bigone) spotWsHandleOrderBook5(symbol string, ch chan<- any) {
	bids := bo.spotWsOrderBookBids[symbol]
	asks := bo.spotWsOrderBookAsks[symbol]
	if bids.Size() < 5 || asks.Size() < 5 {
		return
	}
	obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
	obd.Symbol = symbol
	obd.Level = 5
	obd.Time = 0 // bigone 不提供
	obd.Bids = obd.Bids[:0]
	obd.Asks = obd.Asks[:0]

	it := bids.Iterator()
	for i := 0; i < 5; i++ {
		it.Next()
		val, _ := decimal.NewFromString(it.Value())
		tk := Ticker{Price: it.Key(), Quantity: val}
		obd.Bids = append(obd.Bids, tk)
	}
	it = asks.Iterator()
	for i := 0; i < 5; i++ {
		it.Next()
		val, _ := decimal.NewFromString(it.Value())
		tk := Ticker{Price: it.Key(), Quantity: val}
		obd.Asks = append(obd.Asks, tk)
	}
	ch <- obd
}

// = priv channel
func (bo *Bigone) SpotWsPrivateOpen() error {
	url := "wss://big.one/ws/v2"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bo.spotWsPrivateConn, _, err = dialer.Dial(url, http.Header{
		"Sec-WebSocket-Protocol": []string{"json"},
	})
	if err != nil {
		return errors.New(bo.Name() + " spot.ws.priv connect failed! " + err.Error())
	}

	reqId := gutils.RandomStr(8)
	req := fmt.Sprintf(`{"requestId":"%s", "authenticateCustomerRequest":{"token":"Bearer %s"}}`,
		reqId, bo.jwt())
	bo.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req))
	_, msg, err := bo.spotWsPrivateConn.ReadMessage()
	if err != nil {
		bo.SpotWsPrivateClose()
		return errors.New(bo.Name() + " spot.ws.priv recv login resp err:" + err.Error())
	}
	resp := struct {
		RequestId string `json:"requestId,omitempty"`
		Success   struct {
			Ok bool `json:"ok,omitempty"` // 200 is ok
		} `json:"success,omitempty"`
	}{}
	if err = json.Unmarshal(msg, &resp); err != nil {
		bo.SpotWsPrivateClose()
		return errors.New(bo.Name() + " spot.ws.priv auth resp err:" + err.Error())
	}
	if reqId != resp.RequestId || resp.Success.Ok != true {
		bo.SpotWsPrivateClose()
		return errors.New(bo.Name() + " spot.ws.priv auth fail:" + string(msg))
	}

	bo.spotWsPrivateClosedMtx.Lock()
	bo.spotWsPrivateClosed = false
	bo.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (bo *Bigone) SpotWsPrivateSubscribe(channels []string) {
	for _, c := range channels {
		if c == "orders" {
			req := fmt.Sprintf(`{"requestId": "%s", "subscribeAllViewerOrdersRequest":{}}`,
				gutils.RandomStr(8))
			bo.spotWsPrivateConnMtx.Lock()
			bo.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req))
			bo.spotWsPrivateConnMtx.Unlock()
		} else if c == "balance" {
			req := fmt.Sprintf(`{"requestId": "%s", "subscribeViewerAccountsRequest":{}}`,
				gutils.RandomStr(8))
			bo.spotWsPrivateConnMtx.Lock()
			bo.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(req))
			bo.spotWsPrivateConnMtx.Unlock()
		}
	}
}
func (bo *Bigone) SpotWsPrivateIsClosed() bool {
	bo.spotWsPrivateClosedMtx.RLock()
	defer bo.spotWsPrivateClosedMtx.RUnlock()
	return bo.spotWsPrivateClosed
}
func (bo *Bigone) SpotWsPrivateClose() {
	bo.spotWsPrivateClosedMtx.Lock()
	defer bo.spotWsPrivateClosedMtx.Unlock()
	if bo.spotWsPrivateClosed {
		return
	}
	bo.spotWsPrivateClosed = true
	bo.spotWsPrivateConn.Close()
}
func (bo *Bigone) SpotWsPrivateLoop(ch chan<- any) {
	defer bo.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 29 * time.Second
	pongWait := pingInterval + 2*time.Second
	bo.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bo.SpotWsPrivateIsClosed() {
				break
			}
			bo.spotWsPrivateConnMtx.Lock()
			bo.spotWsPrivateConn.WriteMessage(websocket.PingMessage, nil)
			bo.spotWsPrivateConnMtx.Unlock()
		}
	}()
	bo.spotWsPrivateConn.SetPongHandler(func(message string) error {
		bo.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, recv, err := bo.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !bo.SpotWsPrivateIsClosed() {
				ilog.Warning(bo.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		ilog.Rinfo(bo.Name() + " spot priv ws: " + string(recv))
		msg := boSpotWsPrivMsgPool.Get().(*BigoneSpotWsPrivMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(bo.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.OrderUpdate != nil {
			bo.spotWsHandleOrder(msg.OrderUpdate, ch)
		} else if msg.AccountUpdate != nil {
			bo.spotWsHandleAccountUpdate(msg.AccountUpdate, ch)
		} else if msg.AccountSnap != nil {
			bo.spotWsHandleAccountSnap(msg.AccountSnap, ch)
		} else if bytes.Contains(recv, []byte(`"heartbeat":`)) {
		} else if bytes.Contains(recv, []byte(`"success":`)) {
		} else {
			ilog.Error(bo.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		boSpotWsPrivMsgPool.Put(msg)
	}
}
func (bo *Bigone) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
	order := struct {
		Order struct {
			Symbol    string          `json:"market,omitempty"`
			OrderId   string          `json:"id,omitempty"`
			ClientId  string          `json:"clientOrderId,omitempty"`
			Price     decimal.Decimal `json:"price,omitempty"`
			Qty       decimal.Decimal `json:"amount,omitempty"`
			FilledQty decimal.Decimal `json:"filledAmount,omitempty"`
			AvgPrice  decimal.Decimal `json:"avgDealPrice,omitempty"`
			Status    string          `json:"state,omitempty"`
			Type      string          `json:"type,omitempty"`
			Side      string          `json:"side,omitempty"`
			FeeQty    decimal.Decimal `json:"filledFees,omitempty"`
			Time      string          `json:"createdAt,omitempty"`
			UTime     string          `json:"updatedAt,omitempty"`
		} `json:"order"`
	}{}
	if err := json.Unmarshal(data, &order); err == nil && order.Order.OrderId != "" {
		ctime, _ := time.Parse(time.RFC3339, order.Order.Time)
		utime, _ := time.Parse(time.RFC3339, order.Order.UTime)
		symbol := strings.ReplaceAll(order.Order.Symbol, "-", "")
		arr := strings.Split(order.Order.Symbol, "-")
		so := &SpotOrder{
			Symbol:    symbol,
			OrderId:   order.Order.OrderId,
			ClientId:  order.Order.ClientId,
			Price:     order.Order.Price,
			Qty:       order.Order.Qty,
			FilledQty: order.Order.FilledQty,
			FilledAmt: order.Order.FilledQty.Mul(order.Order.AvgPrice),
			Status:    bo.toStdOrderStatus(order.Order.Status),
			Type:      bo.toStdOrderType(order.Order.Type),
			Side:      bo.toStdSide(order.Order.Side),
			FeeQty:    order.Order.FeeQty.Neg(), // 换成负数
			CTime:     ctime.UnixMilli(),
			UTime:     utime.UnixMilli(),
		}
		if !so.FeeQty.IsZero() {
			so.FeeAsset = arr[1]
			if so.Side == "BUY" {
				so.FeeAsset = arr[0]
			}
		}
		ch <- so
	}
}
func (bo *Bigone) spotWsHandleAccountSnap(data json.RawMessage, ch chan<- any) {
	soL := struct {
		Accounts []struct {
			Symbol  string          `json:"asset"`
			Balance decimal.Decimal `json:"balance"`
			Locked  decimal.Decimal `json:"lockedBalance"`
		} `json:"accounts,omitempty"`
	}{}
	if err := json.Unmarshal(data, &soL); err == nil && len(soL.Accounts) > 0 {
		for _, v := range soL.Accounts {
			if v.Balance.IsZero() {
				continue
			}
			ch <- &SpotAsset{
				Symbol: v.Symbol,
				Avail:  v.Balance.Sub(v.Locked),
				Locked: v.Locked,
				Total:  v.Balance,
			}
		}
	}
}
func (bo *Bigone) spotWsHandleAccountUpdate(data json.RawMessage, ch chan<- any) {
	as := struct {
		Account struct {
			Symbol  string          `json:"asset"`
			Balance decimal.Decimal `json:"balance"`
			Locked  decimal.Decimal `json:"lockedBalance"`
		} `json:"account,omitempty"`
	}{}
	if err := json.Unmarshal(data, &as); err == nil && len(as.Account.Symbol) > 0 {
		ch <- &SpotAsset{
			Symbol: as.Account.Symbol,
			Avail:  as.Account.Balance.Sub(as.Account.Locked),
			Locked: as.Account.Locked,
			Total:  as.Account.Balance,
		}
	}
}
