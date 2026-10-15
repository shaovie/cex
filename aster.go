package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/gorilla/websocket"
)

type Aster struct {
	Unsupported
	Http
	name      string
	account   string
	apikey    string // API Wallet的地址(signer)
	secretkey string
	localIP   string
	debug     bool
	privKey   *secp256k1.PrivateKey // API Wallet私钥, 用于EIP-712签名
	nonceMtx  sync.Mutex
	nonce     int64

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

// AsSubscribeArg 订阅/退订参数, id必须是自增整数
type AsSubscribeArg struct {
	Id     int64    `json:"id,omitempty"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}

const asSpotEndpoint = "https://sapi.asterdex.com"
const asApiDeadline = 1500 * time.Millisecond

func NewAster(account, apikey, secretkey, localIP string) (*Aster, error) {
	var privKey *secp256k1.PrivateKey
	if len(secretkey) != 0 { // API Wallet私钥, 用于签名
		var err error
		if privKey, err = asPrivKey(secretkey); err != nil {
			return nil, err
		}
		signer := asAddress(privKey)
		if len(apikey) == 0 {
			apikey = signer
		} else if !strings.EqualFold(apikey, signer) {
			return nil, errors.New("aster signer private key not match address! " + apikey)
		}
	}
	client, err := NewClient("aster", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Aster{
		Http: Http{
			client: client,
		},
		name:      "aster",
		account:   account,
		apikey:    apikey,
		secretkey: secretkey,
		localIP:   localIP,
		privKey:   privKey,
	}
	return cexObj, nil
}
func (as *Aster) Name() string {
	return as.name
}
func (as *Aster) Account() string {
	return as.account
}
func (as *Aster) ApiKey() string {
	return as.apikey
}
func (as *Aster) Debug(v bool) {
	as.debug = v
}
func (as *Aster) Init() error {
	as.spotWsPublicClosed = true
	as.spotWsPrivateClosed = true
	return nil
}

// buildQuery 私有接口参数: params+nonce+signer+signature, 统一放query string
// 签名: EIP-712(API Wallet私钥, params&nonce=xxx&signer=0x...)
func (as *Aster) buildQuery(params string) string {
	msg := "nonce=" + strconv.FormatInt(as.nextNonce(), 10) + "&signer=" + as.apikey
	if params != "" {
		msg = params + "&" + msg
	}
	return msg + "&signature=" + asSign(as.privKey, msg)
}
func (as *Aster) nextNonce() int64 {
	as.nonceMtx.Lock()
	defer as.nonceMtx.Unlock()
	nonce := time.Now().UnixMicro()
	if nonce <= as.nonce { // 同一微秒内递增, 保证唯一
		nonce = as.nonce + 1
	}
	as.nonce = nonce
	return nonce
}
func (as *Aster) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(as.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(as.Name() + " " + api + " " + ret.Msg)
}
func (as *Aster) toStdSide(side string) string {
	if side == "BUY" || side == "Buy" {
		return "BUY"
	} else if side == "SELL" || side == "Sell" {
		return "SELL"
	}
	return ""
}
func (as *Aster) toStdOrderType(orderType string) string {
	if orderType == "LIMIT" || orderType == "LIMIT_MAKER" || orderType == "IMMEDIATE_OR_CANCEL" ||
		orderType == "FILL_OR_KILL" {
		return "LIMIT"
	} else if orderType == "MARKET" {
		return "MARKET"
	}
	return ""
}
func (as *Aster) toStdTimeInForce(timeInForce string) string {
	if timeInForce == "GTC" || timeInForce == "IOC" || timeInForce == "FOK" {
		return timeInForce
	} else if timeInForce == "GTX" {
		return "GTC"
	}
	return ""
}
func (as *Aster) toStdOrderStatus(status string) string {
	if status == "NEW" || status == "PARTIALLY_FILLED" || status == "FILLED" ||
		status == "CANCELED" || status == "REJECTED" || status == "EXPIRED" {
		return status
	}
	return ""
}
