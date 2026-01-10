package cex

import (
	"sync"

	"github.com/shopspring/decimal"
)

var (
	wsPublicTickerPool     *sync.Pool
	wsPublicOrderBook5Pool *sync.Pool
	wsPublicBBOPool        *sync.Pool
)

func init() {
	wsPublicTickerPool = &sync.Pool{
		New: func() any { return &Pub24hTicker{} },
	}
	wsPublicOrderBook5Pool = &sync.Pool{
		New: func() any {
			return &OrderBookDepth{
				Bids: make([]Ticker, 0, 5),
				Asks: make([]Ticker, 0, 5),
			}
		},
	}
	wsPublicBBOPool = &sync.Pool{
		New: func() any {
			return &BestBidAsk{}
		},
	}
}

type SpotExchangePairRule struct {
	Symbol        string          // BTCUSDT
	Base          string          // BTC
	Quote         string          // USDT
	MinPrice      decimal.Decimal // MUST
	MaxPrice      decimal.Decimal // MUST
	PriceTickSize decimal.Decimal // MUST 价格最小变动单位
	MinOrderQty   decimal.Decimal // MUST
	MaxOrderQty   decimal.Decimal // MUST
	QtyStep       decimal.Decimal // MUST 数量最小变动单位
	MinNotional   decimal.Decimal // MUST
	BaseScale     int32           // for bigone
	Time          int64           // 取到数据的时间 second
}

func (exp *SpotExchangePairRule) AdjustPrice(v decimal.Decimal) decimal.Decimal {
	if v.LessThan(exp.MinPrice) || v.GreaterThan(exp.MaxPrice) {
		return decimal.Decimal{}
	}
	return v.Div(exp.PriceTickSize).Floor().Mul(exp.PriceTickSize)
}
func (exp *SpotExchangePairRule) AdjustQty(price, v decimal.Decimal) decimal.Decimal {
	if v.LessThan(exp.MinOrderQty) || v.GreaterThan(exp.MaxOrderQty) {
		return decimal.Decimal{}
	}
	ret := v.Div(exp.QtyStep).Floor().Mul(exp.QtyStep)
	if price.IsPositive() && exp.MinNotional.IsPositive() &&
		price.Mul(ret).LessThan(exp.MinNotional) {
		return decimal.Zero
	}
	return ret
}

type FuturesExchangePairRule struct {
	Typ                string // UM / CM
	Symbol             string // BTCUSDT
	Base               string
	Quote              string
	ContractSize       decimal.Decimal // 合约面值 for CM
	ContractMultiplier decimal.Decimal // 合约乘数，每个合约代表多少单位的标的资产
	MinPrice           decimal.Decimal
	MaxPrice           decimal.Decimal
	PriceTickSize      decimal.Decimal // 价格最小变动单位
	MinOrderQty        decimal.Decimal // 标的资产数量，不是张数
	MaxOrderQty        decimal.Decimal // 标的资产数量，不是张数
	QtyStep            decimal.Decimal // 合约数量最小单位(不是张数)
	MinNotional        decimal.Decimal // MUST for UM
	Time               int64           // 取到数据的时间 second
}

func (exp *FuturesExchangePairRule) AdjustPrice(v decimal.Decimal) decimal.Decimal {
	if v.LessThan(exp.MinPrice) || v.GreaterThan(exp.MaxPrice) {
		return decimal.Zero
	}
	return v.Div(exp.PriceTickSize).Floor().Mul(exp.PriceTickSize)
}
func (exp *FuturesExchangePairRule) AdjustQty(price, v decimal.Decimal) decimal.Decimal {
	if exp.Typ == "CM" { // 币本位是张数
		return v
	}
	if v.LessThan(exp.MinOrderQty) || v.GreaterThan(exp.MaxOrderQty) {
		return decimal.Zero
	}
	ret := v.Div(exp.QtyStep).Floor().Mul(exp.QtyStep)
	if price.IsPositive() && exp.MinNotional.IsPositive() &&
		price.Mul(ret).LessThan(exp.MinNotional) {
		return decimal.Zero
	}
	return ret
}

