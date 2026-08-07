package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mailru/easyjson"
	"github.com/shaovie/gutils/ilog"
)

var (
	kcWsPubMsgPool sync.Pool
)

func init() {
	kcWsPubMsgPool = sync.Pool{
		New: func() any {
			return &KucoinWsPubMsg{}
		},
	}
}

func (kc *Kucoin) SpotWsPublicOpen() error {
	url := "wss://x-push-spot.kucoin.com"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	kc.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(kc.Name() + " spot.ws.public con failed! " + err.Error())
	}

	kc.spotWsPublicClosedMtx.Lock()
	kc.spotWsPublicClosed = false
	kc.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (kc *Kucoin) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" { // 用ticker实现
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kc.getSpotSymbol(v); sym != "" {
					req := fmt.Sprintf(`{"action":"subscribe","channel":"obu",`+
						`"tradeType":"SPOT","depth":"1","symbol":"%s"}`, sym)
					kc.spotWsPublicConnMtx.Lock()
					kc.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
					kc.spotWsPublicConnMtx.Unlock()
				}
			}
		}
	}
}
func (kc *Kucoin) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	for _, c := range channels {
		arr := strings.Split(c, "@")
		if arr[0] == "bbo" { // 用ticker实现
			var symbolArr []string
			if len(arr) > 1 && len(arr[1]) > 0 {
				symbolArr = strings.Split(arr[1], ",")
			} else {
				continue
			}
			for _, v := range symbolArr {
				if sym := kc.getSpotSymbol(v); sym != "" {
					req := fmt.Sprintf(`{"action":"unsubscribe","channel":"obu",`+
						`"tradeType":"SPOT","depth":"1","symbol":"%s"}`, sym)
					kc.spotWsPublicConnMtx.Lock()
					kc.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
					kc.spotWsPublicConnMtx.Unlock()
				}
			}
		}
	}
}
func (kc *Kucoin) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (kc *Kucoin) SpotWsPublicLoop(ch chan<- any) {
	defer kc.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 2*time.Second
	kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
	pingExit := make(chan struct{})
	defer close(pingExit)
	go func(exitChan <-chan struct{}) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		var pingMsg = []byte(`{"type":"ping"}`)
		for {
			select {
			case <-exitChan:
				return
			case <-ticker.C:
				if kc.SpotWsPublicIsClosed() {
					break
				}
				kc.spotWsPublicConnMtx.Lock()
				kc.spotWsPublicConn.WriteMessage(websocket.TextMessage, pingMsg)
				kc.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := kc.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !kc.SpotWsPublicIsClosed() {
				ilog.Warning(kc.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := kcWsPubMsgPool.Get().(*KucoinWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(kc.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Channel == "obu.SPOT" {
			if msg.T == "snapshot" {
				kc.spotWsHandleBBO(msg.Data, ch)
			}
		} else if msg.Type == "pong" {
			kc.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		} else {
			//ilog.Error(kc.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		kcWsPubMsgPool.Put(msg)
	}
}
func (kc *Kucoin) SpotWsPublicIsClosed() bool {
	kc.spotWsPublicClosedMtx.RLock()
	defer kc.spotWsPublicClosedMtx.RUnlock()
	return kc.spotWsPublicClosed
}
func (kc *Kucoin) SpotWsPublicClose() {
	kc.spotWsPublicClosedMtx.Lock()
	defer kc.spotWsPublicClosedMtx.Unlock()
	if kc.spotWsPublicClosed {
		return
	}
	kc.spotWsPublicClosed = true
	kc.spotWsPublicConn.Close()
}
func (kc *Kucoin) spotWsHandleBBO(data json.RawMessage, ch chan<- any) {
	bbo := KucoinTicker{}
	if err := easyjson.Unmarshal(data, &bbo); err == nil && len(bbo.Bids) > 0 && len(bbo.Asks) > 0 {
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		obd.Symbol = strings.ReplaceAll(bbo.Symbol, "-", "")
		obd.BidPrice = bbo.Bids[0][0]
		obd.BidQty = bbo.Bids[0][1]
		obd.AskPrice = bbo.Asks[0][0]
		obd.AskQty = bbo.Asks[0][1]
		ch <- obd
	}
}
