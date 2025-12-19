package cex

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shaovie/gutils/ilog"
)

type Binance struct {
	Unsupported
	name      string
	account   string
	apikey    string
	secretkey string

	// spot websocket
	spotWsPublicConn      *websocket.Conn
	spotWsPublicConnMtx   sync.Mutex
	spotWsPublicClosed    bool
	spotWsPublicClosedMtx sync.RWMutex

	spotWsPrivateConn      *websocket.Conn
	spotWsPrivateConnMtx   sync.Mutex
	spotWsPrivateClosed    bool
	spotWsPrivateClosedMtx sync.RWMutex

	// contract websocket
	futuresWsPublicTyp       string
	futuresWsPublicConn      *websocket.Conn
	futuresWsPublicConnMtx   sync.Mutex
	futuresWsPublicClosed    bool
	futuresWsPublicClosedMtx sync.RWMutex

	futuresWsPrivateTyp       string
	futuresWsPrivateConn      *websocket.Conn // for user data stream
	futuresWsPrivateConnMtx   sync.Mutex
	futuresWsPrivateClosed    bool
	futuresWsPrivateClosedMtx sync.RWMutex

	futuresWsPrivateApiConn      *websocket.Conn // for api
	futuresWsPrivateApiConnMtx   sync.Mutex
	futuresWsPrivateApiClosed    bool
	futuresWsPrivateApiClosedMtx sync.RWMutex

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

var (
	bnWsPubMsgPool sync.Pool
)

const bnSpotEndpoint = "https://api2.binance.com"
const bnUMFuturesEndpoint = "https://fapi.binance.com"
const bnCMFuturesEndpoint = "https://dapi.binance.com"
const bnWalletEndpoint = "https://api2.binance.com"
const bnApiDeadline = 1200 * time.Millisecond

func init() {
	bnWsPubMsgPool = sync.Pool{
		New: func() any {
			return &BinanceWsPubMsg{}
		},
	}
}
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

	bn.futuresWsPublicTyp = ""
	bn.futuresWsPublicClosed = true
	bn.futuresWsPrivateTyp = ""
	bn.futuresWsPrivateClosed = true
	bn.futuresWsPrivateApiClosed = true

	bn.wsUnifiedContractConClosed = true

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
func (bn *Binance) wsSign(kv map[string]any) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for i, key := range keys {
		val := kv[key]
		valStr := ""
		switch v := val.(type) {
		case bool:
			valStr = strconv.FormatBool(v)
		case string:
			valStr = v
		case int:
			valStr = strconv.FormatInt(int64(v), 10)
		case int64:
			valStr = strconv.FormatInt(v, 10)
		default:
			ilog.Error(bn.Name() + " wsSign unsupported type")
		}
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(key)
		buf.WriteByte('=')
		buf.WriteString(valStr)
	}
	return bn.sign(buf.String())
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
