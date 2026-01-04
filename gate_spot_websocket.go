package cex

import (
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
	gtWsPubMsgPool  sync.Pool
	gtWsPrivMsgPool sync.Pool
)

func init() {
	gtWsPubMsgPool = sync.Pool{
		New: func() any {
			return &GateWsSpotPubMsg{}
		},
	}
	gtWsPrivMsgPool = sync.Pool{
		New: func() any {
			return &GtWsPrivMsg{}
		},
	}
}
func (gt *Gate) SpotWsPublicOpen() error {
	url := "wss://api.gateio.ws/ws/v4/"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	gt.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(gt.Name() + " spot.ws.public con failed! " + err.Error())
	}
	gt.spotWsPublicClosedMtx.Lock()
	gt.spotWsPublicClosed = false
	gt.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (gt *Gate) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	now := time.Now().Unix()
	arg := GateSubscribeArg{Time: now, Event: "subscribe"}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			arg.Channel = "spot.order_book"
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbol := gt.getSpotSymbol(sym); symbol != "" {
					arg.Payload = []string{symbol, "5", "100ms"}
					req, _ := json.Marshal(&arg)
					gt.spotWsPublicConnMtx.Lock()
					gt.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
					gt.spotWsPublicConnMtx.Unlock()
				}
			}
		} else if arr[0] == "ticker" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			symbolList := make([]string, 0, len(symbolArr))
			for _, v := range symbolArr {
				if sym := gt.getSpotSymbol(v); sym != "" {
					symbolList = append(symbolList, sym)
				}
			}
			arg.Channel = "spot.tickers"
			arg.Payload = symbolList
			if len(arg.Payload) > 0 {
				req, _ := json.Marshal(&arg)
				gt.spotWsPublicConnMtx.Lock()
				gt.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
				gt.spotWsPublicConnMtx.Unlock()
			}
		}
	}
}
func (gt *Gate) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	now := time.Now().Unix()
	arg := GateSubscribeArg{Time: now, Event: "unsubscribe"}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "orderbook5" {
			if len(arr) < 2 || len(arr[1]) == 0 {
				continue
			}
			arg.Channel = "spot.order_book"
			symbolArr := strings.Split(arr[1], ",")
			for _, sym := range symbolArr {
				if symbol := gt.getSpotSymbol(sym); symbol != "" {
					arg.Payload = []string{symbol, "5", "100ms"}
					req, _ := json.Marshal(&arg)
					gt.spotWsPublicConnMtx.Lock()
					gt.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
					gt.spotWsPublicConnMtx.Unlock()
				}
			}
		} else if arr[0] == "ticker" {
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			symbolList := make([]string, 0, len(symbolArr))
			for _, v := range symbolArr {
				if sym := gt.getSpotSymbol(v); sym != "" {
					symbolList = append(symbolList, sym)
				}
			}
			arg.Channel = "spot.tickers"
			arg.Payload = symbolList
			if len(arg.Payload) > 0 {
				req, _ := json.Marshal(&arg)
				gt.spotWsPublicConnMtx.Lock()
				gt.spotWsPublicConn.WriteMessage(websocket.TextMessage, req)
				gt.spotWsPublicConnMtx.Unlock()
			}
		}
	}
}
func (gt *Gate) SpotWsPublicTickerPoolPut(v any) {
	wsPublicTickerPool.Put(v)
}
func (gt *Gate) SpotWsPublicOrderBook5PoolPut(v any) {
	wsPublicOrderBook5Pool.Put(v)
}
func (gt *Gate) SpotWsPublicLoop(ch chan<- any) {
	defer gt.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 21 * time.Second
	pongWait := pingInterval + 2*time.Second
	gt.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if gt.SpotWsPublicIsClosed() {
				break
			}
			s := fmt.Sprintf(`{"time":%d,"channel":"spot.ping"}`, time.Now().Unix())
			gt.spotWsPublicConnMtx.Lock()
			gt.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(s))
			gt.spotWsPublicConnMtx.Unlock()
		}
	}()

	for {
		_, recv, err := gt.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !gt.SpotWsPublicIsClosed() {
				ilog.Warning(gt.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := gtWsPubMsgPool.Get().(*GateWsSpotPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(gt.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}

		if msg.Channel == "spot.order_book" {
			if msg.Event == "update" {
				gt.spotWsHandleOrderBook(msg.Data, ch)
			}
		} else if msg.Channel == "spot.tickers" {
			if msg.Event == "update" {
				gt.spotWsHandle24hTickers(msg.Data, ch)
			}
		} else if msg.Channel == "spot.pong" {
			gt.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		} else {
			ilog.Error(gt.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		gtWsPubMsgPool.Put(msg)
	}
}
func (gt *Gate) SpotWsPublicIsClosed() bool {
	gt.spotWsPublicClosedMtx.RLock()
	defer gt.spotWsPublicClosedMtx.RUnlock()
	return gt.spotWsPublicClosed
}
func (gt *Gate) SpotWsPublicClose() {
	gt.spotWsPublicClosedMtx.Lock()
	defer gt.spotWsPublicClosedMtx.Unlock()
	if gt.spotWsPublicClosed {
		return
	}
	gt.spotWsPublicClosed = true
	gt.spotWsPublicConn.Close()
}
func (gt *Gate) spotWsHandleOrderBook(data json.RawMessage, ch chan<- any) {
	depth := gt.spotWsPublicOrderBookInnerPool.Get().(*GateSpotOrderBook)
	defer gt.spotWsPublicOrderBookInnerPool.Put(depth)
	depth.Bids = depth.Bids[:0]
	depth.Asks = depth.Asks[:0]
	if err := easyjson.Unmarshal(data, depth); err == nil {
		if len(depth.Bids) != len(depth.Asks) {
			ilog.Error(gt.Name() + " spot.ws.public " + depth.Symbol + " orderbook exception")
			return
		}
		obd := wsPublicOrderBook5Pool.Get().(*OrderBookDepth)
		obd.Symbol = strings.ReplaceAll(depth.Symbol, "_", "")
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
func (gt *Gate) spotWsHandle24hTickers(data json.RawMessage, ch chan<- any) {
	ticker := gt.spotWsPublicTickerInnerPool.Get().(*GateSpot24hTicker)
	defer gt.spotWsPublicTickerInnerPool.Put(ticker)
	if err := json.Unmarshal(data, ticker); err == nil {
		tk := wsPublicTickerPool.Get().(*Pub24hTicker)
		tk.Symbol = strings.ReplaceAll(ticker.Symbol, "_", "")
		tk.LastPrice = ticker.Last
		tk.Volume = ticker.Volume
		tk.QuoteVolume = ticker.QuoteVolume
		ch <- tk
	}
}

// = priv channel
func (gt *Gate) SpotWsPrivateOpen() error {
	url := "wss://api.gateio.ws/ws/v4/"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	gt.spotWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(gt.Name() + " connect failed! " + err.Error())
	}

	arg := struct {
		Time    int64  `json:"time"`
		Channel string `json:"channel"`
		Event   string `json:"event"`
		Payload struct {
			Apikey    string `json:"api_key"`
			Sign      string `json:"signature"`
			Timestamp string `json:"timestamp"`
			ReqId     string `json:"req_id"`
		} `json:"payload"`
	}{
		Channel: "spot.login",
		Event:   "api",
	}
	now := time.Now().Unix()
	arg.Payload.Apikey = gt.apikey
	arg.Payload.Sign = gt.wsLoginSign(arg.Channel, "api", now)
	arg.Payload.Timestamp = strconv.FormatInt(now, 10)
	arg.Payload.ReqId = gutils.RandomStr(8)
	req, _ := json.Marshal(&arg)
	gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	_, msg, err := gt.spotWsPrivateConn.ReadMessage()
	if err != nil {
		gt.SpotWsPrivateClose()
		return errors.New(gt.Name() + " spot.ws.priv recv login resp err:" + err.Error())
	}
	resp := struct {
		RequestId string `json:"request_id,omitempty"`
		Header    struct {
			Channel string `json:"channel,omitempty"`
			Status  string `json:"status,omitempty"` // 200 is ok
		} `json:"header,omitempty"`
	}{}
	if err = json.Unmarshal(msg, &resp); err != nil {
		gt.SpotWsPrivateClose()
		return errors.New(gt.Name() + " spot.ws.priv login resp err:" + err.Error())
	}
	if resp.Header.Channel != "spot.login" || resp.Header.Status != "200" {
		gt.SpotWsPrivateClose()
		return errors.New(gt.Name() + " spot.ws.priv login fail:" + string(msg))
	}

	gt.spotWsPrivateClosedMtx.Lock()
	gt.spotWsPrivateClosed = false
	gt.spotWsPrivateClosedMtx.Unlock()
	return nil
}
func (gt *Gate) SpotWsPrivateSubscribe(channels []string) {
	now := time.Now().Unix()
	arg := GateSubscribeArg{Time: now, Event: "subscribe"}
	for _, c := range channels {
		if c == "orders" {
			channel := "spot.orders"
			arg.Channel = channel
			arg.Payload = []string{"!all"}
			arg.Auth = &GatePrivAuth{
				Method: "api_key",
				Key:    gt.apikey,
				Sign:   gt.wsSign(channel, "subscribe", now),
			}
			req, _ := json.Marshal(&arg)
			gt.spotWsPrivateConnMtx.Lock()
			if err := gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
				ilog.Warning(gt.Name() + " spot.ws.priv subscribe net error! " + err.Error())
			}
			gt.spotWsPrivateConnMtx.Unlock()
		} else if c == "balance" {
			channel := "spot.balances"
			arg.Channel = channel
			arg.Auth = &GatePrivAuth{
				Method: "api_key",
				Key:    gt.apikey,
				Sign:   gt.wsSign(channel, "subscribe", now),
			}
			req, _ := json.Marshal(&arg)
			gt.spotWsPrivateConnMtx.Lock()
			if err := gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
				ilog.Warning(gt.Name() + " spot.ws.priv subscribe net error! " + err.Error())
			}
			gt.spotWsPrivateConnMtx.Unlock()
		}
	}
}
func (gt *Gate) SpotWsPrivateIsClosed() bool {
	gt.spotWsPrivateClosedMtx.RLock()
	defer gt.spotWsPrivateClosedMtx.RUnlock()
	return gt.spotWsPrivateClosed
}
func (gt *Gate) SpotWsPrivateClose() {
	gt.spotWsPrivateClosedMtx.Lock()
	defer gt.spotWsPrivateClosedMtx.Unlock()
	if gt.spotWsPrivateClosed {
		return
	}
	gt.spotWsPrivateClosed = true
	gt.spotWsPrivateConn.Close()
}

type GtWsPrivMsg struct {
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"result,omitempty"`

	// api
	RequestId string `json:"request_id,omitempty"`
	Ack       bool   `json:"ack,omitempty"`
	Header    struct {
		Channel string `json:"channel,omitempty"`
		Status  string `json:"status,omitempty"` // 200 is ok
	} `json:"header,omitempty"`
	RespData struct {
		Result json.RawMessage `json:"result,omitempty"`
		Errs   struct {
			Label   string `json:"label,omitempty"`
			Message string `json:"message,omitempty"`
		} `json:"errs,omitempty"`
	} `json:"data,omitempty"`
}

func (v *GtWsPrivMsg) reset() {
	v.Channel = ""
	v.Event = ""
	v.Data = nil
	v.RequestId = ""
	v.Ack = false
	v.Header.Channel = ""
	v.RespData.Result = nil
	v.RespData.Errs.Label = ""
	v.RespData.Errs.Message = ""
}
func (gt *Gate) SpotWsPrivateLoop(ch chan<- any) {
	defer gt.SpotWsPrivateClose()
	defer close(ch)

	pingInterval := 23 * time.Second
	pongWait := pingInterval + 2*time.Second
	gt.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if gt.SpotWsPrivateIsClosed() {
				break
			}
			s := fmt.Sprintf(`{"time":%d,"channel":"spot.ping"}`, time.Now().Unix())
			gt.spotWsPrivateConnMtx.Lock()
			gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(s))
			gt.spotWsPrivateConnMtx.Unlock()
		}
	}()

	for {
		_, recv, err := gt.spotWsPrivateConn.ReadMessage()
		if err != nil {
			if !gt.SpotWsPrivateIsClosed() {
				ilog.Warning(gt.Name() + " spot.ws.priv channel read: " + err.Error())
			}
			break
		}
		if gt.debug {
			ilog.Rinfo(gt.Name() + " spot priv ws: " + string(recv))
		}
		msg := gtWsPrivMsgPool.Get().(*GtWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(gt.Name() + " spot.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.RequestId != "" { // ws api
			if msg.Ack != true { // ack 忽略
				if msg.Header.Channel == "spot.order_place" {
					gt.spotWsHandlePlaceOrderResp(msg.RequestId,
						msg.RespData.Errs.Message, msg.RespData.Result, ch)
				} else if msg.Header.Channel == "spot.order_cancel" {
					gt.spotWsHandleCancelOrderResp(msg.RespData.Errs.Message)
				} else if msg.Header.Channel == "spot.login" {
					if msg.Header.Status != "200" {
						ilog.Error(gt.Name() + " spot.ws.priv login fail: " + msg.RespData.Errs.Message)
						gtWsPrivMsgPool.Put(msg)
						break // exit
					}
				}
			}
		} else {
			if msg.Channel == "spot.orders" {
				if msg.Event != "subscribe" {
					gt.spotWsHandleOrder(msg.Data, ch)
				}
			} else if msg.Channel == "spot.balances" {
				if msg.Event != "subscribe" {
					gt.spotWsHandleBalanceUpdate(msg.Data, ch)
				}
			} else if msg.Channel == "spot.pong" {
				gt.spotWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
			} else {
				ilog.Error(gt.Name() + " spot.ws.priv recv unknown msg: " + string(recv))
			}
		}
	END:
		gtWsPrivMsgPool.Put(msg)
	}
}
func (gt *Gate) spotWsHandleOrder(data json.RawMessage, ch chan<- any) {
	orders := []struct {
		Symbol       string          `json:"currency_pair,omitempty"`
		OrderId      string          `json:"id,omitempty"`
		ClientId     string          `json:"text,omitempty"`
		Price        decimal.Decimal `json:"price,omitempty"`
		Qty          decimal.Decimal `json:"amount,omitempty"`
		CummQuoteQty decimal.Decimal `json:"filled_total,omitempty"`
		ExecutedQty  decimal.Decimal `json:"filled_amount,omitempty"`
		Left         decimal.Decimal `json:"left,omitempty"`
		Status       string          `json:"status,omitempty"`
		Type         string          `json:"type,omitempty"`
		TimeInForce  string          `json:"time_in_force,omitempty"` // GTC/FOK/IOC
		Side         string          `json:"side,omitempty"`
		FeeCoin      string          `json:"fee_currency,omitempty"`
		FeeQty       decimal.Decimal `json:"fee,omitempty"`
		GtQty        decimal.Decimal `json:"gt_fee,omitempty"`    //
		Event        string          `json:"event,omitempty"`     //
		FinishAs     string          `json:"finish_as,omitempty"` //
		Time         string          `json:"create_time_ms,omitempty"`
		UTime        string          `json:"update_time_ms,omitempty"`
	}{}
	if err := json.Unmarshal(data, &orders); err == nil && len(orders) > 0 {
		for _, order := range orders {
			clientId := ""
			idx := strings.Index(order.ClientId, "t-")
			if idx != -1 && len(order.ClientId) > 2 {
				clientId = order.ClientId[2:]
			}
			if !order.GtQty.IsZero() {
				order.FeeCoin = "GT"
				order.FeeQty = order.GtQty
			}
			if order.Left.IsPositive() && order.Qty.GreaterThan(order.Left) {
				order.Status = "partially_filled"
			}
			if order.Event == "put" {
				order.Status = "open"
			} else if order.Event == "update" {
				if order.FinishAs == "open" {
					order.Status = "partially_filled"
				}
			} else if order.Event == "finish" {
				if order.FinishAs == "filled" {
					order.Status = "filled"
				} else if order.FinishAs == "cancelled" {
					order.Status = "cancelled"
				}
			}
			if order.Status == "" {
				ilog.Error(gt.Name() + " unknow s status" + string(data))
			}
			t, _ := strconv.ParseInt(order.Time, 10, 64)
			ut, _ := strconv.ParseInt(order.UTime, 10, 64)
			ch <- &SpotOrder{
				Symbol:      strings.ReplaceAll(order.Symbol, "_", ""),
				OrderId:     order.OrderId,
				ClientId:    clientId,
				Price:       order.Price,
				Qty:         order.Qty,
				FilledQty:   order.ExecutedQty,
				FilledAmt:   order.CummQuoteQty,
				Status:      gt.toStdOrderStatus(order.Status),
				Type:        gt.toStdOrderType(order.Type),
				TimeInForce: gt.toStdTimeInForce(order.TimeInForce),
				Side:        gt.toStdSide(order.Side),
				FeeQty:      order.FeeQty.Neg(), // 换成负数
				FeeAsset:    order.FeeCoin,
				CTime:       t,
				UTime:       ut,
			}
		}
	}
}
func (gt *Gate) spotWsHandleBalanceUpdate(data json.RawMessage, ch chan<- any) {
	res := []struct {
		Symbol  string          `json:"currency"` // symbol
		Total   decimal.Decimal `json:"total"`
		Balance decimal.Decimal `json:"available"`
		Locked  decimal.Decimal `json:"freeze"`
	}{}
	if err := json.Unmarshal(data, &res); err == nil && len(res) > 0 {
		for _, as := range res {
			ch <- &SpotAsset{
				Symbol: as.Symbol,
				Avail:  as.Balance,
				Locked: as.Locked,
				Total:  as.Total,
			}
		}
	}
}
func (gt *Gate) spotWsHandlePlaceOrderResp(reqId, errS string,
	data json.RawMessage, ch chan<- any) {
	if errS != "" {
		order := &SpotOrder{
			RequestId: reqId,
			Err:       errS,
		}
		ch <- order
		return
	}

	ret := struct {
		OrderId  string `json:"id,omitempty"`
		ClientId string `json:"text,omitempty"`
	}{}
	if err := json.Unmarshal(data, &ret); err != nil {
		ilog.Error(gt.Name() + " spot.ws.priv handle place order resp: " + err.Error())
		return
	}
	clientId := ""
	idx := strings.Index(ret.ClientId, "t-")
	if idx != -1 && len(ret.ClientId) > 2 {
		clientId = ret.ClientId[2:]
	}
	ch <- &SpotOrder{
		RequestId: reqId,
		OrderId:   ret.OrderId,
		ClientId:  clientId,
	}
}
func (gt *Gate) spotWsHandleCancelOrderResp(errS string) {
	if errS != "" {
		ilog.Error(gt.Name() + " spot cancel order fail! " + errS)
	}
}

// priv ws api
func (gt *Gate) SpotWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {
	if gt.SpotWsPrivateIsClosed() {
		return "", errors.New(gt.Name() + " spot priv ws closed")
	}
	type Param struct {
		ClientId    string `json:"text,omitempty"`
		Symbol      string `json:"currency_pair"`
		Price       string `json:"price,omitempty"`
		Qty         string `json:"amount"`
		Side        string `json:"side"`
		Type        string `json:"type"`
		TimeInForce string `json:"time_in_force,omitempty"`
	}
	type Payload struct {
		ReqId    string `json:"req_id"`
		ReqParam Param  `json:"req_param"`
	}
	type Req struct {
		Time    int64   `json:"time"`
		Channel string  `json:"channel"`
		Event   string  `json:"event"`
		Payload Payload `json:"payload"`
	}
	symbolS := gt.getSpotSymbol(symbol)
	req := &Req{
		Time:    time.Now().Unix(),
		Channel: "spot.order_place",
		Event:   "api",
		Payload: Payload{
			ReqId: gutils.RandomStr(16),
			ReqParam: Param{
				Symbol: symbolS,
				Qty:    qty.String(),
				Side:   gt.fromStdSide(side),
				Type:   gt.fromStdOrderType(orderType),
			},
		},
	}
	if cltId != "" {
		req.Payload.ReqParam.ClientId = "t-" + cltId
	}
	if orderType == "LIMIT" {
		req.Payload.ReqParam.Price = price.String()
	}
	if timeInForce != "" {
		req.Payload.ReqParam.TimeInForce = gt.fromStdTimeInForce(timeInForce)
	} else if orderType == "MARKET" {
		req.Payload.ReqParam.TimeInForce = "ioc"
	}
	reqJson, _ := json.Marshal(req)

	gt.spotWsPrivateConnMtx.Lock()
	defer gt.spotWsPrivateConnMtx.Unlock()
	if err := gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(gt.Name() + " send fail: " + err.Error())
	}
	return req.Payload.ReqId, nil
}
func (gt *Gate) SpotWsCancelOrder(symbol, orderId, cltId string) (string, error) {
	if gt.SpotWsPrivateIsClosed() {
		return "", errors.New(gt.Name() + " spot priv ws closed")
	}
	if orderId == "" && cltId != "" {
		orderId = "t-" + cltId
	}
	type Param struct {
		OrderId string `json:"order_id,omitempty"`
		Symbol  string `json:"currency_pair"`
	}
	type Payload struct {
		ReqId    string `json:"req_id"`
		ReqParam Param  `json:"req_param"`
	}
	type Req struct {
		Time    int64   `json:"time"`
		Channel string  `json:"channel"`
		Event   string  `json:"event"`
		Payload Payload `json:"payload"`
	}
	symbolS := gt.getSpotSymbol(symbol)
	req := &Req{
		Time:    time.Now().Unix(),
		Channel: "spot.order_cancel",
		Event:   "api",
		Payload: Payload{
			ReqId: gutils.RandomStr(16),
			ReqParam: Param{
				Symbol:  symbolS,
				OrderId: orderId,
			},
		},
	}
	reqJson, _ := json.Marshal(req)

	gt.spotWsPrivateConnMtx.Lock()
	defer gt.spotWsPrivateConnMtx.Unlock()
	if err := gt.spotWsPrivateConn.WriteMessage(websocket.TextMessage, reqJson); err != nil {
		return "", errors.New(gt.Name() + " send fail: " + err.Error())
	}
	return req.Payload.ReqId, nil
}
