package cex

import (
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type Safetrade struct {
	Unsupported
	Http
	name    string
	localIP string

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
		name:    "safetrade",
		localIP: localIP,
	}
	return cexObj, nil
}
func (sa *Safetrade) Name() string {
	return sa.name
}
func (sa *Safetrade) Account() string {
	return ""
}
func (sa *Safetrade) ApiKey() string {
	return ""
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
