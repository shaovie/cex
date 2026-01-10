package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Bybit struct {
	Unsupported
	name      string
	account   string
	apikey    string
	secretkey string
	debug     bool

	// spot websocket
	spotWsPublicConn      *websocket.Conn
	spotWsPublicConnMtx   sync.Mutex
	spotWsPublicClosed    bool
	spotWsPublicClosedMtx sync.RWMutex

	spotWsPrivateConn      *websocket.Conn
	spotWsPrivateConnMtx   sync.Mutex
	spotWsPrivateClosed    bool
	spotWsPrivateClosedMtx sync.RWMutex
}

const bbUniEndpoint = "https://api.bybit.com"
const bbApiDeadline = 1200 * time.Millisecond

func (bb *Bybit) Name() string {
	return bb.name
}
func (bb *Bybit) Account() string {
	return bb.account
}
func (bb *Bybit) ApiKey() string {
	return bb.apikey
}
func (bb *Bybit) Debug(v bool) {
	bb.debug = v
}
func (bb *Bybit) Init() error {
	bb.spotWsPublicClosed = true
	bb.spotWsPrivateClosed = true
	return nil
}
func (bb *Bybit) buildHeaders(query, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	params := ts + bb.apikey + "3000" + query + body
	return map[string]string{
		"X-BAPI-SIGN":        bb.sign(params),
		"X-BAPI-API-KEY":     bb.apikey,
		"X-BAPI-TIMESTAMP":   ts,
		"X-BAPI-RECV-WINDOW": "3000",
	}
}
func (bb *Bybit) sign(params string) string {
	h := hmac.New(sha256.New, []byte(bb.secretkey))
	h.Write([]byte(params))
	return hex.EncodeToString(h.Sum(nil))
}
func (bb *Bybit) toStdSide(side string) string {
	if side == "Buy" {
		return "BUY"
	} else if side == "Sell" {
		return "SELL"
	}
	return ""
}
func (bb *Bybit) fromStdSide(side string) string {
	if side == "BUY" {
		return "Buy"
	} else if side == "SELL" {
		return "Sell"
	}
	return ""
}
func (bb *Bybit) toStdOrderType(orderType string) string {
	if orderType == "Limit" {
		return "LIMIT"
	} else if orderType == "Market" {
		return "MARKET"
	}
	return ""
}
func (bb *Bybit) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "Limit"
	} else if orderType == "MARKET" {
		return "Market"
	}
	return ""
}
func (bb *Bybit) toStdOrderStatus(status string) string {
	if status == "New" {
		return "NEW"
	} else if status == "Rejected" {
		return "REJECTED"
	} else if status == "PartiallyFilled" {
		return "PARTIALLY_FILLED"
	} else if status == "Filled" {
		return "FILLED"
	} else if status == "Cancelled" {
		return "CANCELED"
	}
	return ""
}
func (bb *Bybit) fromStdCategory(v string) string {
	if v == "CM" {
		return "inverse"
	}
	return "linear"
}
