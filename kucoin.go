package cex

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Kucoin struct {
	Unsupported
	CexOptions
	Http
	name       string
	account    string
	apikey     string
	secretkey  string
	passphrase string
	localIP    string
	debug      bool

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

var (
	kcSpotSymbolMap    map[string]string
	kcSpotSymbolMapMtx sync.RWMutex
)

const kcSpotEndpoint = "https://api.kucoin.com"
const kcApiDeadline = 1500 * time.Millisecond

func init() {
	kcSpotSymbolMap = make(map[string]string)
}

func NewKucoin(account, apikey, secretkey, passphrase, localIP string) (*Kucoin, error) {
	client, err := NewClient("kucoin", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Kucoin{
		Http: Http{
			client: client,
		},
		name:       "kucoin",
		account:    account,
		apikey:     apikey,
		secretkey:  secretkey,
		passphrase: passphrase,
		localIP:    localIP,
	}
	return cexObj, nil
}
func (kc *Kucoin) Name() string {
	return kc.name
}
func (kc *Kucoin) Account() string {
	return kc.account
}
func (kc *Kucoin) ApiKey() string {
	return kc.apikey
}
func (kc *Kucoin) Debug(v bool) {
	kc.debug = v
}
func (kc *Kucoin) Init() error {
	kc.resetBboThrottle()
	kc.spotWsPublicClosed = true
	kc.spotWsPrivateClosed = true
	return nil
}
func (kc *Kucoin) getSpotSymbol(symbol string) string {
	kcSpotSymbolMapMtx.RLock()
	defer kcSpotSymbolMapMtx.RUnlock()
	return kcSpotSymbolMap[symbol]
}

// buildHeaders 私有接口的请求头
// 签名: base64(hmac-sha256(secretkey, timestamp+method+requestPath+queryString+body))
// passphrase: base64(hmac-sha256(secretkey, passphrase))
func (kc *Kucoin) buildHeaders(method, path, params, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	h := hmac.New(sha256.New, []byte(kc.secretkey))
	h.Write([]byte(ts + method + path + params + body))
	ph := hmac.New(sha256.New, []byte(kc.secretkey))
	ph.Write([]byte(kc.passphrase))
	return map[string]string{
		"KC-API-KEY":         kc.apikey,
		"KC-API-SIGN":        base64.StdEncoding.EncodeToString(h.Sum(nil)),
		"KC-API-TIMESTAMP":   ts,
		"KC-API-PASSPHRASE":  base64.StdEncoding.EncodeToString(ph.Sum(nil)),
		"KC-API-KEY-VERSION": "2",
		"Content-Type":       "application/json",
	}
}
func (kc *Kucoin) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(kc.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(kc.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(kc.Name() + " " + api + " " + ret.Code + " " + ret.Msg)
}
func (kc *Kucoin) toStdSide(side string) string {
	if side == "BUY" {
		return "BUY"
	} else if side == "SELL" {
		return "SELL"
	}
	return ""
}
func (kc *Kucoin) fromStdSide(side string) string {
	if side == "BUY" {
		return "BUY"
	} else if side == "SELL" {
		return "SELL"
	}
	return ""
}
func (kc *Kucoin) toStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "LIMIT"
	} else if orderType == "MARKET" {
		return "MARKET"
	}
	return ""
}
func (kc *Kucoin) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" || orderType == "MARKET" {
		return orderType
	}
	return ""
}
func (kc *Kucoin) toStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" || timeInForce == "IOC" || timeInForce == "FOK" {
		return timeInForce
	}
	return ""
}
func (kc *Kucoin) fromStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" || timeInForce == "IOC" || timeInForce == "FOK" {
		return timeInForce
	}
	return ""
}

// toStdOrderStatus 0未触发 1已触发 2未成交 3已成交 4部分成交 5已撤销 6部分成交后撤销
func (kc *Kucoin) toStdOrderStatus(status int) string {
	if status == 0 || status == 1 || status == 2 {
		return "NEW"
	} else if status == 3 {
		return "FILLED"
	} else if status == 4 {
		return "PARTIALLY_FILLED"
	} else if status == 5 || status == 6 {
		return "CANCELED"
	}
	return ""
}
func (kc *Kucoin) toStdWithdrawStatus(status string) string {
	if status == "SUCCESS" {
		return "COMPLETED"
	} else if status == "PROCESSING" {
		return "PENDING"
	} else if status == "FAILURE" {
		return "FAILED"
	} else if status == "CANCELLED" || status == "CANCELED" {
		return "CANCELED"
	}
	return ""
}
