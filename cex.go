package cex

import (
	"errors"
	"sync"

	"github.com/shopspring/decimal"
)

type Exchanger interface {
	Init() error

	Name() string
	ApiKey() string
	Account() string

	//= spot
	// rest api
	SpotSupported() bool
	SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error)
	SpotGetAll24hTicker() (map[string]Spot24hTicker, error) // bigone 不支持
	SpotGetAllAssets() (map[string]*SpotAsset, error)

	// 市价BUY qty=quote amt, 市价SELL qty=base qty, 参数涵义参考 struct SpotOrder
	SpotPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
		side, timeInForce, orderType string) (string, error)
	// orderId, cltId 二选一
	SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error
	// orderId, cltId 二选一
	SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error)

	//= ws public
	// cex object 如果closed需要重新连接时，请不要复用，一定要创建新的obj
	SpotWsPublicOpen() error
	// channels: orderbook5@symbolA,symbolB
	//           ticker@symbolA,symbolB     // bigone不支持
	SpotWsPublicSubscribe(channels []string)
	SpotWsPublicUnsubscribe(channels []string)
	SpotWsPublicTickerPoolPut(v any)
	SpotWsPublicOrderBook5PoolPut(v any)
	// Loop结束时会close(ch)
	SpotWsPublicLoop(ch chan<- any)
	SpotWsPublicClose()
	SpotWsPublicIsClosed() bool

	//= ws private
	// cex object 如果closed需要重新连接时，请不要复用，一定要创建新的obj
	SpotWsPrivateOpen() error
	// channels: orders
	//           balance
	SpotWsPrivateSubscribe(channels []string)
	// Loop结束时会close(ch)
	SpotWsPrivateLoop(ch chan<- any)
	SpotWsPrivateClose()
	SpotWsPrivateIsClosed() bool
	// 市价BUY qty=quote amt, 市价SELL qty=base qty, 参数涵义参考 struct SpotOrder
	SpotWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
		side, timeInForce, orderType string) (string /*req id*/, error)
	// orderId, cltId 二选一
	SpotWsCancelOrder(symbol, orderId, cltId string) error

	//= futures, typ=UM,U本位 typ=CM,币本位
	FuturesSupported(typ string) bool
	FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error)
	FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal
	FuturesGetAll24hTicker(typ string) (map[string]Futures24hTicker, error)
	FuturesGetAllFundingRate(typ string) (map[string]FundingRate, error)
	FuturesGetKLine(typ, symbol, interval string, startTime, endTime, lmt int64) ([]KLine, error)
	FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error)
	FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal
	// interval 1m,5m,30m,1h,6h,12h,1d startTime/endTime is second
	// 返回顺序[11:15:00,11:16:00,11:17:00]
	FuturesPlaceOrder(typ, symbol, clientId string,
		price, qty decimal.Decimal, side, orderType, timeInForce string,
		positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error)
	FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error)
	FuturesCancelOrder(typ string, symbol /*BTCUSDT*/, orderId, cltId string) error
	//  单仓:0/双仓:1 切换
	FuturesSwitchPositionMode(typ string, mode int) error
	//  全仓:0/逐仓:1 切换
	FuturesSwitchTradeMode(typ string, symbol string /*BTCUSDT*/, mode, leverage int) error

	// ws
	// channels: orderbook@symbol
	//           ticker@symbol,symbol2
	WsContractPubChannelOpen() error
	WsContractPubChannelSubscribe(channels []string)
	WsContractPubChannelUnsubscribe(channels []string)
	WsContractPubChannelSetTickerPool(p *sync.Pool)
	WsContractPubChannelSetOrderBookPool(p *sync.Pool)
	WsContractPubChannelLoop(ch chan<- any)
	WsContractPubChannelClose()
	WsContractPubChannelIsClosed() bool
	// priv
	WsContractPrivChannelOpen() error
	// channels: orders
	// 		     positions
	WsContractPrivChannelSubscribe(channels []string)
	WsContractPrivChannelLoop(ch chan<- any)
	WsContractPrivChannelClose()
	WsContractPrivChannelIsClosed() bool
	WsContractPlaceOrder(category, symbol, clientId, reqId string, /*BTCUSDT*/
		price, qty decimal.Decimal, side, orderType, timeInForce string,
		positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) error
	WsContractCancelOrder(category, symbol, orderId, reqId string) error

	//= 统一账户
	// rest api
	UnifiedGetAssets() (map[string]*UnifiedAsset, error)

	// ws
	WsUnifiedChannelOpen() error
	// channels: balance@symbol1,symbol2,symbol3
	WsUnifiedChannelSubscribe(channels []string)
	WsUnifiedChannelLoop(ch chan<- any)
	WsUnifiedChannelClose()
	WsUnifiedChannelIsClosed() bool
}

var (
	CexList       map[string]string
	CexSXList     map[string]string // 缩写
	CexFeeCoinMap map[string]string
)

func Init() error {
	ruleInit()
	return nil
}

func init() {
	CexList = make(map[string]string)
	CexList["binance"] = "Binance"
	CexList["gate"] = "Gate"
	CexList["okx"] = "Okx"
	CexList["bigone"] = "BigONE"
	//CexList["bybit"] 	= "Bybit"
	//CexList["bitget"] 	= "Bitget"

	CexSXList = make(map[string]string)
	CexSXList["binance"] = "BN"
	CexSXList["gate"] = "GT"
	CexSXList["okx"] = "OK"
	CexSXList["bybit"] = "BY"
	CexSXList["bigone"] = "BO"

	CexFeeCoinMap = make(map[string]string)
	CexFeeCoinMap["gate"] = "GT"
	CexFeeCoinMap["binance"] = "BNB"
}

func New(cexName, account, apikey, secretkey, passwd string) (Exchanger, error) {
	var cexObj Exchanger
	if cexName == "binance" {
		cexObj = &Binance{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
		}
	} else if cexName == "gate" {
		cexObj = &Gate{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
		}
	} else if cexName == "okx" {
		cexObj = &Okx{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
			passwd:    passwd,
		}
	} else if cexName == "bigone" {
		cexObj = &Bigone{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
		}
	} /*else if cexName == "bybit" {
		cexObj = &Bybit{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
		}
	} else if cexName == "bitget" {
		cexObj = &Bitget{
			name:      cexName,
			account:   account,
			apikey:    apikey,
			secretkey: secretkey,
			passwd:    passwd,
		}
	} else {
		return nil, errors.New("unknown cex platform : " + cexName)
	}*/
	if err := cexObj.Init(); err != nil {
		return nil, errors.New(cexObj.Name() + " init failed! " + err.Error())
	}
	return cexObj, nil
}
