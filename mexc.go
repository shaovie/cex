package cex

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Mexc struct {
	Unsupported
	name      string
	account   string
	apikey    string
	secretkey string

	// spot websocket
	spotWsPublicConn               *websocket.Conn
	spotWsPublicConnMtx            sync.Mutex
	spotWsPublicClosed             bool
	spotWsPublicClosedMtx          sync.RWMutex
	spotWsPublicTickerInnerPool    *sync.Pool
	spotWsPublicOrderBookInnerPool *sync.Pool
}
type McSubscribeArg struct {
	Id     string   `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}
type McWsApiArg struct {
	Id     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

const mcUniEndpoint = "https://api.mexc.com"
const mcApiDeadline = 1500 * time.Millisecond

func (mc *Mexc) Name() string {
	return mc.name
}
func (mc *Mexc) Account() string {
	return mc.account
}
func (mc *Mexc) ApiKey() string {
	return mc.apikey
}
func (mc *Mexc) Init() error {
	mc.spotWsPublicClosed = true
	mc.spotWsPublicTickerInnerPool = &sync.Pool{
		New: func() any {
			return &MexcSpot24hTicker{}
		},
	}
	mc.spotWsPublicOrderBookInnerPool = &sync.Pool{
		New: func() any {
			return &MexcSpotOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
	return nil
}
func (mc *Mexc) handleExceptionResp(api string, resp []byte) error {
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(mc.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(ret.Msg)
}
