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
	Debug(v bool)

	//= spot
	// rest api
	SpotSupported() bool
	SpotServerTime() (int64, error) // millisecond
	SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error)
	SpotGetAll24hTicker() (map[string]Pub24hTicker, error) // bigone 不支持
	// 获取订单簿买1/卖1挂单数据
	SpotGetBBO(symbol string) (BestBidAsk, error)
	SpotGetAllAssets() (map[string]*SpotAsset, error)
	IsXStock(symbol /*AAPLxUSD*/ string) bool

	// 市价 amt/qty任选1(优先amt) binance全支持, bigone只qty, gate,okx只amt
	// 限价 只能qty=base qty, 参数涵义参考 struct SpotOrder
	SpotPlaceOrder(symbol, cltId string, price, amt, qty decimal.Decimal,
		side, timeInForce, orderType string, postOnly bool) (string, error)
	// only bigone
	SpotPlaceOrderMultiple([]SpotPostOrder) error
	// orderId, cltId 二选一
	SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error
	// orderId, cltId 二选一 (kraken 不支持cltId)
	SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error)
	SpotGetOpenOrders(symbol string) ([]*SpotOrder, error)
	SpotGetFilledOrders(symbol string) ([]*SpotOrder, error)
	SpotGetTradeFee(symbol string) (SpotTradeFee, error)

	//= ws public
	// cex object 如果closed需要重新连接时，请不要复用，一定要创建新的obj
	SpotWsPublicOpen() error
	// channels: orderbook5@symbolA,symbolB (5档)
	//           bbo@symbolA,symbolB     // 最优买卖价 只binance,bybit,bbo,okx,gate实现
	//           ticker@symbolA,symbolB     // bigone不支持
	//           trades@symbolA,symbolB // 仅限bigone,binance
	// 每个交易所支持的参数数量不同
	SpotWsPublicSubscribe(channels []string)
	SpotWsPublicUnsubscribe(channels []string)
	SpotWsPublicTickerPoolPut(v any)
	SpotWsPublicOrderBook5PoolPut(v any)
	SpotWsPublicBBOPoolPut(v any)
	SpotWsPublicTradePoolPut(v any)
	// Loop结束时会close(ch)
	SpotWsPublicLoop(ch chan<- any)
	SpotWsPublicClose()
	SpotWsPublicIsClosed() bool

	//= ws private
	// cex object 如果closed需要重新连接时，请不要复用，一定要创建新的obj
	SpotWsPrivateSupported() bool
	SpotWsPrivateOpen() error
	// channels: orders
	//           balance
	SpotWsPrivateSubscribe(channels []string)
	// Loop结束时会close(ch)
	SpotWsPrivateLoop(ch chan<- any)
	// 返回参数1:上次pong的时间(0表示还没收到pong)，参数2:期望的pong时间(0表示还没开始ping), 参3:ping周期
	SpotWsPrivateLastPong() (int64, int64, int64)
	SpotWsPrivateClose()
	SpotWsPrivateIsClosed() bool
	// 市价 amt/qty任选1(优先amt) binance全支持, bigone只qty, gate,okx只amt
	// 限价 只能qty=base qty, 参数涵义参考 struct SpotOrder
	// postOnly = true 只做Maker(仅限OrderType=LIMIT) 只有bigone/okx支持
	SpotWsPlaceOrder(symbol, cltId string, price, amt, qty decimal.Decimal,
		side, timeInForce, orderType string, postOnly bool) (string /*req id*/, error)
	// orderId, cltId 二选一
	SpotWsCancelOrder(symbol, orderId, cltId string) (string, error)

	//= margin
	// 全仓杠杆账户详情
	MarginSupported() bool
	MarginGetCrossAccountInfo() (*MarginCrossAccountInfo, error)
	MarginGetMaxBorrowable(symbol /*BTC*/ string) (MarginMaxBorrowable, error)
	// 市价 amt/qty任选1(优先amt) binance全支持
	// 限价 只能qty=base qty, 参数涵义参考 struct MarginOrder
	// sideEffectType: NO_SIDE_EFFECT,MARGIN_BUY,AUTO_REPAY,AUTO_BORROW_REPAY
	MarginPlaceOrder(symbol, cltId string, price, amt, qty decimal.Decimal,
		side, timeInForce, orderType, sideEffectType string, isIsolated bool) (string, decimal.Decimal, string, error)
	// orderId, cltId 二选一
	MarginCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string, isIsolated bool) error
	// orderId, cltId 二选一
	MarginGetOrder(symbol, orderId, cltId string, isIsolated bool) (*MarginOrder, error)
	MarginGetTrades(symbol, orderId string, isIsolated bool) ([]*MarginTrade, error)
	// 全仓杠杆账户还款
	MarginRepay(symbol string, qty decimal.Decimal, isIsolated bool) error
	MarginGetAssetInfo(symbol /*BTC*/ string) (MarginAssetInfo, error)

	//= futures, typ=UM,U本位 typ=CM,币本位
	FuturesSupported(typ string) bool
	FuturesServerTime(typ string) (int64, error)
	FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error)
	FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal
	FuturesGetAll24hTicker(typ string) (map[string]Pub24hTicker, error)
	FuturesGetBBO(typ, symbol string) (BestBidAsk, error)
	FuturesGetAllFundingRate(typ string) (map[string]FundingRate, error)
	FuturesGetFundingRateHistory(typ, symbol string, startTime, endTime int64) ([]FundingRateHistory, error)
	// for binance
	FuturesGetFundingRateMarkPrice(typ, symbol string) (FundingRateMarkPrice, error)
	FuturesGetAllAssets(typ string) (map[string]*FuturesAsset, error)
	// interval 1m,5m,30m,1h,6h,12h,1d startTime/endTime is second
	// 返回顺序[11:15:00,11:16:00,11:17:00]
	FuturesGetKLine(typ, symbol, interval string, startTime, endTime, lmt int64) ([]KLine, error)
	FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error)
	FuturesGetAllPositions(typ string) (map[string]*FuturesPositions, error)
	FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal
	// CM中 qty为合约张数, positionMode=BOTH,LONG/SHORT
	FuturesPlaceOrder(typ, symbol, clientId string,
		price, qty decimal.Decimal, side, orderType, timeInForce, positionMode string,
		tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error)
	FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error)
	// symbol 为空取所有的
	FuturesGetOpenOrders(typ, symbol string) ([]*FuturesOrder, error)
	FuturesCancelOrder(typ string, symbol /*BTCUSDT*/, orderId, cltId string) error
	//  单仓:0/双仓:1 切换
	FuturesSwitchPositionMode(typ string, mode int) error
	//  全仓:0/逐仓:1 切换
	FuturesSwitchTradeMode(typ, symbol string /*BTCUSDT*/, mode, leverage int) error
	// 获取交易对的杠杆分层标准 For binance
	FuturesMaintMargin(typ, symbol string) ([]*FuturesLeverageBracket, error)
	// 获取账户损益资金流水 For binance, plType: FUNDING_FEE
	FuturesGetProfitLossHistory(typ, symbol, plType string, startTime, endTime int64) (
		[]FuturesProfitLossHistory, error)

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
	FuturesWsPrivateSupported(typ string) bool
	FuturesWsPrivateOpen(typ string) error
	// channels: orders
	//           positions
	//           balance // 只有binance
	FuturesWsPrivateSubscribe(channels []string)
	FuturesWsPrivateLoop(ch chan<- any)
	FuturesWsPrivateClose()
	FuturesWsPrivateIsClosed() bool

	// return reqId,err, CM中 qty为合约张数,
	FuturesWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
		side, orderType, timeInForce, positionMode string,
		tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error)
	FuturesWsCancelOrder(symbol, orderId, cltId string) (string, error)

	//= 统一账户
	// rest api
	UnifiedGetAssets() (map[string]*UnifiedAsset, error)

	// ws
	UnifiedWsSupported() bool
	UnifiedWsOpen() error
	// channels: balance@symbol1,symbol2,symbol3
	UnifiedWsSubscribe(channels []string)
	UnifiedWsLoop(ch chan<- any)
	UnifiedWsClose()
	UnifiedWsIsClosed() bool

	//= wallet
	// chain: TRX/MOB
	// memo is key at Kraken
	Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error)
	GetWithdrawalHistory(symbol string) ([]WithdrawResult, error)
	// from,to:FUNDING,SPOT,UM_FUTURE,CM_FUTURE,UNIFIED,MARGIN
	// typ: NORMAL, MASTER_TO_SUB, SUB_TO_MASTER, SUB_INTERNAL
	Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error
	// 资金账户获取资产
	FundingGetAllAssets() (map[string]*FundingAsset, error)
	FundingGetAsset(symbol string) (FundingAsset, error)
	// network is optional
	GetDepositAddress(symbol, network string) ([]DepositAddress, error)
	// only bigone
	GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error)
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
	CexList["bybit"] = "Bybit"
	CexList["kraken"] = "Kraken"
	CexList["ktx"] = "Ktx"
	CexList["kucoin"] = "Kucoin"
	//CexList["mexc"] = "Mexc"
	//CexList["bitget"]= "Bitget"

	CexSXList = make(map[string]string)
	CexSXList["binance"] = "BN"
	CexSXList["gate"] = "GT"
	CexSXList["okx"] = "OK"
	CexSXList["bybit"] = "BY"
	CexSXList["bigone"] = "BO"
	CexSXList["kraken"] = "KK"
	CexSXList["mexc"] = "MC"
	CexSXList["ktx"] = "KTX"
	CexSXList["kucoin"] = "KC"

	CexFeeCoinMap = make(map[string]string)
	CexFeeCoinMap["gate"] = "GT"
	CexFeeCoinMap["binance"] = "BNB"
}

