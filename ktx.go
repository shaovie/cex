package cex

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Ktx struct {
	Unsupported
	CexOptions
	Http
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

func NewKtx(localIP string) (*Ktx, error) {
	client, err := NewClient("ktx", localIP)
	if err != nil {
		return nil, err
	}
	cexObj := &Ktx{
		Http: Http{client: client},
		name: "ktx",
	}
	return cexObj, nil
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
	ktx.resetBboThrottle()
	ktx.spotWsPublicClosed = true
	return nil
}
func (ktx *Ktx) getSpotSymbol(symbol string) string {
	ktxSpotSymbolMapMtx.RLock()
	defer ktxSpotSymbolMapMtx.RUnlock()
	return ktxSpotSymbolMap[symbol]
}
