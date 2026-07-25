package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Kraken struct {
	Unsupported
	name               string
	account            string
	apikey             string
	secretkey          string
	debug              bool
	secretkeyHadDecode bool

	// spot websocket
	spotWsPublicConn             *websocket.Conn
	spotWsPublicConnMtx          sync.Mutex
	spotWsPublicClosed           bool
	spotWsPublicClosedMtx        sync.RWMutex
	spotWsOrderBookBids          map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
	spotWsOrderBookAsks          map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
	spotWsOrderBookSeqId         map[string]int64
	spotWsOrderBookBid1Ask1Cache BestBidAsk
	spotWsOrderCachedInfo        map[string]*KrakenCachedOrder

	spotWsPrivateConn             *websocket.Conn
	spotWsPrivateConnMtx          sync.Mutex
	spotWsPrivateClosed           bool
	spotWsPrivateClosedMtx        sync.RWMutex
	spotWsPrivateToken            string
	spotWsPrivatePongTime         int64
	spotWsPrivatePingInterval     int64
	spotWsPrivateExpectedPongTime int64
}

var (
	kkSpotSymbolMap    map[string]string
	kkSpotSymbolMapMtx sync.RWMutex

	kkSpotWssSymbolMap    map[string]string
	kkSpotWssSymbolMapMtx sync.RWMutex

	kkXStocksSymbolMap    map[string]string
	kkXStocksSymbolMapMtx sync.RWMutex

	kkNonceSeq int64 // 单个apikey下的nonce必须是自增的
)

const kkSpotEndpoint = "https://api.kraken.com"
const kkApiDeadline = 1500 * time.Millisecond

func init() {
	kkSpotSymbolMap = make(map[string]string)
	kkSpotWssSymbolMap = make(map[string]string)
	kkXStocksSymbolMap = make(map[string]string)
}
func NewKraken(account, apikey, secretkey string) *Kraken {
	cexObj := &Kraken{
		name:      "kraken",
		account:   account,
		apikey:    apikey,
		secretkey: secretkey,
	}
	return cexObj
}
func (kk *Kraken) Name() string {
	return kk.name
}
func (kk *Kraken) Account() string {
	return kk.account
}
func (kk *Kraken) ApiKey() string {
	return kk.apikey
}
func (kk *Kraken) Debug(v bool) {
	kk.debug = v
}
func (kk *Kraken) IsXStock(v string) bool {
	return kk.isXStocksSymbol(v)
}
func (kk *Kraken) Init() error {
	kk.spotWsPublicClosed = true
	kk.spotWsPrivateClosed = true
	kk.spotWsPrivateToken = ""
	kk.spotWsPrivatePongTime = 0
	kk.spotWsPrivateExpectedPongTime = 0
	kk.spotWsOrderBookBids = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 512)
	kk.spotWsOrderBookAsks = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 512)
	kk.spotWsOrderBookSeqId = make(map[string]int64, 16)
	kk.spotWsOrderCachedInfo = make(map[string]*KrakenCachedOrder, 16)

	if kk.secretkeyHadDecode == false {
		sk, _ := base64.StdEncoding.DecodeString(kk.secretkey)
		kk.secretkey = string(sk)
		kk.secretkeyHadDecode = true
	}
	return nil
}
func (kk *Kraken) buildHeaders(path string, values url.Values) (map[string]string, string) {
	seq := atomic.AddInt64(&kkNonceSeq, 1)
	ts := strconv.FormatInt(time.Now().UnixNano()+seq, 10)
	values.Set("nonce", ts)
	params := values.Encode()
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
		"API-Sign":     kk.sign(ts, path, params),
		"API-Key":      kk.apikey,
	}
	return headers, params
}
func (kk *Kraken) sign(nonce, path, params string) string {
	sha := sha256.New()
	sha.Write([]byte(nonce + params))
	shaSum := sha.Sum(nil)

	h := hmac.New(sha512.New, []byte(kk.secretkey))
	h.Write(append([]byte(path), shaSum...))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
func (kk *Kraken) getSpotSymbol(symbol string) string {
	kkSpotSymbolMapMtx.RLock()
	defer kkSpotSymbolMapMtx.RUnlock()
	return kkSpotSymbolMap[symbol]
}
func (kk *Kraken) getSpotWssSymbol(symbol string) string {
	kkSpotWssSymbolMapMtx.RLock()
	defer kkSpotWssSymbolMapMtx.RUnlock()
	return kkSpotWssSymbolMap[symbol]
}
func (kk *Kraken) isXStocksSymbol(symbol string) bool {
	kkXStocksSymbolMapMtx.RLock()
	defer kkXStocksSymbolMapMtx.RUnlock()
	_, ok := kkXStocksSymbolMap[symbol]
	return ok
}
func (kk *Kraken) toStdSide(side string) string {
	if side == "buy" {
		return "BUY"
	} else if side == "sell" {
		return "SELL"
	}
	return ""
}
func (kk *Kraken) fromStdSide(side string) string {
	if side == "BUY" {
		return "buy"
	} else if side == "SELL" {
		return "sell"
	}
	return ""
}
func (kk *Kraken) toStdOrderType(orderType string) string {
	if orderType == "limit" {
		return "LIMIT"
	} else if orderType == "market" {
		return "MARKET"
	}
	return ""
}
func (kk *Kraken) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "limit"
	} else if orderType == "MARKET" {
		return "market"
	}
	return ""
}
func (kk *Kraken) toStdOrderStatus(status string) string {
	if status == "new" || status == "pending_new" || status == "pending" || status == "open" {
		return "NEW"
	} else if status == "filled" || status == "closed" {
		return "FILLED"
	} else if status == "expired" {
		return "EXPIRED"
	} else if status == "partially_filled" {
		return "PARTIALLY_FILLED"
	} else if status == "canceled" {
		return "CANCELED"
	}
	return ""
}
func (kk *Kraken) toStdSymbol(s string) string {
	if s == "XBT" || s == "XXBT" {
		return "BTC"
	}
	if s == "XETH" {
		return "ETH"
	}
	if s == "XLTC" {
		return "LTC"
	}
	if s == "XETC" {
		return "ETC"
	}
	if s == "XXDG" {
		return "XDG"
	}
	if s == "XXLM" {
		return "XLM"
	}
	if s == "XXMR" {
		return "XMR"
	}
	if s == "ZUSD" {
		return "USD"
	}
	return s
}
func (kk *Kraken) toStdWithdrawStatus(c string) string {
	if c == "Success" {
		return "COMPLETED"
	} else if c == "Failure" {
		return "FAILED"
	} else if c == "Initial" || c == "Pending" || c == "Settled" {
		return "PENDING"
	}
	return ""
}
