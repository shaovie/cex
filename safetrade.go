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

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Safetrade struct {
	Unsupported
	Http
	name      string
	account   string
	apikey    string
	secretkey string
	localIP   string

	// spot websocket
	spotWsPublicConn       *websocket.Conn
	spotWsPublicConnMtx    sync.Mutex
	spotWsPublicClosed     bool
	spotWsPublicClosedMtx  sync.RWMutex
	spotWsPublicBBOStreams map[string]bool

	// depth频道推的是增量, 本地按symbol维护盘口; bids降序, asks升序, Min()即最优价
	spotWsOrderBookBids map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
	spotWsOrderBookAsks map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
}

var (
	stSpotSymbolMap    map[string]string // 标准符号(BTCUSDT) -> 交易所symbol(btcusdt)
	stSpotSymbolMapMtx sync.RWMutex
)

const stEndpoint = "https://safe.trade/api/v2"
const stWsOrigin = "https://safe.trade"
const stApiDeadline = 1500 * time.Millisecond

// 该站WAF会拦非浏览器UA的请求, REST与WS都必须带上
const stUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

var stHttpHeaders = map[string]string{
	"User-Agent": stUserAgent,
	"Accept":     "application/json",
}

func init() {
	stSpotSymbolMap = make(map[string]string)
}

func NewSafetrade(account, apikey, secretkey, localIP string) (*Safetrade, error) {
	client, err := NewClient("safetrade", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Safetrade{
		Http: Http{
			client: client,
		},
		name:      "safetrade",
		account:   account,
		apikey:    apikey,
		secretkey: secretkey,
		localIP:   localIP,
	}
	return cexObj, nil
}
func (sa *Safetrade) Name() string {
	return sa.name
}
func (sa *Safetrade) Account() string {
	return sa.account
}
func (sa *Safetrade) ApiKey() string {
	return sa.apikey
}
func (sa *Safetrade) Debug(v bool) {
}
func (sa *Safetrade) Init() error {
	sa.spotWsPublicClosed = true
	sa.spotWsOrderBookBids = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 16)
	sa.spotWsOrderBookAsks = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 16)
	sa.spotWsPublicBBOStreams = make(map[string]bool, 4)
	return nil
}
func (sa *Safetrade) getSpotSymbol(symbol string) string {
	stSpotSymbolMapMtx.RLock()
	defer stSpotSymbolMapMtx.RUnlock()
	return stSpotSymbolMap[symbol]
}

// buildHeaders 私有接口的请求头, 签名: HMAC-SHA256(secretkey, nonce+apikey), nonce为毫秒时间戳
func (sa *Safetrade) buildHeaders() map[string]string {
	nonce := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := make(map[string]string, len(stHttpHeaders)+4)
	for k, v := range stHttpHeaders {
		headers[k] = v
	}
	headers["Content-Type"] = "application/json"
	headers["X-Auth-Apikey"] = sa.apikey
	headers["X-Auth-Nonce"] = nonce
	headers["X-Auth-Signature"] = sa.sign(nonce)
	return headers
}
func (sa *Safetrade) sign(nonce string) string {
	h := hmac.New(sha256.New, []byte(sa.secretkey))
	h.Write([]byte(nonce + sa.apikey))
	return hex.EncodeToString(h.Sum(nil))
}
func (sa *Safetrade) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(sa.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Errors []string `json:"errors,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(sa.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	if len(ret.Errors) == 0 {
		return errors.New(sa.Name() + " " + api + " " + string(resp))
	}
	return errors.New(sa.Name() + " " + ret.Errors[0])
}
func (sa *Safetrade) toStdSide(side string) string {
	if side == "buy" {
		return "BUY"
	} else if side == "sell" {
		return "SELL"
	}
	return ""
}
func (sa *Safetrade) fromStdSide(side string) string {
	if side == "BUY" {
		return "buy"
	} else if side == "SELL" {
		return "sell"
	}
	return ""
}
func (sa *Safetrade) toStdOrderType(orderType string) string {
	if orderType == "limit" {
		return "LIMIT"
	} else if orderType == "market" || orderType == "market_quote" {
		return "MARKET"
	}
	return ""
}
func (sa *Safetrade) fromStdOrderType(orderType string) string {
	if orderType == "LIMIT" {
		return "limit"
	} else if orderType == "MARKET" {
		return "market"
	}
	return ""
}

// toStdOrderState state: pending/wait/done/cancel/rejected
func (sa *Safetrade) toStdOrderState(state string) string {
	if state == "pending" || state == "wait" {
		return "NEW"
	} else if state == "done" {
		return "FILLED"
	} else if state == "cancel" {
		return "CANCELED"
	} else if state == "rejected" {
		return "REJECTED"
	}
	return ""
}

// toStdWithdrawStatus 提现状态
func (sa *Safetrade) toStdWithdrawStatus(status string) string {
	if status == "prepared" || status == "accepted" || status == "processing" ||
		status == "under_review" || status == "confirming" {
		return "PENDING"
	} else if status == "succeed" {
		return "COMPLETED"
	} else if status == "canceled" {
		return "CANCELED"
	} else if status == "rejected" || status == "to_reject" {
		return "REJECTED"
	} else if status == "failed" || status == "errored" || status == "skipped" {
		return "FAILED"
	}
	return ""
}
