package cex

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
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

	// 盘口: fulldepth是增量流, 本地用treemap维护, 买一卖一从本地盘口取
	// 订阅时同步取fullDepth快照(fullDepth+fulldepth都含UI和API订单, updateId同一套)
	// Loop只有一个goroutine处理盘口, 只有订阅时缓存的快照需要加锁
	spotWsSnapMtx         sync.Mutex
	spotWsSnapCache       map[string]*BnAlphaDepthSnapshot
	spotWsOrderBookBids   map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
	spotWsOrderBookAsks   map[string]*treemap.Map[decimal.Decimal, decimal.Decimal]
	spotWsOrderBookSnapId map[string]int64 // fullDepth快照的lastUpdateId
	spotWsOrderBookPrevId map[string]int64 // 最后一个已应用事件的u, 0表示快照后还没应用过
	spotWsBBOCache        map[string]*BestBidAsk
	spotWsPublicBBOSet    map[string]bool // 订阅了bbo的symbol
}

// BnAlphaDepthSnapshot fullDepth快照, 订阅时取好, Loop收到首个增量事件后用它建盘口
type BnAlphaDepthSnapshot struct {
	LastUpdateId int64
	Bids         [][2]decimal.Decimal
	Asks         [][2]decimal.Decimal
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
	ba.spotWsSnapCache = make(map[string]*BnAlphaDepthSnapshot, 16)
	ba.spotWsOrderBookBids = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 512)
	ba.spotWsOrderBookAsks = make(map[string]*treemap.Map[decimal.Decimal, decimal.Decimal], 512)
	ba.spotWsOrderBookSnapId = make(map[string]int64, 16)
	ba.spotWsOrderBookPrevId = make(map[string]int64, 16)
	ba.spotWsBBOCache = make(map[string]*BestBidAsk, 16)
	ba.spotWsPublicBBOSet = make(map[string]bool, 16)
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
