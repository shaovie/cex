package cex

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Gate struct {
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

	wsContractPrivCon              *websocket.Conn
	wsContractPrivConMtx           sync.Mutex
	wsContractPrivChannelClosed    bool
	wsContractPrivChannelClosedMtx sync.RWMutex

	wsUnifiedCon              *websocket.Conn
	wsUnifiedConMtx           sync.Mutex
	wsUnifiedChannelClosed    bool
	wsUnifiedChannelClosedMtx sync.RWMutex
}

type GatePrivAuth struct {
	Method string `json:"method"`
	Key    string `json:"KEY"`
	Sign   string `json:"SIGN"`
}
type GateSubscribeArg struct {
	Time    int64         `json:"time"`
	Channel string        `json:"channel"`
	Event   string        `json:"event"`
	Payload []string      `json:"payload,omitempty"`
	Auth    *GatePrivAuth `json:"auth,omitempty"`
}

var (
	gtSpotSymbolMap    map[string]string
	gtSpotSymbolMapMtx sync.Mutex

	gtContractSymbolMap    map[string]string
	gtContractSymbolMapMtx sync.Mutex
)

const gtUniEndpoint = "https://api.gateio.ws"
const gtApiDeadline = 1500 * time.Millisecond

func init() {
	gtSpotSymbolMap = make(map[string]string)
	gtContractSymbolMap = make(map[string]string)
}
func (gt *Gate) Name() string {
	return gt.name
}
func (gt *Gate) Account() string {
	return gt.account
}
func (gt *Gate) ApiKey() string {
	return gt.apikey
}
func (gt *Gate) Debug(v bool) {
	gt.debug = v
}
func (gt *Gate) Init() error {
	gt.spotWsPublicClosed = true
	gt.spotWsPrivateClosed = true

	gt.wsContractPubChannelClosed = true
	gt.wsContractPrivChannelClosed = true
	gt.wsUnifiedChannelClosed = true

	gt.spotWsPublicTickerInnerPool = &sync.Pool{
		New: func() any {
			return &GateSpot24hTicker{}
		},
	}
	gt.spotWsPublicOrderBookInnerPool = &sync.Pool{
		New: func() any {
			return &GateSpotOrderBook{
				Bids: make([][2]decimal.Decimal, 0, 5),
				Asks: make([][2]decimal.Decimal, 0, 5),
			}
		},
	}
	return nil
}
func (gt *Gate) buildHeaders(method, path, params, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	headers := map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"SIGN":         gt.sign(method, path, params, body, ts),
		"KEY":          gt.apikey,
		"Timestamp":    ts,
	}
	return headers
}
func (gt *Gate) getSpotSymbol(symbol string) string {
	gtSpotSymbolMapMtx.Lock()
	defer gtSpotSymbolMapMtx.Unlock()
	return gtSpotSymbolMap[symbol]
}
func (gt *Gate) getContractSymbol(symbol string) string {
	gtContractSymbolMapMtx.Lock()
	defer gtContractSymbolMapMtx.Unlock()
	return gtContractSymbolMap[symbol]
}
func (gt *Gate) handleExceptionResp(api string, resp []byte) error {
	ret := struct {
		Label string `json:"label,omitempty"`
		Msg   string `json:"message,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(gt.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(ret.Msg)
}
func (gt *Gate) sign(method, path, params, body, ts string) string {
	h := sha512.New()
	if len(body) > 0 {
		h.Write([]byte(body))
	}
	hashedPayload := hex.EncodeToString(h.Sum(nil))

	msg := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", method, path, params, hashedPayload, ts)
	hm := hmac.New(sha512.New, []byte(gt.secretkey))
	hm.Write([]byte(msg))
	return hex.EncodeToString(hm.Sum(nil))
}
func (gt *Gate) wsSign(channel, event string, ts int64) string {
	msg := fmt.Sprintf("channel=%s&event=%s&time=%d", channel, event, ts)
	hm := hmac.New(sha512.New, []byte(gt.secretkey))
	hm.Write([]byte(msg))
	return hex.EncodeToString(hm.Sum(nil))
}
func (gt *Gate) wsLoginSign(channel, event string, ts int64) string {
	msg := fmt.Sprintf("%s\n%s\n%s\n%d", event, channel, "", ts)
	hm := hmac.New(sha512.New, []byte(gt.secretkey))
	hm.Write([]byte(msg))
	return hex.EncodeToString(hm.Sum(nil))
}
func (gt *Gate) toStdSide(side string) string {
	if side == "buy" {
		return "BUY"
	} else if side == "sell" {
		return "SELL"
	}
	return ""
}
func (gt *Gate) fromStdSide(side string) string {
	if side == "BUY" {
		return "buy"
	} else if side == "SELL" {
		return "sell"
	}
	return ""
}
func (gt *Gate) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "limit"
	} else if orderType == "MARKET" {
		return "market"
	}
	return ""
}
func (gt *Gate) toStdOrderType(orderType string) string {
	if orderType == "limit" {
		return "LIMIT"
	} else if orderType == "market" {
		return "MARKET"
	}
	return ""
}
func (gt *Gate) toStdTimeInForce(timeInForce string) string {
	if timeInForce == "gtc" {
		return "GTC"
	} else if timeInForce == "ioc" {
		return "IOC"
	} else if timeInForce == "fok" {
		return "FOK"
	}
	return ""
}
func (gt *Gate) fromStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" {
		return "gtc"
	} else if timeInForce == "IOC" {
		return "ioc"
	} else if timeInForce == "FOK" {
		return "fok"
	}
	return ""
}
func (gt *Gate) toStdOrderStatus(status string) string {
	if status == "open" {
		return "NEW"
	} else if status == "closed" || status == "filled" {
		return "FILLED"
	} else if status == "cancelled" {
		return "CANCELED"
	} else if status == "partially_filled" {
		return "PARTIALLY_FILLED"
	}
	return ""
}
