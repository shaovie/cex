package cex

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Ktx struct {
	Unsupported
	name string

	// spot websocket
	spotWsPublicConn      *websocket.Conn
	spotWsPublicConnMtx   sync.Mutex
	spotWsPublicClosed    bool
	spotWsPublicClosedMtx sync.RWMutex
}

var (
	ktxSpotSymbolMap    map[string]string
	ktxSpotSymbolMapMtx sync.RWMutex
)

const ktxSpotEndpoint = "https://api.ktx.com/api"
const ktxApiDeadline = 1500 * time.Millisecond

func init() {
	ktxSpotSymbolMap = make(map[string]string)
}

func NewKtx() *Ktx {
	cexObj := &Ktx{
		name: "ktx",
	}
	return cexObj
}
func (ktx *Ktx) Name() string {
	return ktx.name
}
func (ktx *Ktx) Account() string {
	return ""
}
func (ktx *Ktx) ApiKey() string {
	return ""
}
func (ktx *Ktx) Debug(v bool) {
}
func (ktx *Ktx) Init() error {
	ktx.spotWsPublicClosed = true
	return nil
}
func (ktx *Ktx) getSpotSymbol(symbol string) string {
	ktxSpotSymbolMapMtx.RLock()
	defer ktxSpotSymbolMapMtx.RUnlock()
	return ktxSpotSymbolMap[symbol]
}