type SpotAsset struct {
	Symbol string          // BTC
	Total  decimal.Decimal // 总共
	Avail  decimal.Decimal // 可用
	Locked decimal.Decimal
}

func (sa *SpotAsset) Val(v *SpotAsset) {
	if v.Symbol != "" {
		sa.Symbol = v.Symbol
	}
	sa.Avail = v.Avail
	if !v.Total.Equals(decimal.NewFromInt(999999999)) {
		sa.Total = v.Total
	}
	if !v.Locked.Equals(decimal.NewFromInt(999999999)) {
		sa.Locked = v.Locked
	}
}

type FuturesAsset struct {
	Symbol            string          // BTC
	Total             decimal.Decimal // 账户余额/钱包余额
	Avail             decimal.Decimal // 可用下单余额
	MaxWithdrawAmount decimal.Decimal // for binance's rest api
}

type UnifiedAsset struct {
	Symbol string          // BTC
	Total  decimal.Decimal // 总共
	Avail  decimal.Decimal // 可用
	Locked decimal.Decimal
}

func (sa *UnifiedAsset) Val(v *UnifiedAsset) {
	if v.Symbol != "" {
		sa.Symbol = v.Symbol
	}
	if !v.Avail.Equals(decimal.NewFromInt(999999999)) {
		sa.Avail = v.Avail
	}
	if !v.Total.Equals(decimal.NewFromInt(999999999)) {
		sa.Total = v.Total
	}
	if !v.Locked.Equals(decimal.NewFromInt(999999999)) {
		sa.Locked = v.Locked
	}
}

type SpotOrder struct {
	RequestId string // for ws 不要超过28, 在SpotWsPlaceOrder成功后返回
	Err       string // for ws
	// 如果ws api下单失败，会只返回以上部分

	Symbol   string // BTCUSDT
	OrderId  string
	ClientId string
	// 如果ws api下单成功，会只返回以上部分

	Price decimal.Decimal
	Qty   decimal.Decimal // 用户设置的原始订单数量

	FilledQty decimal.Decimal // 交易的订单数量
	FilledAmt decimal.Decimal // 累计交易的金额

	Status      string          // NEW/PARTIALLY_FILLED/FILLED/CANCELED/REJECTED/EXPIRED
	Type        string          // LIMIT/MARKET
	TimeInForce string          // GTC/FOK/IOC
	Side        string          // BUY/SELL
	FeeAsset    string          // 交易费资产类型, 比如GT,BNB,USDT
	FeeQty      decimal.Decimal // 手续费数量(消耗为负值)
	CTime       int64           // msec
	UTime       int64           // msec
}

type Ticker struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
}
type OrderBookDepth struct {
	Symbol string // BTCUSDT
	Level  int    // 保存传入的参数，必须
	Time   int64  // msec  0  表示交易所不提供
	Bids   []Ticker
	Asks   []Ticker
}
type BestBidAsk struct {
	Symbol   string // BTCUSDT
	Time     int64  // msec  0  表示交易所不提供
	BidPrice decimal.Decimal
	BidQty   decimal.Decimal
	AskPrice decimal.Decimal
	AskQty   decimal.Decimal
}
type Pub24hTicker struct {
	Cex         string          // for internel
	Symbol      string          // BTCUSDT
	LastPrice   decimal.Decimal // 最近成交价
	Volume      decimal.Decimal
	BaseVolume  decimal.Decimal // futures-CM 24小时成交额(标的数量)
	QuoteVolume decimal.Decimal // futures-UM 24小时成交额
}

