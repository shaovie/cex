package cex

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type BnAlpha struct {
	Unsupported
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
}

// alpha的symbol是 ALPHA_<tokenId><quote>(比如ALPHA_173USDT), 对外的标准symbol是BASEQUOTE(比如ZKJUSDT)
// 两个map由 SpotLoadAllPairRule 从 Token列表 + 交易对信息 里生成
var (
	baSpotSymbolMap    map[string]string // 标准symbol -> 交易所symbol
	baSpotSymbolStdMap map[string]string // 交易所symbol -> 标准symbol
	baSpotSymbolMapMtx sync.RWMutex
)

func init() {
	baSpotSymbolMap = make(map[string]string)
	baSpotSymbolStdMap = make(map[string]string)
}

// BaSubscribeArg 订阅/退订参数, id必须是整数
type BaSubscribeArg struct {
	Id     int64    `json:"id,omitempty"`
	Method string   `json:"method"`
	Params []string `json:"params,omitempty"`
}

const baEndpoint = "https://www.binance.com"
const baApiDeadline = 1500 * time.Millisecond
const baRuleDeadline = 10 * time.Second // 规则接口返回近2MB数据, 给长一点

func NewBnAlpha(account, apikey, secretkey, localIP string) (*BnAlpha, error) {
	client, err := NewClient("bnalpha", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &BnAlpha{
		Http: Http{
			client: client,
		},
		name:      "bnalpha",
		account:   account,
		apikey:    apikey,
		secretkey: secretkey,
		localIP:   localIP,
	}
	return cexObj, nil
}
func (ba *BnAlpha) Name() string {
	return ba.name
}
func (ba *BnAlpha) Account() string {
	return ba.account
}
func (ba *BnAlpha) ApiKey() string {
	return ba.apikey
}
func (ba *BnAlpha) Debug(v bool) {
	ba.debug = v
}
func (ba *BnAlpha) Init() error {
	ba.spotWsPublicClosed = true
	return nil
}
func (ba *BnAlpha) getSpotSymbol(symbol string) string { // 标准symbol -> 交易所symbol
	baSpotSymbolMapMtx.RLock()
	defer baSpotSymbolMapMtx.RUnlock()
	return baSpotSymbolMap[symbol]
}
func (ba *BnAlpha) getStdSymbol(symbol string) string { // 交易所symbol -> 标准symbol
	baSpotSymbolMapMtx.RLock()
	defer baSpotSymbolMapMtx.RUnlock()
	if std, ok := baSpotSymbolStdMap[symbol]; ok {
		return std
	}
	return symbol
}
func (ba *BnAlpha) handleExceptionResp(api string, resp []byte) error {
	if len(resp) == 0 {
		return errors.New(ba.Name() + " " + api + " resp empty")
	}
	ret := struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	}{}
	if err := json.Unmarshal(resp, &ret); err != nil {
		return errors.New(ba.Name() + " " + api + " " + err.Error() + " " + string(resp))
	}
	return errors.New(ba.Name() + " " + api + " " + ret.Code + " " + ret.Message)
}
