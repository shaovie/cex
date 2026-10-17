package cex

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/sha3"
)

type Aster struct {
	Unsupported
	CexOptions
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

	// futures websocket
	futuresWsPublicConn      *websocket.Conn
	futuresWsPublicConnMtx   sync.Mutex
	futuresWsPublicClosed    bool
	futuresWsPublicClosedMtx sync.RWMutex

	futuresWsPrivateConn      *websocket.Conn
	futuresWsPrivateConnMtx   sync.Mutex
	futuresWsPrivateClosed    bool
	futuresWsPrivateClosedMtx sync.RWMutex
	futuresWsPrivateListenKey string
}

// AsSubscribeArg 订阅/退订参数, id必须是自增整数
type AsSubscribeArg struct {
	Id     int64    `json:"id,omitempty"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}

const asSpotEndpoint = "https://sapi.asterdex.com"
const asFuturesEndpoint = "https://fapi.asterdex.com"
const asApiDeadline = 1500 * time.Millisecond

func NewAster(account, apikey, secretkey, localIP string) (*Aster, error) {
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
	}
	if len(secretkey) != 0 { // API Wallet私钥, 用于签名
		if cexObj.privKey, err = cexObj.parsePrivKey(secretkey); err != nil {
			return nil, err
		}
		signer := cexObj.address(cexObj.privKey)
		if len(apikey) == 0 {
			cexObj.apikey = signer
		} else if !strings.EqualFold(apikey, signer) {
			return nil, errors.New("aster signer private key not match address! " + apikey)
		}
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
	as.futuresWsPublicClosed = true
	as.futuresWsPrivateClosed = true
	return nil
}

// buildQuery 私有接口参数放query string: params+nonce+signer+signature (GET/DELETE用)
// 签名: EIP-712(API Wallet私钥, query+body)
func (as *Aster) buildQuery(params string) string {
	msg := "nonce=" + strconv.FormatInt(as.nextNonce(), 10) + "&signer=" + as.apikey
	if params != "" {
		msg = params + "&" + msg
	}
	return msg + "&signature=" + as.sign(msg)
}

// buildBody 私有接口参数放请求体(POST用): params+nonce+signer+signature
// 签名: EIP-712(API Wallet私钥, query+body), query为空时就是body本身
func (as *Aster) buildBody(params string) []byte {
	msg := "nonce=" + strconv.FormatInt(as.nextNonce(), 10) + "&signer=" + as.apikey
	if params != "" {
		msg = params + "&" + msg
	}
	return []byte(msg + "&signature=" + as.sign(msg))
}
func (as *Aster) buildHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}
}

// parsePrivKey 解析API Wallet私钥, 支持带0x前缀
func (as *Aster) parsePrivKey(v string) (*secp256k1.PrivateKey, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
	if err != nil || len(b) != 32 {
		return nil, errors.New("aster invalid signer private key")
	}
	return secp256k1.PrivKeyFromBytes(b), nil
}

// address 私钥对应的地址
func (as *Aster) address(priv *secp256k1.PrivateKey) string {
	return as.pubKeyAddress(priv.PubKey())
}

// pubKeyAddress 公钥对应的地址
func (as *Aster) pubKeyAddress(pub *secp256k1.PublicKey) string {
	raw := pub.SerializeUncompressed() // 65字节, 首字节是0x04
	sum := as.keccak256(raw[1:])
	return "0x" + hex.EncodeToString(sum[12:])
}

func (as *Aster) keccak256(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}

// sign EIP-712签名, 返回0x开头的65字节签名(r+s+v, v为27/28)
// 被签名的msg是urlencode后的参数串(含nonce和signer), 签名结果放signature参数
func (as *Aster) sign(msg string) string {
	if as.privKey == nil { // 没配置API Wallet私钥
		return ""
	}
	compact := ecdsa.SignCompact(as.privKey, as.eip712Digest(msg), false)
	sig := make([]byte, 65)
	copy(sig[0:32], compact[1:33])
	copy(sig[32:64], compact[33:65])
	sig[64] = compact[0] // 27/28
	return "0x" + hex.EncodeToString(sig)
}

// eip712Digest EIP-712摘要: keccak256(0x1901 || domainSeparator || messageHash)
// domain = {name:"AsterSignTransaction", version:"1", chainId:1666, verifyingContract:0x0}
// types  = EIP712Domain(name,version,chainId,verifyingContract) + Message(msg string)
func (as *Aster) eip712Digest(msg string) []byte {
	domainTypeHash := as.keccak256([]byte("EIP712Domain(string name,string version," +
		"uint256 chainId,address verifyingContract)"))
	nameHash := as.keccak256([]byte("AsterSignTransaction"))
	verHash := as.keccak256([]byte("1"))
	domain := make([]byte, 0, 160)
	domain = append(domain, domainTypeHash...)
	domain = append(domain, nameHash...)
	domain = append(domain, verHash...)
	var chainId [32]byte
	binary.BigEndian.PutUint64(chainId[24:], asChainID)
	domain = append(domain, chainId[:]...)       // uint256
	domain = append(domain, make([]byte, 32)...) // address 0x0

	data := make([]byte, 0, 64)
	data = append(data, as.keccak256([]byte("Message(string msg)"))...)
	data = append(data, as.keccak256([]byte(msg))...)

	digest := make([]byte, 0, 66)
	digest = append(digest, 0x19, 0x01)
	digest = append(digest, as.keccak256(domain)...)
	digest = append(digest, as.keccak256(data)...)
	return as.keccak256(digest)
}

const asChainID = 1666 // 签名域的chainId

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
		Code  int    `json:"code,omitempty"`
		Msg   string `json:"msg,omitempty"`
		Error string `json:"error,omitempty"` // 有些会返回 error
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " " + api + " " + err.Error())
	}
	return errors.New(as.Name() + " " + api + " " + ret.Msg + " " + ret.Error)
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
