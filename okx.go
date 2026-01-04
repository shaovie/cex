package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Okx struct {
	Unsupported
	name      string
	account   string
	apikey    string
	passwd    string
	secretkey string
	debug     bool

	// spot websocket
	spotWsPublicConn            *websocket.Conn
	spotWsPublicConnMtx         sync.Mutex
	spotWsPublicClosed          bool
	spotWsPublicClosedMtx       sync.RWMutex
	spotWsPublicTickerInnerPool *sync.Pool

	spotWsPrivateConn      *websocket.Conn
	spotWsPrivateConnMtx   sync.Mutex
	spotWsPrivateClosed    bool
	spotWsPrivateClosedMtx sync.RWMutex

	// contract websocket
	wsContractPubCon              *websocket.Conn
	wsContractPubConPongTime      int64
	wsContractPubConMtx           sync.Mutex
	wsContractPubChannelClosed    bool
	wsContractPubChannelClosedMtx sync.RWMutex
	wsContractPubTickerPool       *sync.Pool
	wsContractPubOrderBookPool    *sync.Pool

	wsContractPrivCon              *websocket.Conn
	wsContractPrivConPongTime      int64
	wsContractPrivConMtx           sync.Mutex
	wsContractPrivChannelClosed    bool
	wsContractPrivChannelClosedMtx sync.RWMutex

	wsUnifiedCon              *websocket.Conn
	wsUnifiedConPongTime      int64
	wsUnifiedConMtx           sync.Mutex
	wsUnifiedChannelClosed    bool
	wsUnifiedChannelClosedMtx sync.RWMutex
}

var (
	okxSpotSymbolMap    map[string]string
	okxSpotSymbolMapMtx sync.Mutex

	okxContractSymbolMap    map[string]string
	okxContractSymbolMapMtx sync.Mutex
)

const okUniEndpoint = "https://www.okx.com"
const okApiDeadline = 1500 * time.Millisecond

func init() {
	okxSpotSymbolMap = make(map[string]string)
	okxContractSymbolMap = make(map[string]string)
}
func (ok *Okx) Name() string {
	return ok.name
}
func (ok *Okx) Account() string {
	return ok.account
}
func (ok *Okx) ApiKey() string {
	return ok.apikey
}
func (ok *Okx) Debug(v bool) {
	ok.debug = v
}
func (ok *Okx) Init() error {
	ok.spotWsPublicClosed = true
	ok.spotWsPrivateClosed = true

	ok.wsContractPubChannelClosed = true
	ok.wsContractPrivChannelClosed = true
	ok.wsUnifiedChannelClosed = true

	ok.spotWsPublicTickerInnerPool = &sync.Pool{
		New: func() any {
			return make([]Okx24hTicker, 0, 2)
		},
	}
	return nil
}
func (ok *Okx) getSpotSymbol(symbol string) string {
	okxSpotSymbolMapMtx.Lock()
	defer okxSpotSymbolMapMtx.Unlock()
	return okxSpotSymbolMap[symbol]
}
func (ok *Okx) getContractSymbol(symbol string) string {
	okxContractSymbolMapMtx.Lock()
	defer okxContractSymbolMapMtx.Unlock()
	return okxContractSymbolMap[symbol]
}
func (ok *Okx) buildHeaders(method, path, body string) map[string]string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.999Z")
	return map[string]string{
		"Content-Type":         "application/json",
		"OK-ACCESS-SIGN":       ok.sign(ts + method + path + body),
		"OK-ACCESS-KEY":        ok.apikey,
		"OK-ACCESS-TIMESTAMP":  ts,
		"OK-ACCESS-PASSPHRASE": ok.passwd,
	}
}
func (ok *Okx) sign(params string) string {
	h := hmac.New(sha256.New, []byte(ok.secretkey))
	h.Write([]byte(params))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
func (ok *Okx) toStdSide(side string) string {
	if side == "buy" {
		return "BUY"
	} else if side == "sell" {
		return "SELL"
	} else if side == "long" {
		return "BUY"
	} else if side == "short" {
		return "SELL"
	}
	return ""
}
func (ok *Okx) fromStdSide(side string) string {
	if side == "BUY" {
		return "buy"
	} else if side == "SELL" {
		return "sell"
	}
	return ""
}
func (ok *Okx) toStdOrderType(orderType string) string {
	if orderType == "limit" {
		return "LIMIT"
	} else if orderType == "market" {
		return "MARKET"
	} else if orderType == "ioc" {
		return "IOC"
	} else if orderType == "fok" {
		return "FOK"
	}
	return ""
}
func (ok *Okx) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "limit"
	} else if orderType == "MARKET" {
		return "market"
	} else if orderType == "FOK" {
		return "fok"
	} else if orderType == "IOC" {
		return "ioc"
	}
	return ""
}
func (ok *Okx) fromStdTriggerBy(triggerBy string) string {
	if triggerBy == "MARKET_PRICE" {
		return "last"
	} else if triggerBy == "MARK_PRICE" {
		return "mark"
	}
	return ""
}
func (ok *Okx) toStdTriggerBy(triggerBy string) string {
	if triggerBy == "last" {
		return "MARKET_PRICE"
	} else if triggerBy == "mark" {
		return "MARK_PRICE"
	}
	return ""
}
func (ok *Okx) toStdTradeMode(v string) int {
	if v == "isolated" {
		return 1
	} else if v == "cross" {
		return 0
	} else if v == "cash" { // 非保证金模式
		return 2
	}
	return -1
}
func (ok *Okx) toStdOrderStatus(status string) string {
	if status == "live" {
		return "NEW"
	} else if status == "partially_filled" {
		return "PARTIALLY_FILLED"
	} else if status == "filled" {
		return "FILLED"
	} else if status == "canceled" {
		return "CANCELED"
	}
	return ""
}
