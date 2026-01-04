package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Bigone struct {
	Unsupported
	name      string
	account   string
	apikey    string
	secretkey string
	debug     bool

	// spot websocket
	spotWsPublicConn               *websocket.Conn
	spotWsPublicConnMtx            sync.Mutex
	spotWsPublicClosed             bool
	spotWsPublicClosedMtx          sync.RWMutex
	spotWsPublicTickerInnerPool    *sync.Pool
	spotWsPublicOrderBookInnerPool *sync.Pool
	spotWsOrderBookBids            map[string]*treemap.Map[decimal.Decimal, string]
	spotWsOrderBookAsks            map[string]*treemap.Map[decimal.Decimal, string]
	spotWsOrderBookSeqId           map[string]string

	spotWsPrivateConn      *websocket.Conn
	spotWsPrivateConnMtx   sync.Mutex
	spotWsPrivateClosed    bool
	spotWsPrivateClosedMtx sync.RWMutex
}

var (
	boSpotSymbolMap    map[string]string
	boSpotSymbolMapMtx sync.Mutex

	boFuturesSymbolMap    map[string]string
	boFuturesSymbolMapMtx sync.Mutex
)

const boSpotEndpoint = "https://big.one/api/v3"
const boApiDeadline = 1500 * time.Millisecond

func init() {
	boSpotSymbolMap = make(map[string]string)
	boFuturesSymbolMap = make(map[string]string)
}
func (bo *Bigone) Name() string {
	return bo.name
}
func (bo *Bigone) Account() string {
	return bo.account
}
func (bo *Bigone) ApiKey() string {
	return bo.apikey
}
func (bo *Bigone) Debug(v bool) {
	bo.debug = v
}
func (bo *Bigone) Init() error {
	bo.spotWsPublicClosed = true
	bo.spotWsPrivateClosed = true
	bo.spotWsOrderBookBids = make(map[string]*treemap.Map[decimal.Decimal, string], 512)
	bo.spotWsOrderBookAsks = make(map[string]*treemap.Map[decimal.Decimal, string], 512)
	bo.spotWsOrderBookSeqId = make(map[string]string, 16)

	bo.spotWsPublicOrderBookInnerPool = &sync.Pool{
		New: func() any {
			return &BigoneSpotOrderBook{
				Depth: BigoneSpotOrderBookDepth{
					Bids: make([]BigoneSpotOrderBookItem, 0, 5),
					Asks: make([]BigoneSpotOrderBookItem, 0, 5),
				},
			}
		},
	}
	return nil
}
func (bo *Bigone) jwt() string {
	header := `{"typ":"JWT","alg":"HS256"}`
	header = base64.RawURLEncoding.EncodeToString([]byte(header))
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	payload := `{"type":"OpenAPIV2","sub":"` + bo.apikey + `","nonce":"` + nonce + `"}`
	payload = base64.RawURLEncoding.EncodeToString([]byte(payload))

	params := header + "." + payload
	h := hmac.New(sha256.New, []byte(bo.secretkey))
	h.Write([]byte(params))
	return params + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (bo *Bigone) getSpotSymbol(symbol string) string {
	boSpotSymbolMapMtx.Lock()
	defer boSpotSymbolMapMtx.Unlock()
	return boSpotSymbolMap[symbol]
}
func (bo *Bigone) getFuturesSymbol(symbol string) string {
	boFuturesSymbolMapMtx.Lock()
	defer boFuturesSymbolMapMtx.Unlock()
	return boFuturesSymbolMap[symbol]
}

func (bo *Bigone) toStdSide(side string) string {
	if side == "BID" {
		return "BUY"
	} else if side == "ASK" {
		return "SELL"
	}
	return ""
}
func (bo *Bigone) fromStdSide(side string) string {
	if side == "BUY" {
		return "BID"
	} else if side == "SELL" {
		return "ASK"
	}
	return ""
}
func (bo *Bigone) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "LIMIT"
	} else if orderType == "MARKET" {
		return "MARKET"
	}
	return ""
}
func (bo *Bigone) toStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "LIMIT"
	} else if orderType == "MARKET" {
		return "MARKET"
	}
	return ""
}
func (bo *Bigone) toStdOrderStatus(status string) string {
	if status == "PENDING" || status == "OPENING" {
		return "NEW"
	} else if status == "PENDING" {
		return "PARTIALLY_FILLED"
	} else if status == "FILLED" {
		return "FILLED"
	} else if status == "CANCELLED" {
		return "CANCELED"
	} else if status == "REJECTED" {
		return "REJECTED"
	}
	return ""
}
func (bo *Bigone) fromStdChainName(c string) string {
	if c == "TRX" {
		return "Tron"
	} else if c == "MOB" {
		return "Mobilecoin"
	} else if c == "BSC" {
		return "BinanceSmartChain"
	}
	return ""
}
func (bo *Bigone) toStdWithdrawStatus(c string) string {
	if c == "COMPLETED" {
		return "DONE"
	} else if c == "FAILED" {
		return "FAILED"
	} else if c == "CANCELLED" {
		return "CANCELLED"
	} else if c == "PENDING" {
		return "PENDING"
	}
	return ""
}
