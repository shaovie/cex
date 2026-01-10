package cex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shaovie/gutils/ihttp"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func (bn *Binance) UnifiedGetAssets() (map[string]*UnifiedAsset, error) {
	url := bnUnifiedEndpoint + "/papi/v1/balance?" + bn.httpQuerySign("")
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("UnifiedGetAssets", resp)
	}
	ret := []struct {
		Symbol string          `json:"asset"`
		Total  decimal.Decimal `json:"totalWalletBalance"`
		Avail  decimal.Decimal `json:"crossMarginFree"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}

	uniAssets := make(map[string]*UnifiedAsset)
	for _, r := range ret {
		if r.Total.IsZero() {
			continue
		}
		as := &UnifiedAsset{
			Symbol: r.Symbol,
		}
		as.Avail = r.Avail
		as.Total = r.Total
		as.Locked = as.Total.Sub(as.Avail)
		uniAssets[as.Symbol] = as
	}

	return uniAssets, nil
}
func (bn *Binance) UnifiedWsSupported() bool {
	return true
}
func (bn *Binance) UnifiedWsOpen() error {
	listenKey, err := bn.getListenKey("UNIFIED")
	if err != nil {
		return errors.New(bn.Name() + " get listenkey fail! " + err.Error())
	}
	url := "wss://fstream.binance.com/pm/ws/" + listenKey
	dialer := websocket.Dialer{
		EnableCompression: true, // 启用压缩扩展
		HandshakeTimeout:  2 * time.Second,
	}
	bn.unifiedWsConn, _, err = dialer.Dial(url, nil)
	if err != nil {
		return errors.New(bn.Name() + " unified.ws connect failed! " + err.Error())
	}
	bn.unifiedWsConnClosedMtx.Lock()
	bn.unifiedWsConnClosed = false
	bn.unifiedWsConnClosedMtx.Unlock()
	return nil
}
func (bn *Binance) UnifiedWsSubscribe(channels []string) {
}
func (bn *Binance) UnifiedWsIsClosed() bool {
	bn.unifiedWsConnClosedMtx.RLock()
	defer bn.unifiedWsConnClosedMtx.RUnlock()
	return bn.unifiedWsConnClosed
}
func (bn *Binance) UnifiedWsClose() {
	bn.unifiedWsConnMtx.Lock()
	defer bn.unifiedWsConnMtx.Unlock()
	if bn.unifiedWsConnClosed {
		return
	}
	bn.unifiedWsConnClosed = true
	bn.unifiedWsConn.Close()
}
func (bn *Binance) UnifiedWsLoop(ch chan<- any) {
	defer bn.UnifiedWsClose()
	defer close(ch)

	pingInterval := 28 * time.Second
	pongWait := pingInterval + 2*time.Second
	bn.unifiedWsConn.SetReadDeadline(time.Now().Add(pongWait))
	bn.unifiedWsConn.SetPongHandler(func(string) error {
		bn.unifiedWsConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if bn.UnifiedWsIsClosed() {
				break
			}
			bn.unifiedWsConnMtx.Lock()
			bn.unifiedWsConn.WriteMessage(websocket.PingMessage, nil)
			bn.unifiedWsConnMtx.Unlock()
		}
	}()

	go func() {
		ticker := time.NewTicker((3600 - 110) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if bn.UnifiedWsIsClosed() {
				break
			}
			bn.getListenKey("UNIFIED")
		}
	}()

	type Msg struct {
		Event  string          `json:"e,omitempty"`
		Time   int64           `json:"E,omitempty"` // msec
		Result json.RawMessage `json:"B,omitempty"`
	}
	for {
		_, recv, err := bn.unifiedWsConn.ReadMessage()
		if err != nil {
			if !bn.UnifiedWsIsClosed() {
				ilog.Warning(bn.Name() + " unified.ws channel read: " + err.Error())
			}
			break
		}
		ilog.Rinfo("bn unified :" + string(recv))
		msg := Msg{}
		if err = json.Unmarshal(recv, &msg); err != nil {
			ilog.Error(bn.Name() + " unified.ws recv invalid msg:" + string(recv))
			continue
		}
		if msg.Event == "outboundAccountPosition" { // outboundAccountPosition
			bn.unifiedWsHandleBalance(msg.Result, ch)
		}
	}
}
func (bn *Binance) unifiedWsHandleBalance(data json.RawMessage, ch chan<- any) {
	bl := []struct {
		Symbol string          `json:"a,omitempty"`
		Avail  decimal.Decimal `json:"f,omitempty"`
		Locked decimal.Decimal `json:"l,omitempty"`
	}{}
	if err := json.Unmarshal(data, &bl); err == nil && len(bl) > 0 {
		for _, as := range bl {
			ch <- &UnifiedAsset{
				Symbol: as.Symbol,
				Total:  as.Avail.Add(as.Locked),
				Avail:  as.Avail,
				Locked: as.Locked,
			}
		}
	} else {
		ilog.Error(bn.Name() + " unified.ws account channle unmarshal: " +
			err.Error() + " " + string(data))
	}
}