type FuturesOrder struct {
	RequestId string // for ws
	Err       string // for ws

	OrderId   string
	ClientId  string
	Symbol    string          // BTCUSDT
	Side      string          // BUY/SELL
	Type      string          // LIMIT/MARKET
	Price     decimal.Decimal // 下单价
	Qty       decimal.Decimal // 下单数量 (不是size)
	Status    string          // NEW/PARTIALLY_FILLED/FILLED/CANCELED/REJECTED
	FilledQty decimal.Decimal // 累计成交数量
	FilledAmt decimal.Decimal // 累计交易的金额  在CM中为标的数量
	AvgPrice  decimal.Decimal // 仅在CM中有效

	FeeAsset string          // 交易费资产类型
	FeeQty   decimal.Decimal // 手续费金额 一般是指usdt金额
	CTime    int64
	UTime    int64
}
type FuturesPosition struct {
	Symbol      string // BTCUSDT
	Side        string
	PositionQty decimal.Decimal // 持仓数量  正数, 在CM中为张数
	EntryPrice  decimal.Decimal // 开仓均价
	// 以上必须
	LiqPrice         decimal.Decimal // 强平价格
	UnRealizedProfit decimal.Decimal // 未结盈亏
	Leverage         decimal.Decimal // 杠杆  币安推送不带个值
	UTime            int64           // mill second
}

func (cp *FuturesPosition) Val(v *FuturesPosition) {
	if v.Symbol != "" {
		cp.Symbol = v.Symbol
	}
	if v.Side != "" {
		cp.Side = v.Side
	}
	cp.PositionQty = v.PositionQty
	cp.EntryPrice = v.EntryPrice
	cp.UnRealizedProfit = v.UnRealizedProfit
	if !v.LiqPrice.IsNegative() {
		cp.LiqPrice = v.LiqPrice
	}
	if v.Leverage.IsPositive() {
		cp.Leverage = v.Leverage
	}
	if v.UTime > 0 {
		cp.UTime = v.UTime
	}
}

type FundingRate struct {
	Symbol   string          // BTCUSDT
	Val      decimal.Decimal // 下次资金费率
	NextTime int64           // 下次结算时间second
	UTime    int64           // second
}
type FundingRateHistory struct {
	FundingRate decimal.Decimal // 结算资金费率
	MarkPrice   decimal.Decimal // 资金费对应标记价格
	Time        int64           // 结算时间second
}
type FundingRateMarkPrice struct {
	MarkPrice   decimal.Decimal
	FundingRate decimal.Decimal // 下次资金费率
	NextTime    int64           // 下次结算时间 msec
}
type KLine struct {
	OpenTime    int64 // sec
	OpenPrice   decimal.Decimal
	HighPrice   decimal.Decimal
	ClosePrice  decimal.Decimal
	LowPrice    decimal.Decimal
	Volume      decimal.Decimal // 成交量
	QuoteVolume decimal.Decimal // 成交金额
}

type WithdrawReturn struct {
	WId    string          // USDT
	Symbol string          // USDT
	Qty    decimal.Decimal //
	Status string          // PENDING/DONE/FAILED/CANCELLED
	Txid   string
	Addr   string
	Chain  string
	Memo   string
}
type FuturesLeverageBracket struct {
	Bracket          int64           // 层级
	InitialLeverage  int64           // 该层允许的最高初始杠杆倍数
	NotionalCap      decimal.Decimal // 该层对应的名义价值上限  For UM
	NotionalFloor    decimal.Decimal // 该层对应的名义价值下限  For UM
	QtyCap           decimal.Decimal // 该层对应的数量上限 For CM
	QtyFloor         decimal.Decimal // 该层对应的数量下限  For CM
	MaintMarginRatio decimal.Decimal // 该层对应的维持保证金率
	Cum              decimal.Decimal // 速算数
}
type FuturesProfitLossHistory struct {
	Income decimal.Decimal
	Asset  string //
	Typ    string // FUNDING_FEE
	Time   int64  // msec
}
