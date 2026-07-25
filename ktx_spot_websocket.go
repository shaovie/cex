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
	ktxWsPubMsgPool sync.Pool
)

func init() {
	ktxWsPubMsgPool = sync.Pool{
		New: func() any {
			return &KtxWsPubMsg{}
		},
	}
}

func (ktx *Ktx) SpotWsPublicOpen() error {
	url := "wss://m-stream.ktx.com"
	var err error
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	ktx.spotWsPublicConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(ktx.Name() + " spot.ws.public con failed! " + err.Error())
	}

	ktx.spotWsPublicClosedMtx.Lock()
	ktx.spotWsPublicClosed = false
	ktx.spotWsPublicClosedMtx.Unlock()
	return nil
}
func (ktx *Ktx) SpotWsPublicSubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	streams := make([]string, 0, 4)
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
				if sym := ktx.getSpotSymbol(v); sym != "" {
					streams = append(streams, "spot."+sym+".ticker")
				}
			}
		}
	}
	if len(streams) > 0 {
		jv, _ := json.Marshal(streams)
		req := fmt.Sprintf(`{"method":"SUBSCRIBE","params":%s}`, string(jv))
		ktx.spotWsPublicConnMtx.Lock()
		ktx.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		ktx.spotWsPublicConnMtx.Unlock()
	}
}
func (ktx *Ktx) SpotWsPublicUnsubscribe(channels []string) {
	if len(channels) == 0 {
		return
	}
	streams := make([]string, 0, 4)
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
				if sym := ktx.getSpotSymbol(v); sym != "" {
					streams = append(streams, "spot."+sym+".ticker")
				}
			}
		}
	}
	if len(streams) > 0 {
		jv, _ := json.Marshal(streams)
		req := fmt.Sprintf(`{"method":"UNSUBSCRIBE","params":%s}`, string(jv))
		ktx.spotWsPublicConnMtx.Lock()
		ktx.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(req))
		ktx.spotWsPublicConnMtx.Unlock()
	}
}
func (ktx *Ktx) SpotWsPublicBBOPoolPut(v any) {
	wsPublicBBOPool.Put(v)
}
func (ktx *Ktx) SpotWsPublicLoop(ch chan<- any) {
	defer ktx.SpotWsPublicClose()
	defer close(ch)

	pingInterval := 30 * time.Second
	pongWait := pingInterval + 2*time.Second
	ktx.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
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
				if ktx.SpotWsPublicIsClosed() {
					break
				}
				s := fmt.Sprintf(`{"ping":%d}`, time.Now().UnixMilli())
				ktx.spotWsPublicConnMtx.Lock()
				ktx.spotWsPublicConn.WriteMessage(websocket.TextMessage, []byte(s))
				ktx.spotWsPublicConnMtx.Unlock()
			}
		}
	}(pingExit)

	for {
		_, recv, err := ktx.spotWsPublicConn.ReadMessage()
		if err != nil {
			if !ktx.SpotWsPublicIsClosed() {
				ilog.Warning(ktx.Name() + " spot.ws.public channel read: " + err.Error())
			}
			break
		}
		msg := ktxWsPubMsgPool.Get().(*KtxWsPubMsg)
		msg.reset()
		if err = easyjson.Unmarshal(recv, msg); err != nil {
			ilog.Error(ktx.Name() + " spot.ws.public recv invalid msg:" + string(recv))
			goto END
		}
		if msg.Stream != "" {
			if strings.HasSuffix(msg.Stream, ".ticker") {
				ktx.spotWsHandleBBO(msg.Data, ch)
			}
		} else if msg.Pong > 0 {
			ktx.spotWsPublicConn.SetReadDeadline(time.Now().Add(pongWait))
		} else if msg.Op != "" {
		} else {
			ilog.Error(ktx.Name() + " spot.ws.public recv unknown msg: " + string(recv))
		}
	END:
		ktxWsPubMsgPool.Put(msg)
	}
}
func (ktx *Ktx) SpotWsPublicIsClosed() bool {
	ktx.spotWsPublicClosedMtx.RLock()
	defer ktx.spotWsPublicClosedMtx.RUnlock()
	return ktx.spotWsPublicClosed
}
func (ktx *Ktx) SpotWsPublicClose() {
	ktx.spotWsPublicClosedMtx.Lock()
	defer ktx.spotWsPublicClosedMtx.Unlock()
	if ktx.spotWsPublicClosed {
		return
	}
	ktx.spotWsPublicClosed = true
	ktx.spotWsPublicConn.Close()
}
func (ktx *Ktx) spotWsHandleBBO(data json.RawMessage, ch chan<- any) {
	bbo := KtxTicker{}
	if err := easyjson.Unmarshal(data, &bbo); err == nil {
		obd := wsPublicBBOPool.Get().(*BestBidAsk)
		obd.Symbol = strings.ReplaceAll(bbo.Symbol, "_", "")
		obd.BidPrice = bbo.BidPrice
		obd.BidQty = bbo.BidQty
		obd.AskPrice = bbo.AskPrice
		obd.AskQty = bbo.AskQty
		ch <- obd
	}
}
