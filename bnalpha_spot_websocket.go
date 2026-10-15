package cex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"

	"github.com/shaovie/gutils/ilog"
)

var (
	baWsPubMsgPool             sync.Pool
	baSpotWsPublicBBOInnerPool sync.Pool
)

func init() {
	baWsPubMsgPool = sync.Pool{
		New: func() any {
			return &BnAlphaWsPubMsg{}
		},
	}
	baSpotWsPublicBBOInnerPool = sync.Pool{
		New: func() any {
			return &BnAlphaBookTicker{}
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

// spotWsStreams 转换频道参数: bbo@ZKJUSDT -> alpha_173usdt@bookTicker
func (ba *BnAlpha) spotWsStreams(channels []string) []string {
	params := make([]string, 0, 4)
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
			if symbolS := ba.getSpotSymbol(sym); len(symbolS) > 0 { // 标准symbol转交易所symbol
				sym = symbolS
			}
			params = append(params, strings.ToLower(sym)+"@bookTicker")
		}
	}
	return params
}
func (ba *BnAlpha) spotWsPublicEvent(method string, channels []string) {
	params := ba.spotWsStreams(channels)
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
		if len(msg.Stream) > 0 { // 订阅/退订响应没有stream
			ba.spotWsHandlePublic(msg.Stream, msg.Data, ch)
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
func (ba *BnAlpha) spotWsHandlePublic(stream string, data []byte, ch chan<- any) {
	l := len(stream)
	if l > 11 && stream[l-11:l] == "@bookTicker" {
		ba.spotWsHandleBBO(data, ch)
	} else {
		ilog.Error(ba.Name() + " spot.ws.public recv unknown stream: " + stream)
	}
}
func (ba *BnAlpha) spotWsHandleBBO(data []byte, ch chan<- any) {
	bboRecv := baSpotWsPublicBBOInnerPool.Get().(*BnAlphaBookTicker)
	defer baSpotWsPublicBBOInnerPool.Put(bboRecv)
	if err := easyjson.Unmarshal(data, bboRecv); err != nil || len(bboRecv.Symbol) == 0 {
		return
	}
	bbo := wsPublicBBOPool.Get().(*BestBidAsk)
	bbo.Symbol = ba.getStdSymbol(bboRecv.Symbol) // 交易所symbol转标准symbol
	bbo.Time = bboRecv.Time
	bbo.BidPrice = bboRecv.BidPrice
	bbo.BidQty = bboRecv.BidQty
	bbo.AskPrice = bboRecv.AskPrice
	bbo.AskQty = bboRecv.AskQty
	ch <- bbo
}
