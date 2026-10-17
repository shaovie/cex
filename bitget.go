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

type Bitget struct {
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

const bgEndpoint = "https://api.bitget.com"
const bgApiDeadline = 1500 * time.Millisecond

func NewBitget(account, apikey, secretkey, passphrase, localIP string) (*Bitget, error) {
	client, err := NewClient("bitget", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Bitget{
		Http: Http{
			client: client,
		},
		name:       "bitget",
		account:    account,
		apikey:     apikey,
		secretkey:  secretkey,
		passphrase: passphrase,
		localIP:    localIP,
	}
	return cexObj, nil
}
func (bg *Bitget) Name() string {
	return bg.name
}
func (bg *Bitget) Account() string {
	return bg.account
}
func (bg *Bitget) ApiKey() string {
	return bg.apikey
}
func (bg *Bitget) Debug(v bool) {
	bg.debug = v
}
func (bg *Bitget) Init() error {
	bg.resetBboThrottle()
	bg.spotWsPublicClosed = true
	bg.spotWsPrivateClosed = true
	return nil
}

// buildHeaders 私有接口的请求头
// 签名: base64(hmac-sha256(secretkey, timestamp+method+requestPath+"?"+query+body)), 无query时不含"?"
func (bg *Bitget) buildHeaders(method, path, params, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	pre := ts + method + path
	if params != "" {
		pre += "?" + params
	}
	pre += body

	h := hmac.New(sha256.New, []byte(bg.secretkey))
	h.Write([]byte(pre))
	return map[string]string{
		"ACCESS-KEY":        bg.apikey,
		"ACCESS-SIGN":       base64.StdEncoding.EncodeToString(h.Sum(nil)),
		"ACCESS-TIMESTAMP":  ts,
		"ACCESS-PASSPHRASE": bg.passphrase,
		"Content-Type":      "application/json",
		"locale":            "zh-CN",
	}
}

// wsLoginSign websocket登录签名: base64(hmac-sha256(secretkey, timestamp+"GET"+"/user/verify"))
func (bg *Bitget) wsLoginSign() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := hmac.New(sha256.New, []byte(bg.secretkey))
	h.Write([]byte(ts + "GET" + "/user/verify"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
func (bg *Bitget) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(bg.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bg.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(bg.Name() + " " + api + " " + ret.Code + " " + ret.Msg)
}
func (bg *Bitget) toStdSide(side string) string {
	if side == "buy" {
		return "BUY"
	} else if side == "sell" {
		return "SELL"
	}
	return ""
}
func (bg *Bitget) fromStdSide(side string) string {
	if side == "BUY" {
		return "buy"
	} else if side == "SELL" {
		return "sell"
	}
	return ""
}
func (bg *Bitget) toStdOrderType(orderType string) string {
	if orderType == "limit" {
		return "LIMIT"
	} else if orderType == "market" {
		return "MARKET"
	}
	return ""
}
func (bg *Bitget) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "limit"
	} else if orderType == "MARKET" {
		return "market"
	}
	return ""
}
func (bg *Bitget) toStdTimeInForce(timeInForce string) string {
	if timeInForce == "gtc" || timeInForce == "post_only" {
		return "GTC"
	} else if timeInForce == "ioc" {
		return "IOC"
	} else if timeInForce == "fok" {
		return "FOK"
	}
	return ""
}
func (bg *Bitget) fromStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" {
		return "gtc"
	} else if timeInForce == "IOC" {
		return "ioc"
	} else if timeInForce == "FOK" {
		return "fok"
	}
	return ""
}

// toStdOrderStatus live未成交/new已进入撮合/partially_filled/filled/canceled
func (bg *Bitget) toStdOrderStatus(status string) string {
	if status == "live" || status == "new" {
		return "NEW"
	} else if status == "partially_filled" {
		return "PARTIALLY_FILLED"
	} else if status == "filled" {
		return "FILLED"
	} else if status == "canceled" || status == "cancelled" {
		return "CANCELED"
	}
	return ""
}
func (bg *Bitget) toStdWithdrawStatus(status string) string {
	if status == "pending" {
		return "PENDING"
	} else if status == "success" {
		return "COMPLETED"
	} else if status == "fail" {
		return "FAILED"
	}
	return ""
}
