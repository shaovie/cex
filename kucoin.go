package cex

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Kucoin struct {
	Unsupported
	name string

	// spot websocket
	spotWsPublicConn      *websocket.Conn
	spotWsPublicConnMtx   sync.Mutex
	spotWsPublicClosed    bool
	spotWsPublicClosedMtx sync.RWMutex
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

func NewKucoin() *Kucoin {
	cexObj := &Kucoin{
		name: "kucoin",
	}
	return cexObj
}
func (kc *Kucoin) Name() string {
	return kc.name
}
func (kc *Kucoin) Account() string {
	return ""
}
func (kc *Kucoin) ApiKey() string {
	return ""
}
func (kc *Kucoin) Debug(v bool) {
}
func (kc *Kucoin) Init() error {
	kc.spotWsPublicClosed = true
	return nil
}
func (kc *Kucoin) getSpotSymbol(symbol string) string {
	kcSpotSymbolMapMtx.RLock()
	defer kcSpotSymbolMapMtx.RUnlock()
	return kcSpotSymbolMap[symbol]
}
