package cex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

// = futures priv channel
func (bb *Bybit) FuturesWsPrivateSupported(typ string) bool {
	return true
}
func (bb *Bybit) FuturesWsPrivateOpen(typ string) error {
	url := "wss://stream.bybit.com/v5/private"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	if bb.localIP != "" {
		localAddr := &net.TCPAddr{
			IP:   net.ParseIP(bb.localIP),
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
	bb.futuresWsPrivateConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bb.Name() + " connect failed! " + err.Error())
	}
	bb.futuresWsPrivateClosedMtx.Lock()
	bb.futuresWsPrivateClosed = false
	bb.futuresWsPrivateClosedMtx.Unlock()

	expires := time.Now().UnixMilli() + 3000
	authMessage := map[string]interface{}{
		"op":   "auth",
		"args": []any{bb.apikey, expires, bb.wsSign(expires)},
	}
	req, _ := json.Marshal(authMessage)
	bb.futuresWsPrivateConn.WriteMessage(websocket.TextMessage, req)
	_, msg, err := bb.futuresWsPrivateConn.ReadMessage()
	if err != nil {
		bb.FuturesWsPrivateClose()
		return errors.New(bb.Name() + " futures.ws.priv recv auth resp err:" + err.Error())
	}
	resp := struct {
		Result bool   `json:"success"`
		Err    string `json:"ret_msg"`
		Op     string `json:"op"`
	}{}
	if err = json.Unmarshal(msg, &resp); err != nil {
		bb.FuturesWsPrivateClose()
		return errors.New(bb.Name() + " futures.ws.priv auth resp err:" + err.Error())
	}
	if resp.Op != "auth" || resp.Result != true {
		bb.FuturesWsPrivateClose()
		return errors.New(bb.Name() + " futures.ws.priv auth fail:" + string(msg))
	}
	return nil
}
func (bb *Bybit) FuturesWsPrivateSubscribe(channels []string) {
	arg := BbSubscribeArg{Op: "subscribe"}
	for _, c := range channels {
		if c == "orders" {
			arg.Args = append(arg.Args, "order")
		} else if c == "positions" {
			arg.Args = append(arg.Args, "position")
		} else if c == "balance" {
			arg.Args = append(arg.Args, "wallet")
		}
	}
	if len(arg.Args) > 0 {
		req, _ := json.Marshal(&arg)
		bb.futuresWsPrivateConnMtx.Lock()
		if err := bb.futuresWsPrivateConn.WriteMessage(websocket.TextMessage, req); err != nil {
			ilog.Warning(bb.Name() + " futures.ws.priv subscribe net error! " + err.Error())
		}
		bb.futuresWsPrivateConnMtx.Unlock()
	}
}
func (bb *Bybit) FuturesWsPrivateIsClosed() bool {
	bb.futuresWsPrivateClosedMtx.RLock()
	defer bb.futuresWsPrivateClosedMtx.RUnlock()
	return bb.futuresWsPrivateClosed
}
func (bb *Bybit) FuturesWsPrivateClose() {
	bb.futuresWsPrivateClosedMtx.Lock()
	defer bb.futuresWsPrivateClosedMtx.Unlock()
	if bb.futuresWsPrivateClosed {
		return
	}
	bb.futuresWsPrivateClosed = true
	bb.futuresWsPrivateConn.Close()
}
func (bb *Bybit) FuturesWsPrivateLoop(ch chan<- any) {
	defer bb.FuturesWsPrivateClose()
	defer close(ch)

	pingInterval := 31 * time.Second
	pongWait := pingInterval + 2*time.Second
	bb.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		ping := `{"op":"ping"}`
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if bb.FuturesWsPrivateIsClosed() {
					return
				}
				bb.futuresWsPrivateConnMtx.Lock()
				bb.futuresWsPrivateConn.WriteMessage(websocket.TextMessage, []byte(ping))
				bb.futuresWsPrivateConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := bb.futuresWsPrivateConn.ReadMessage()
		if err != nil {
			if !bb.FuturesWsPrivateIsClosed() {
				ilog.Warning(bb.Name() + " futures.ws.priv channel read: " + err.Error())
			}
			break
		}
		if bb.debug {
			ilog.Rinfo(bb.Name() + " futures priv ws: " + string(recv))
		}
		msg := bbWsPrivMsgPool.Get().(*BybitWsPrivMsg)
		msg.reset()
		if err = json.Unmarshal(recv, msg); err != nil {
			ilog.Error(bb.Name() + " futures.ws.priv recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Op == "ping" {
			bb.futuresWsPrivateConn.SetReadDeadline(time.Now().Add(pongWait))
		} else if msg.Topic == "wallet" {
			bb.futuresWsHandleWallet(msg.Data, ch)
		} else if msg.Topic == "order" {
			bb.futuresWsHandleOrder(msg.Data, ch)
		} else if msg.Topic == "position" {
			bb.futuresWsHandlePosition(msg.Data, ch)
		} else {
			if msg.Op == "subscribe" { // 订阅的响应
				if bytes.Contains(recv, []byte("false")) {
					ilog.Error(bb.Name() + " futures.ws.priv recv subscribe err:" + string(recv))
				}
			}
		}
	END:
		bbWsPrivMsgPool.Put(msg)
	}
}
func (bb *Bybit) futuresWsHandleOrder(data json.RawMessage, ch chan<- any) {
	orders := []struct {
		Symbol       string            `json:"symbol"` // BTCUSDT
		OrderId      string            `json:"orderId"`
		ClientId     string            `json:"orderLinkId"`
		Price        decimal.Decimal   `json:"price"`
		Quantity     decimal.Decimal   `json:"qty"`          // 用户设置的原始订单数量
		Type         string            `json:"orderType"`    // Limit/Market
		Side         string            `json:"side"`         // Buy/Sell
		ExecutedQty  decimal.Decimal   `json:"cumExecQty"`   // 交易的订单数量
		CummQuoteQty decimal.Decimal   `json:"cumExecValue"` // 累计交易的金额
		AvgPrice     decimal.Decimal   `json:"avgPrice"`
		FeeQty       decimal.Decimal   `json:"cumExecFee"`
		Rpnl         decimal.Decimal   `json:"closedPnl"`
		Status       string            `json:"orderStatus"`
		Time         string            `json:"createdTime"` // msec
		UTime        string            `json:"updatedTime"` // msec
		FeeDetail    map[string]string `json:"cumFeeDetail"`
	}{}
	if err := json.Unmarshal(data, &orders); err == nil && len(orders) > 0 {
		for i := range orders {
			fo := &FuturesOrder{
				Symbol:         orders[i].Symbol,
				OrderId:        orders[i].OrderId,
				ClientId:       orders[i].ClientId,
				Price:          orders[i].Price,
				Qty:            orders[i].Quantity,
				FilledQty:      orders[i].ExecutedQty,
				FilledAmt:      orders[i].CummQuoteQty,
				AvgPrice:       orders[i].AvgPrice,
				FeeQty:         orders[i].FeeQty.Neg(), // 换成负数
				RealizedProfit: orders[i].Rpnl,
				Status:         bb.toStdOrderStatus(orders[i].Status),
				Type:           bb.toStdOrderType(orders[i].Type),
				Side:           bb.toStdSide(orders[i].Side),
			}
			fo.CTime, _ = strconv.ParseInt(orders[i].Time, 10, 64)
			fo.UTime, _ = strconv.ParseInt(orders[i].UTime, 10, 64)
			for k, v := range orders[i].FeeDetail {
				fo.FeeAsset = k
				fo.FeeQty, _ = decimal.NewFromString(v)
				fo.FeeQty = fo.FeeQty.Neg() // 换成负数
				break
			}
			ch <- fo
		}
	}
}
func (bb *Bybit) futuresWsHandlePosition(data json.RawMessage, ch chan<- any) {
	posL := []struct {
		Symbol        string          `json:"symbol"` // BTCUSDT
		Side          string          `json:"side"`   // Buy/Sell,空仓时为""
		PositionIdx   int             `json:"positionIdx"`
		PositionQty   decimal.Decimal `json:"size"`
		EntryPrice    decimal.Decimal `json:"entryPrice"`
		LiqPrice      decimal.Decimal `json:"liqPrice"`
		Leverage      decimal.Decimal `json:"leverage"`
		UnrealisedPnl decimal.Decimal `json:"unrealisedPnl"`
		Time          string          `json:"updatedTime"` // msec
	}{}
	if err := json.Unmarshal(data, &posL); err == nil {
		for i := range posL {
			mode, side := 0, bb.toStdSide(posL[i].Side)
			if posL[i].PositionIdx == 1 { // 双向持仓
				mode, side = 1, "BUY"
			} else if posL[i].PositionIdx == 2 {
				mode, side = 1, "SELL"
			}
			utime, _ := strconv.ParseInt(posL[i].Time, 10, 64)
			cp := FuturesPosition{
				Mode:             mode,
				Symbol:           posL[i].Symbol,
				Side:             side,
				PositionQty:      posL[i].PositionQty.Abs(),
				EntryPrice:       posL[i].EntryPrice,
				LiqPrice:         posL[i].LiqPrice,
				Leverage:         posL[i].Leverage,
				UnRealizedProfit: posL[i].UnrealisedPnl,
				UTime:            utime,
			}
			ch <- &cp
		}
	}
}
func (bb *Bybit) futuresWsHandleWallet(data json.RawMessage, ch chan<- any) {
	wallets := []struct {
		Coin []struct {
			Symbol string          `json:"coin"`
			Total  decimal.Decimal `json:"walletBalance"`
		} `json:"coin"`
	}{}
	if err := json.Unmarshal(data, &wallets); err == nil {
		for i := range wallets {
			for j := range wallets[i].Coin {
				fa := FuturesAsset{
					Symbol:            wallets[i].Coin[j].Symbol,
					Total:             wallets[i].Coin[j].Total,
					Avail:             wallets[i].Coin[j].Total,
					MaxWithdrawAmount: decimal.NewFromInt(-999999999),
				}
				ch <- &fa
			}
		}
	}
}
