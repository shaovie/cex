package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Binance struct {
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

	spotWsPrivateConn      *websocket.Conn
	spotWsPrivateConnMtx   sync.Mutex
	spotWsPrivateClosed    bool
	spotWsPrivateClosedMtx sync.RWMutex

	// contract websocket
	wsContractPubCon              *websocket.Conn
	wsContractPubConMtx           sync.Mutex
	wsContractPubChannelClosed    bool
	wsContractPubChannelClosedMtx sync.RWMutex
	wsContractPubTickerPool       *sync.Pool
	wsContractPubOrderBookPool    *sync.Pool

	wsContractPrivCon              *websocket.Conn // for user data stream
	wsContractPrivConMtx           sync.Mutex
	wsContractPrivChannelClosed    bool
	wsContractPrivChannelClosedMtx sync.RWMutex
	wsContractPrivChannelListenKey string

	wsContractPrivApiCon              *websocket.Conn // for api
	wsContractPrivApiConMtx           sync.Mutex
	wsContractPrivApiChannelClosed    bool
	wsContractPrivApiChannelLogout    bool
	wsContractPrivApiChannelClosedMtx sync.RWMutex

	wsUnifiedContractCon          *websocket.Conn
	wsUnifiedContractListenKey    string
	wsUnifiedContractConMtx       sync.Mutex
	wsUnifiedContractConClosed    bool
	wsUnifiedContractConClosedMtx sync.RWMutex
}

type BnSubscribeArg struct {
	Id     string   `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}
type BnWsApiArg struct {
	Id     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

const bnSpotEndpoint = "https://api2.binance.com"
const bnUMFuturesEndpoint = "https://fapi.binance.com"
const bnCMFuturesEndpoint = "https://dapi.binance.com"
const bnApiDeadline = 1200 * time.Millisecond

func (bn *Binance) Name() string {
	return bn.name
}
func (bn *Binance) Account() string {
	return bn.account
}
func (bn *Binance) ApiKey() string {
	return bn.apikey
}
func (bn *Binance) Init() error {
	bn.spotWsPublicClosed = true
	bn.spotWsPrivateClosed = true

	bn.wsContractPubChannelClosed = true
	bn.wsContractPrivChannelClosed = true
	bn.wsContractPrivApiChannelLogout = true
	bn.wsContractPrivApiChannelClosed = true

	bn.wsUnifiedContractConClosed = true

	bn.spotWsPublicTickerInnerPool = &sync.Pool{
		New: func() any {
			return &BinanceSpot24hTicker{}
		},
	}
	bn.spotWsPublicOrderBookInnerPool = &sync.Pool{
		New: func() any {
			return &BinanceSpotOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
	return nil
}
func (bn *Binance) httpQuerySign(query string) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	params := "recvWindow=3000&timestamp=" + ts + query
	return params + "&signature=" + bn.sign(params)
}
func (bn *Binance) sign(params string) string {
	h := hmac.New(sha256.New, []byte(bn.secretkey))
	h.Write([]byte(params))
	return hex.EncodeToString(h.Sum(nil))
}
func (bn *Binance) handleExceptionResp(api string, resp []byte) error {
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bn.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(ret.Msg)
}
func (bn *Binance) toStdSide(side string) string {
	if side == "Buy" || side == "BUY" {
		return "BUY"
	} else if side == "Sell" || side == "SELL" {
		return "SELL"
	} else if side == "LONG" {
		return "BUY"
	} else if side == "SHORT" {
		return "SELL"
	}
	return ""
}
func (bn *Binance) toStdOrderType(orderType string) string {
	if orderType == "Limit" {
		return "LIMIT"
	} else if orderType == "Market" {
		return "MARKET"
	}
	return ""
}
func (bn *Binance) toStdTradeMode(v string) int {
	if v == "isolated" || v == "ISOLATED" {
		return 1
	} else if v == "crossed" || v == "CROSSED" || v == "cross" {
		return 0
	}
	return -1
}
