package cex

import (
	"errors"
	"sync"

	"github.com/shopspring/decimal"
)

type Unsupported struct {
}

// = spot
func (us *Unsupported) SpotSupported() bool            { return false }
func (us *Unsupported) SpotServerTime() (int64, error) { return 0, errors.New("not support") }
func (us *Unsupported) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) SpotGetAll24hTicker() (map[string]Spot24hTicker, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) SpotPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {
	return "", errors.New("not support")
}
func (us *Unsupported) SpotCancelOrder(symbol, orderId, cltId string) error {
	return errors.New("not support")
}
func (us *Unsupported) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	return nil, errors.New("not support")
}

func (us *Unsupported) SpotWsPublicOpen() error                   { return errors.New("not support") }
func (us *Unsupported) SpotWsPublicSubscribe(channels []string)   {}
func (us *Unsupported) SpotWsPublicUnsubscribe(channels []string) {}
func (us *Unsupported) SpotWsPublicTickerPoolPut(v any)           {}
func (us *Unsupported) SpotWsPublicOrderBook5PoolPut(v any)       {}
func (us *Unsupported) SpotWsPublicLoop(ch chan<- any)            {}
func (us *Unsupported) SpotWsPublicClose()                        {}
func (us *Unsupported) SpotWsPublicIsClosed() bool                { return true }
func (us *Unsupported) SpotWsPrivateOpen() error                  { return errors.New("not support") }
func (us *Unsupported) SpotWsPrivateSubscribe(channels []string)  {}
func (us *Unsupported) SpotWsPrivateLoop(ch chan<- any)           {}
func (us *Unsupported) SpotWsPrivateClose()                       {}
func (us *Unsupported) SpotWsPrivateIsClosed() bool               { return true }
func (us *Unsupported) SpotWsPlaceOrder(symbol, cltId string, price, qty decimal.Decimal,
	side, timeInForce, orderType string) (string, error) {
	return "", errors.New("not support")
}
func (us *Unsupported) SpotWsCancelOrder(s, o, c string) error { return errors.New("not support") }

// = contract
func (us *Unsupported) FuturesSupported(typ string) bool { return false }
func (us *Unsupported) FuturesLoadAllPairRule(typ string) (map[string]*FuturesExchangePairRule, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesGetAll24hTicker(typ string) (map[string]Futures24hTicker, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesGetAllFundingRate(typ string) (map[string]FundingRate, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesGetKLine(typ, symbol, interval string, startTime, endTime, lmt int64) ([]KLine, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesGetAllPositionList(typ string) (map[string]*FuturesPosition, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesSizeToQty(typ, symbol string, size decimal.Decimal) decimal.Decimal {
	return decimal.Zero
}
func (us *Unsupported) FuturesQtyToSize(typ, symbol string, qty decimal.Decimal) decimal.Decimal {
	return decimal.Zero
}
func (us *Unsupported) FuturesPlaceOrder(typ, symbol, clientId string,
	price, qty decimal.Decimal, side, orderType, timeInForce string,
	positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) (string, error) {
	return "", errors.New("not support")
}
func (us *Unsupported) FuturesCancelOrder(typ, symbol, orderId, cltId string) error {
	return errors.New("not support")
}
func (us *Unsupported) FuturesGetOrder(typ, symbol, orderId, cltId string) (*FuturesOrder, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) FuturesSwitchPositionMode(typ string, mode int) error {
	return errors.New("not support")
}
func (us *Unsupported) FuturesSwitchTradeMode(typ, symbol string, mode, lver int) error {
	return errors.New("not support")
}
func (us *Unsupported) WsContractPubChannelOpen() error                   { return errors.New("not support") }
func (us *Unsupported) WsContractPubChannelSubscribe(channels []string)   {}
func (us *Unsupported) WsContractPubChannelUnsubscribe(channels []string) {}
func (us *Unsupported) WsContractPubChannelSetTickerPool(p *sync.Pool)    {}
func (us *Unsupported) WsContractPubChannelSetOrderBookPool(p *sync.Pool) {}
func (us *Unsupported) WsContractPubChannelLoop(ch chan<- any)            {}
func (us *Unsupported) WsContractPubChannelClose()                        {}
func (us *Unsupported) WsContractPubChannelIsClosed() bool                { return true }
func (us *Unsupported) WsContractPrivChannelOpen() error                  { return errors.New("not support") }
func (us *Unsupported) WsContractPrivChannelSubscribe(channels []string)  {}
func (us *Unsupported) WsContractPrivChannelLoop(ch chan<- any)           {}
func (us *Unsupported) WsContractPrivChannelClose()                       {}
func (us *Unsupported) WsContractPrivChannelIsClosed() bool               { return true }
func (us *Unsupported) WsContractPlaceOrder(category, symbol, clientId, reqId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, orderType, timeInForce string,
	positionMode /*0单仓,1双仓*/, tradeMode /*全仓:0/逐仓:1*/, reduceOnly int) error {
	return errors.New("not support")
}
func (us *Unsupported) WsContractCancelOrder(c, sm, od, rq string) error {
	return errors.New("not support")
}

// unified
func (us *Unsupported) UnifiedGetAssets() (map[string]*UnifiedAsset, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) WsUnifiedChannelOpen() error                 { return errors.New("not support") }
func (us *Unsupported) WsUnifiedChannelSubscribe(channels []string) {}
func (us *Unsupported) WsUnifiedChannelLoop(ch chan<- any)          {}
func (us *Unsupported) WsUnifiedChannelClose()                      {}
func (us *Unsupported) WsUnifiedChannelIsClosed() bool              { return true }

// wallet
func (us *Unsupported) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	return nil, errors.New("not support")
}
func (us *Unsupported) CancelWithdrawal(wid string) error { return errors.New("not support") }