func NewPublic(cexName string) (Exchanger, error) {
	return New(cexName, "", "", "", "", "")
}
func NewPrivate(cexName, account, apikey, secretkey, passwd string) (Exchanger, error) {
	return New(cexName, account, apikey, secretkey, passwd, "")
}
func NewPrivateWithLocalIP(cexName, account, apikey, secretkey, passwd, localIp string) (Exchanger, error) {
	return New(cexName, account, apikey, secretkey, passwd, localIp)
}
func New(cexName, account, apikey, secretkey, passwd, localIp string) (Exchanger, error) {
	var cexObj Exchanger
	var err error
	if cexName == "binance" {
		cexObj, err = NewBinance(account, apikey, secretkey, localIp)
	} else if cexName == "gate" {
		cexObj, err = NewGate(account, apikey, secretkey, localIp)
	} else if cexName == "okx" {
		cexObj = NewOkx(account, apikey, secretkey, passwd)
	} else if cexName == "bigone" {
		cexObj, err = NewBigone(account, apikey, secretkey, localIp)
	} else if cexName == "bybit" {
		cexObj = NewBybit(account, apikey, secretkey)
	} else if cexName == "ktx" {
		cexObj = NewKtx()
	} else if cexName == "kucoin" {
		cexObj = NewKucoin()
	} else if cexName == "kraken" {
		cexObj = NewKraken(account, apikey, secretkey)
	} else {
		return nil, errors.New("unknown cex platform : " + cexName)
	}
	if err != nil {
		return nil, errors.New(cexName + " create failed! " + err.Error())
	}
	if err = cexObj.Init(); err != nil {
		return nil, errors.New(cexObj.Name() + " init failed! " + err.Error())
	}
	return cexObj, nil
}
