package cex

import (
	"errors"

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
	SpotServerTime() (int64, error)
	SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error)
	SpotGetAll24hTicker() (map[string]Pub24hTicker, error) // bigone 不支持
	SpotGetBBO(symbol string) (BestBidAsk, error)
	SpotGetAllAssets() (map[string]*SpotAsset, error)

	// 市价BUY qty=quote amt, 市价SELL qty=base qty, 参数涵义参考 struct SpotOrder
	SpotPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
		side, timeInForce, orderType string) (string, error)
	// orderId, cltId 二选一
	SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error
	// orderId, cltId 二选一
	SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error)
	SpotGetOpenOrders(symbol string) ([]*SpotOrder, error)

	//= ws public
	// cex object 如果closed需要重新连接时，请不要复用，一定要创建新的obj
	SpotWsPublicOpen() error
	// channels: orderbook5@symbolA,symbolB (5档)
	//           bbo@symbolA,symbolB     // 最优买卖价 只binance实现
	//           ticker@symbolA,symbolB     // bigone不支持
	SpotWsPublicSubscribe(channels []string)
	SpotWsPublicUnsubscribe(channels []string)
	SpotWsPublicTickerPoolPut(v any)
	SpotWsPublicOrderBook5PoolPut(v any)
	SpotWsPublicBBOPoolPut(v any)
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
	SpotWsCancelOrder(symbol, orderId, cltId string) (string, error)

	//= futures, typ=UM,U本位 typ=CM,币本位
	FuturesSupported(typ string) bool
	FuturesServerTime(typ string) (int64, error)
	FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error)
	FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal
	FuturesGetAll24hTicker(typ string) (map[string]Pub24hTicker, error)
	FuturesGetBBO(typ, symbol string) (BestBidAsk, error)
	FuturesGetAllFundingRate(typ string) (map[string]FundingRate, error)
	// for binance
	FuturesGetFundingRateMarkPrice(typ, symbol string) (FundingRateMarkPrice, error)
	FuturesGetAllAssets(typ string) (map[string]*FuturesAsset, error)
	// interval 1m,5m,30m,1h,6h,12h,1d startTime/endTime is second
	// 返回顺序[11:15:00,11:16:00,11:17:00]
	FuturesGetKLine(typ, symbol, interval string, startTime, endTime, lmt int64) ([]KLine, error)
	FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error)
	FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal
	// CM中 qty为合约张数
	FuturesPlaceOrder(typ, symbol, clientId string,
		price, qty decimal.Decimal, side, orderType, timeInForce string,
		positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error)
	FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error)
	FuturesGetOpenOrders(typ, symbol string) ([]*FuturesOrder, error)
	FuturesCancelOrder(typ string, symbol /*BTCUSDT*/, orderId, cltId string) error
	//  单仓:0/双仓:1 切换
	FuturesSwitchPositionMode(typ string, mode int) error
	//  全仓:0/逐仓:1 切换
	FuturesSwitchTradeMode(typ, symbol string /*BTCUSDT*/, mode, leverage int) error
	// 获取交易对的杠杆分层标准 For binance
	FuturesMaintMargin(typ, symbol string) ([]*FuturesLeverageBracket, error)

	// ws
	// channels: orderbook5@symbolA,symbolB
	//           bbo@symbolA,symbolB     // 最优买卖价 只binance实现
	//           ticker@symbol,symbol2
	FuturesWsPublicOpen(typ string) error
	FuturesWsPublicSubscribe(channels []string)
	FuturesWsPublicUnsubscribe(channels []string)
	FuturesWsPublicTickerPoolPut(v any)
	FuturesWsPublicOrderBook5PoolPut(v any)
	FuturesWsPublicBBOPoolPut(v any)
	// Loop结束时会close(ch)
	FuturesWsPublicLoop(ch chan<- any)
	FuturesWsPublicClose()
	FuturesWsPublicIsClosed() bool

	// priv
	FuturesWsPrivateOpen(typ string) error
	// channels: orders
	//           positions
	FuturesWsPrivateSubscribe(channels []string)
	FuturesWsPrivateLoop(ch chan<- any)
	FuturesWsPrivateClose()
	FuturesWsPrivateIsClosed() bool

	// return reqId,err, CM中 qty为合约张数,
	FuturesWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
		side, orderType, timeInForce string,
		positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error)
	FuturesWsCancelOrder(symbol, orderId, cltId string) (string, error)

	//= 统一账户
	// rest api
	UnifiedGetAssets() (map[string]*UnifiedAsset, error)

	// ws
	UnifiedWsOpen() error
	// channels: balance@symbol1,symbol2,symbol3
	UnifiedWsSubscribe(channels []string)
	UnifiedWsLoop(ch chan<- any)
	UnifiedWsClose()
	UnifiedWsIsClosed() bool

	//= wallet
	// chain: TRX/MOB
	Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error)
	CancelWithdrawal(wid string) error
	//GetWithdrawalResult(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error)
	// from,to:FUNDING,SPOT,UM_FUTURE,CM_FUTURE,UNIFIED
	Transfer(symbol, from, to string, qty decimal.Decimal) error
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
	//CexList["mexc"] = "Mexc"
	//CexList["bybit"] 	= "Bybit"
	//CexList["bitget"] 	= "Bitget"

	CexSXList = make(map[string]string)
	CexSXList["binance"] = "BN"
	CexSXList["gate"] = "GT"
	CexSXList["okx"] = "OK"
	CexSXList["bybit"] = "BY"
	CexSXList["bigone"] = "BO"
	CexSXList["mexc"] = "MC"

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
