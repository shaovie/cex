package cex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
)

var (
	okxWsPubMsgPool  sync.Pool
	okxWsPrivMsgPool sync.Pool
)

func init() {
	okxWsPubMsgPool = sync.Pool{
		New: func() any {
			return &OkxWsPubMsg{}
		},
	}
	okxWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &OkxWsPrivMsg{}
		},
	}
}
func (ok *Okx) SpotWsPublicOpen() error {
	url := "wss://ws.okx.com:8443/ws/v5/public"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	ok.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(ok.Name() + " spot.ws.public con failed! " + err.Error())
	}

	ok.spotWsPublicConnMtx.Lock()
	ok.spotWsPublicClosed = false
	ok.spotWsPublicConnMtx.Unlock()
	return nil
}
func (ok *Okx) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	type Arg struct {
		Channel  string `json:"channel"`
		InstId   string `json:"instId"`
		InstType string `json:"instType"`
	}
	req := struct {
		Id   string `json:"id"`
		Op   string `json:"op"`
		Args []*Arg `json:"args"`
	}{Id: gutils.RandomStr(8), Op: "subscribe"}
	req.Args = make([]*Arg, 0, 2)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbolS := ok.getSpotSymbol(sym); symbolS != "" {
					arg := Arg{Channel: "books5", InstId: symbolS, InstType: "SPOT"}
					req.Args = append(req.Args, &arg)
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, v := range symbolArr {
					if sym := ok.getSpotSymbol(v); sym != "" {
						arg := Arg{Channel: "tickers", InstId: sym, InstType: "SPOT"}
						req.Args = append(req.Args, &arg)
					}
				}
			}
		}
	}
	if len(req.Args) > 0 {
		subData, _ := json.Marshal(&req)
		ok.spotWsPublicConnMtx.Lock()
		ok.spotWsPublicConn.WriteMessage(websocket.TextMessage, subData)
		ok.spotWsPublicConnMtx.Unlock()
	}
}
func (ok *Okx) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	type Arg struct {
		Channel  string `json:"channel"`
		InstId   string `json:"instId"`
		InstType string `json:"instType"`
	}
	req := struct {
		Op   string `json:"op"`
		Args []*Arg `json:"args"`
	}{Op: "unsubscribe"}
	req.Args = make([]*Arg, 0, 2)
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbolS := ok.getSpotSymbol(sym); symbolS != "" {
					arg := Arg{Channel: "books5", InstId: symbolS, InstType: "SPOT"}
					req.Args = append(req.Args, &arg)
				}
			}
		} else if arr[0] == "ticker" {
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr := strings.Split(arr[1], ",")
				for _, v := range symbolArr {
					if sym := ok.getSpotSymbol(v); sym != "" {
						arg := Arg{Channel: "tickers", InstId: sym, InstType: "SPOT"}
						req.Args = append(req.Args, &arg)
					}
				}
			}
		}
	}
	if len(req.Args) > 0 {
		subData, _ := json.Marshal(&req)
		ok.spotWsPublicConnMtx.Lock()
		ok.spotWsPublicConn.WriteMessage(websocket.TextMessage, subData)
		ok.spotWsPublicConnMtx.Unlock()
	}
}
func (ok *Okx) SpotWsPublicTickerPoolPut(v any) {
	spotWsPublicTickerPool.Put(v)
}
func (ok *Okx) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (ok *Okx) SpotWsPublicLoop(ch chan<- any) {
	defer ok.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 28 * time.Second
	pongWait := pingInterval + 2*time.Second
	ok.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	ok.spotWsPublicConn.SetPongHandler(func(string) error {
		ok.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		var pingMsg = []byte("ping")
		for range ticker.C {
			if ok.SpotWsPublicIsClosed() {
				break
			}
			ok.spotWsPublicConnMtx.Lock()
			ok.spotWsPublicConn.WriteMessage(websocket.TextMessage, pingMsg)
			ok.spotWsPublicConnMtx.Unlock()
		}
	}()

	for {
		_, recv, err := ok.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !ok.SpotWsPublicIsClosed() {
				ilog.Warning(ok.Name() + " spot.ws.public read: " + err.Error())
			}
			break
		}
		// ilog.Rinfo(ok.Name() + " spot pub ws: " + string(recv))
		if len(recv) == 4 && bytes.Equal(recv, []byte("pong")) {
			ok.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		msg := okxWsPubMsgPool.Get().(*OkxWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(ok.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if len(msg.Event) == 0 {
			if msg.Arg.Channel == "books5" {
				ok.spotWsHandleOrderBook5(msg.Arg.Symbol, recv, ch)
			} else if msg.Arg.Channel == "tickers" {
				ok.spotWsHandle24hTickers(msg.Data, ch)
			}
		} else if msg.Event == "error" {
			ilog.Error(ok.Name() + " spot.ws.public recv error event: " + string(recv))
		} else if msg.Event == "subscribe" {
		} else if msg.Event == "unsubscribe" {
		} else if msg.Event == "notice" {
		} else if msg.Event == "channel-conn-count" {
		} else if msg.Event == "channel-conn-count-error" {
			ilog.Error(ok.Name() + " spot.ws.public recv err: " + string(recv))
			okxWsPubMsgPool.Put(msg)
			break
		} else {
			ilog.Error(ok.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		okxWsPubMsgPool.Put(msg)
	}
}
func (ok *Okx) SpotWsPublicIsClosed() bool {
	ok.spotWsPublicConnMtx.Lock()
	defer ok.spotWsPublicConnMtx.Unlock()
	return ok.spotWsPublicClosed
}
func (ok *Okx) SpotWsPublicClose() {
	ok.spotWsPublicConnMtx.Lock()
	defer ok.spotWsPublicConnMtx.Unlock()
	if ok.spotWsPublicClosed {
		return
	}
	ok.spotWsPublicClosed = true
	ok.spotWsPublicConn.Close()
}
func (ok *Okx) spotWsHandleOrderBook5(symbol string, data json.RawMessage, ch chan<- any) {
	firstDash := strings.Index(symbol, "-")
	if firstDash == -1 {
		return
	}
	sym := symbol[:firstDash] + symbol[firstDash+1:]
	depthL := OkxOrderBooks{Data: make([]OkxOrderBook, 0, 1)}
	depthL.Data = append(depthL.Data, OkxOrderBook{
		Bids: make([][2]decimal.Decimal, 0, 5),
		Asks: make([][2]decimal.Decimal, 0, 5),
	})
	if err := json.Unmarshal(data, &depthL); err == nil && len(depthL.Data) > 0 {
		for _, depth := range depthL.Data {
			if len(depth.Bids) != len(depth.Asks) {
				ilog.Error(ok.Name() + " spot.ws.public " + symbol + " exception")
				continue
			}
			obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
			obd.Symbol = sym
			obd.Level = len(depth.Bids)
			obd.Time, _ = strconv.ParseInt(depth.Time, 10, 64)
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
}
func (ok *Okx) spotWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	tickers := ok.spotWsPublicTickerInnerPool.Get().([]Okx24hTicker)
	defer ok.spotWsPublicTickerInnerPool.Put(tickers)
	if err := json.Unmarshal(data, &tickers); err == nil {
		for _, tk := range tickers {
			firstDash := strings.Index(tk.Symbol, "-")
			if firstDash == -1 {
				continue
			}
			sym := tk.Symbol[:firstDash] + tk.Symbol[firstDash+1:]
			t := spotWsPublicTickerPool.Get().(*Spot24hTicker)
			t.Symbol = sym
			t.LastPrice = tk.Last
			t.Volume = tk.Volume
			t.QuoteVolume = tk.QuoteVolume
			ch <- t
		}
	}
}

// priv
func (ok *Okx) SpotWsPrivateOpen() error {
	url := "wss://ws.okx.com:8443/ws/v5/private"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	ok.spotWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(ok.Name() + " connect failed! " + err.Error())
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	loginData := fmt.Sprintf("{"+
		"\"op\": \"login\",\"args\":[{"+
		"\"apiKey\":\"%s\","+
		"\"passphrase\":\"%s\","+
		"\"timestamp\":\"%s\","+
		"\"sign\":\"%s\"}]}",
		ok.apikey, ok.passwd, ts, ok.sign(ts+"GET"+"/users/self/verify"))
	if err := ok.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(loginData)); err != nil {
		ok.SpotWsPrivateClose()
		return errors.New(ok.Name() + " spot.ws.priv send login err:" + err.Error())
	}
	_, msg, err := ok.spotWsPrivateConn.ReadMessage()
	if err != nil {
		ok.SpotWsPrivateClose()
		return errors.New(ok.Name() + " spot.ws.priv recv login resp err:" + err.Error())
	}

	recv := struct {
		Event string `json:"event,omitempty"`
		Code  string `json:"code,omitempty"`
		Msg   string `json:"msg,omitempty"`
	}{}
	if err = json.Unmarshal(msg, &recv); err != nil {
		ok.SpotWsPrivateClose()
		return errors.New(ok.Name() + " spot.ws.priv login resp err:" + err.Error())
	}
	if recv.Event != "login" {
		ok.SpotWsPrivateClose()
		return errors.New(ok.Name() + " spot.ws.priv login failed! err event:" + string(msg))
	}
	if recv.Code != "0" {
		ok.SpotWsPrivateClose()
		return errors.New(ok.Name() + " spot.ws.priv login error! " + string(recv.Msg))
	}

	ok.spotWsPrivateConnMtx.Lock()
	ok.spotWsPrivateClosed = false
	ok.spotWsPrivateConnMtx.Unlock()
	return nil
}
func (ok *Okx) SpotWsPrivateSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	type Arg struct {
		Channel  string `json:"channel"`
		InstType string `json:"instType,omitempty"`
	}
	for _, c := range channels {
		var arg *Arg
		if c == "orders" {
			arg = &Arg{Channel: "orders", InstType: "SPOT"}
		} else {
			continue
		}
		subscribe := struct {
			Op   string `json:"op"`
			Args []*Arg `json:"args"`
		}{Op: "subscribe", Args: []*Arg{arg}}
		subData, _ := json.Marshal(&subscribe)
		ok.spotWsPrivateConnMtx.Lock()
		ok.spotWsPrivateConn.WriteMessage(websocket.TextMessage, subData)
		ok.spotWsPrivateConnMtx.Unlock()
	}
}

type OkxWsPrivMsg struct {
	// api
	RequestId string `json:"id,omitempty"`
	Op        string `json:"op,omitempty"`
	Code      string `json:"code,omitempty"`
	Msg       string `json:"msg,omitempty"`

	// push
	Arg struct {
		Channel string `json:"channel,omitempty"`
	} `json:"arg,omitempty"`
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (v *OkxWsPrivMsg) reset() {
	v.RequestId = ""
	v.Op = ""
	v.Code = ""
	v.Msg = ""
	v.Arg.Channel = ""
	v.Event = ""
	v.Data = nil
}
func (ok *Okx) SpotWsPrivateLoop(ch chan<- any) {
	defer ok.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 25 * time.Second
	pongWait := pingInterval + 2*time.Second
	ok.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	ok.spotWsPrivateConn.SetPongHandler(func(string) error {
		ok.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		var pingMsg = []byte("ping")
		for range ticker.C {
			if ok.SpotWsPrivateIsClosed() {
				break
			}
			ok.spotWsPrivateConnMtx.Lock()
			ok.spotWsPrivateConn.WriteMessage(websocket.TextMessage, pingMsg)
			ok.spotWsPrivateConnMtx.Unlock()
		}
	}()

	for {
		_, recv, err := ok.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !ok.SpotWsPrivateIsClosed() {
				ilog.Warning(ok.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		ilog.Rinfo(ok.Name() + " spot priv ws: " + string(recv))
		if len(recv) == 4 && bytes.Equal(recv, []byte("pong")) {
			ok.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			continue
		}
		msg := okxWsPrivMsgPool.Get().(*OkxWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(ok.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Event == "" && msg.RequestId == "" {
			if msg.Arg.Channel == "orders" {
				ok.spotWsHandleOrder(msg.Data, ch)
			}
		} else if msg.RequestId != "" {
			if msg.Op == "order" {
				ok.spotWsHandlePlaceOrderResp(msg.RequestId, msg.Data, ch)
			} else if msg.Op == "cancel-order" {
				ok.spotWsHandleCancelOrderResp(msg.RequestId, msg.Data, ch)
			}
		} else if msg.Event == "error" {
			ilog.Error(ok.Name() + " spot.ws.priv recv error event: " + string(recv))
		} else if msg.Event == "login" {
		} else if msg.Event == "subscribe" {
		} else if msg.Event == "notice" {
		} else if msg.Event == "channel-conn-count" {
			// {"event":"channel-conn-count","channel":"orders","connCount":"1"
		} else if msg.Event == "channel-conn-count-error" {
			ilog.Error(ok.Name() + " spot.ws.priv recv err: " + string(recv))
			okxWsPrivMsgPool.Put(msg)
			break
		} else {
			ilog.Error(ok.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
		}
	END:
		okxWsPrivMsgPool.Put(msg)
	}
}
func (ok *Okx) SpotWsPrivateIsClosed() bool {
	ok.spotWsPrivateConnMtx.Lock()
	defer ok.spotWsPrivateConnMtx.Unlock()
	return ok.spotWsPrivateClosed
}
func (ok *Okx) SpotWsPrivateClose() {
	ok.spotWsPrivateConnMtx.Lock()
	defer ok.spotWsPrivateConnMtx.Unlock()
	if ok.spotWsPrivateClosed {
		return
	}
	ok.spotWsPrivateClosed = true
	ok.spotWsPrivateConn.Close()
}
func (ok *Okx) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
	orders := []struct {
		InstType    string          `json:"instType"`
		Symbol      string          `json:"instId"`
		OrderId     string          `json:"ordId"`
		ClientId    string          `json:"clOrdId,omitempty"`
		Price       string          `json:"px,omitempty"`
		Qty         decimal.Decimal `json:"sz,omitempty"`
		ExecutedQty string          `json:"accFillSz,omitempty"`
		AvgPrice    string          `json:"avgPx,omitempty"`
		Status      string          `json:"state"`
		Type        string          `json:"ordType,omitempty"`
		Side        string          `json:"side,omitempty"`
		FeeCoin     string          `json:"feeCcy,omitempty"`
		FeeQty      string          `json:"fee,omitempty"`
		Time        string          `json:"cTime"`
		UTime       string          `json:"uTime"`
	}{}
	if err := json.Unmarshal(data, &orders); err == nil && len(orders) > 0 {
		for _, order := range orders {
			if order.InstType != "SPOT" {
				continue
			}
			t, _ := strconv.ParseInt(order.Time, 10, 64)
			ut, _ := strconv.ParseInt(order.UTime, 10, 64)
			avgPrice, _ := decimal.NewFromString(order.AvgPrice)
			sbA := strings.Split(order.Symbol, "-")
			if len(sbA) < 2 {
				continue
			}
			sm := sbA[0] + sbA[1]
			so := &SpotOrder{
				Symbol:   sm,
				OrderId:  order.OrderId,
				ClientId: order.ClientId,
				Qty:      order.Qty,
				Status:   ok.toStdOrderStatus(order.Status),
				Type:     ok.toStdOrderType(order.Type),
				Side:     ok.toStdSide(order.Side),
				FeeAsset: order.FeeCoin,
				CTime:    t,
				UTime:    ut,
			}
			so.Price, _ = decimal.NewFromString(order.Price)
			so.FilledQty, _ = decimal.NewFromString(order.ExecutedQty)
			so.FilledAmt = so.FilledQty.Mul(avgPrice)
			so.FeeQty, _ = decimal.NewFromString(order.FeeQty)
			ch <- so
		}
	}
}
func (ok *Okx) spotWsHandlePlaceOrderResp(reqId string, data json.RawMessage, ch chan<- any) {
	ret := []struct {
		OrderId  string `json:"ordId,omitempty"`
		ClientId string `json:"clOrdId,omitempty"`
		Code     string `json:"sCode,omitempty"`
		Msg      string `json:"sMsg,omitempty"`
	}{}
	if err := json.Unmarshal(data, &ret); err != nil {
		ilog.Error(ok.Name() + " spot.ws.priv handle place order resp: " + err.Error())
		return
	}
	for _, ord := range ret {
		if ord.Code != "0" {
			order := &SpotOrder{
				RequestId: reqId,
				Err:       ord.Msg,
			}
			ch <- order
			continue
		}
		ch <- &SpotOrder{
			RequestId: reqId,
			OrderId:   ord.OrderId,
			ClientId:  ord.ClientId,
		}
	}
}
func (ok *Okx) spotWsHandleCancelOrderResp(reqId string, data json.RawMessage, ch chan<- any) {
	ret := []struct {
		OrderId string `json:"ordId,omitempty"`
		Code    string `json:"sCode,omitempty"`
		Msg     string `json:"sMsg,omitempty"`
	}{}
	if err := json.Unmarshal(data, &ret); err != nil {
		ilog.Error(ok.Name() + " spot.ws.priv handle cancel order resp: " + err.Error())
		return
	}
	for _, ord := range ret {
		if ord.Code != "0" {
			ilog.Error(ok.Name() + " spot.ws.priv cancel order fail! " + string(data))
		}
	}
}
func (ok *Okx) SpotWsPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, timeInForce, orderType string) (string, error) {
	if ok.SpotWsPrivateIsClosed() {
		return "", errors.New(ok.Name() + " spot priv ws closed")
	}
	type Arg struct {
		Symbol    string `json:"instId,omitempty"`
		TradeMode string `json:"tdMode,omitempty"`
		Type      string `json:"ordType,omitempty"`
		Size      string `json:"sz,omitempty"`
		Side      string `json:"side,omitempty"`
		ClientId  string `json:"clOrdId,omitempty"`
		Price     string `json:"px,omitempty"`
	}

	if timeInForce == "IOC" || timeInForce == "FOK" {
		orderType = timeInForce
	}
	symbolS := ok.getSpotSymbol(symbol)
	arg := Arg{
		Symbol:    symbolS,
		TradeMode: "cash",
		ClientId:  clientId,
		Side:      ok.fromStdSide(side),
		Type:      ok.fromStdOrderType(orderType),
		Size:      qty.String(),
	}
	if orderType == "LIMIT" {
		arg.Price = price.String()
	}
	type Req struct {
		Id   string `json:"id"`
		Op   string `json:"op"`
		Args []Arg  `json:"args"`
	}
	req := Req{Id: gutils.RandomStr(16), Op: "order", Args: []Arg{arg}}
	reqJson, _ := json.Marshal(req)
	ok.spotWsPrivateConnMtx.Lock()
	defer ok.spotWsPrivateConnMtx.Unlock()
	if err := ok.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(ok.Name() + " send fail: " + err.Error())
	}
	return req.Id, nil
}
func (ok *Okx) SpotWsCancelOrder(symbol, orderId, cltId string) error {
	if ok.SpotWsPrivateIsClosed() {
		return errors.New(ok.Name() + " spot.ws.priv ws closed")
	}
	type Arg struct {
		Symbol  string `json:"instId"`
		OrderId string `json:"ordId,omitempty"`
		CltId   string `json:"ordId,omitempty"`
	}
	arg := Arg{
		Symbol:  ok.getSpotSymbol(symbol),
		OrderId: orderId,
		CltId:   cltId, // ordId和clOrdId必须传一个, 若传两个，以 ordId 为主
	}
	type Req struct {
		Id   string `json:"id"`
		Op   string `json:"op"`
		Args []Arg  `json:"args"`
	}
	req := Req{Id: gutils.RandomStr(16), Op: "cancel-order", Args: []Arg{arg}}
	reqJson, _ := json.Marshal(req)
	ok.spotWsPrivateConnMtx.Lock()
	defer ok.spotWsPrivateConnMtx.Unlock()
	if err := ok.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return errors.New(ok.Name() + " send fail: " + err.Error())
	}
	return nil
}
