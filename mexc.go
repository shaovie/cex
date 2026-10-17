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
)

type Mexc struct {
	Unsupported
	CexOptions
	Http
	name      string
	account   string
	apikey    string
	secretkey string
	localIP   string
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
	spotWsPrivateListenKey string
}

// McSubscribeArg 订阅/退订参数
type McSubscribeArg struct {
	Id     string   `json:"id,omitempty"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}

const mcUniEndpoint = "https://api.mexc.com"
const mcApiDeadline = 1500 * time.Millisecond

func NewMexc(account, apikey, secretkey, localIP string) (*Mexc, error) {
	client, err := NewClient("mexc", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Mexc{
		Http: Http{
			client: client,
		},
		name:      "mexc",
		account:   account,
		apikey:    apikey,
		secretkey: secretkey,
		localIP:   localIP,
	}
	return cexObj, nil
}
func (mc *Mexc) Name() string {
	return mc.name
}
func (mc *Mexc) Account() string {
	return mc.account
}
func (mc *Mexc) ApiKey() string {
	return mc.apikey
}
func (mc *Mexc) Debug(v bool) {
	mc.debug = v
}
func (mc *Mexc) Init() error {
	mc.spotWsPublicClosed = true
	mc.spotWsPrivateClosed = true
	return nil
}

// buildQuery 私有接口参数: params+timestamp+signature, 统一放query string
// 签名: hmac-sha256(secretkey, params&timestamp=xxx) 小写hex, apiKey放请求头X-MEXC-APIKEY
func (mc *Mexc) buildQuery(params string) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	return mc.signParams(params, ts)
}
func (mc *Mexc) signParams(params, ts string) string {
	msg := "timestamp=" + ts
	if params != "" {
		msg = params + "&timestamp=" + ts
	}
	h := hmac.New(sha256.New, []byte(mc.secretkey))
	h.Write([]byte(msg))
	return msg + "&signature=" + hex.EncodeToString(h.Sum(nil))
}
func (mc *Mexc) buildHeaders() map[string]string {
	return map[string]string{
		"X-MEXC-APIKEY": mc.apikey,
		"Content-Type":  "application/json",
	}
}
func (mc *Mexc) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(mc.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(mc.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(mc.Name() + " " + api + " " + ret.Msg)
}
func (mc *Mexc) toStdSide(side string) string {
	if side == "BUY" || side == "SELL" {
		return side
	}
	return ""
}
func (mc *Mexc) fromStdSide(side string) string {
	if side == "BUY" || side == "SELL" {
		return side
	}
	return ""
}
func (mc *Mexc) toStdOrderType(orderType string) string {
	if orderType == "LIMIT" || orderType == "LIMIT_MAKER" ||
		orderType == "IMMEDIATE_OR_CANCEL" || orderType == "FILL_OR_KILL" {
		return "LIMIT"
	} else if orderType == "MARKET" {
		return "MARKET"
	}
	return ""
}
func (mc *Mexc) toStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" || timeInForce == "IOC" || timeInForce == "FOK" {
		return timeInForce
	}
	return ""
}
func (mc *Mexc) toStdOrderStatus(status string) string {
	if status == "NEW" || status == "PARTIALLY_FILLED" || status == "FILLED" ||
		status == "CANCELED" || status == "REJECTED" || status == "EXPIRED" {
		return status
	}
	return ""
}

// toStdPrivateOrderStatus 私有推送的status: 1 未成交 2 全部成交 3 部分成交 4 已撤销 5 部分撤销
func (mc *Mexc) toStdPrivateOrderStatus(status int) string {
	if status == 1 {
		return "NEW"
	} else if status == 2 {
		return "FILLED"
	} else if status == 3 {
		return "PARTIALLY_FILLED"
	} else if status == 4 || status == 5 {
		return "CANCELED"
	}
	return ""
}

// toStdPrivateOrderType 私有推送的orderType: 1 LIMIT_ORDER 2 POST_ONLY 3 IOC 4 FOK
// 5 MARKET_ORDER 100 止盈止损
func (mc *Mexc) toStdPrivateOrderType(orderType int) string {
	if orderType >= 1 && orderType <= 4 {
		return "LIMIT"
	} else if orderType == 5 {
		return "MARKET"
	}
	return ""
}

// toStdWithdrawStatus 1 APPLY 2 AUDITING 3 WAIT 4 PROCESSING 5 WAIT_PACKAGING 6 WAIT_CONFIRM
// 7 SUCCESS 8 FAILED 9 CANCEL 10 MANUAL
func (mc *Mexc) toStdWithdrawStatus(status int) string {
	if status == 7 {
		return "COMPLETED"
	} else if status == 8 {
		return "FAILED"
	} else if status == 9 {
		return "CANCELED"
	} else if status >= 1 && status <= 6 || status == 10 {
		return "PENDING"
	}
	return ""
}
