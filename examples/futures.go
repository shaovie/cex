package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shaovie/cex"
	"github.com/shaovie/gutils/gutils"
	"github.com/shopspring/decimal"
)

// 合约接口自测: 先rest api, 再wss; 不想测的直接注释掉
//
// CEX=binance TYP=UM go run examples/futures.go                       // 公共接口
// CEX=binance TYP=UM APIKEY=x SECRETKEY=y go run examples/futures.go  // 私有接口(会小额下单, 测试完自动撤单)
//
// 环境变量: CEX 交易所, TYP UM/U本位 CM/币本位(默认UM), APIKEY/SECRETKEY/PASSPHRASE 私有接口
// 结果: 成功一行"OK ...", 失败一行"Error ..."

const (
	symbol = "BTCUSDT"
	wsWait = 15 * time.Second // wss的观察时长
)

func main() {
	cexName, typ := os.Getenv("CEX"), os.Getenv("TYP")
	if typ == "" {
		typ = "UM"
	}
	cexObj, err := cex.NewPrivate(cexName, "", os.Getenv("APIKEY"),
		os.Getenv("SECRETKEY"), os.Getenv("PASSPHRASE"))
	if err != nil || cexObj == nil {
		fmt.Println("Error NewPrivate", cexName, err)
		return
	}
	cexObj.Debug(true) // 打印wss原始报文

	// 下单参数(私有接口用): 价格用买1下方1%(不会立即成交), 数量取满足最小下单量的最小值
	price, qty := decimal.NewFromFloat(60000), decimal.NewFromFloat(0.001)
	if b, err := cexObj.FuturesGetBBO(typ, symbol); err == nil && b.BidPrice.IsPositive() {
		price = b.BidPrice.Mul(decimal.NewFromFloat(0.99))
	}
	if all, err := cexObj.FuturesLoadAllPairRule(typ); err == nil {
		if rule := all[symbol]; rule != nil {
			price = rule.AdjustPrice(price)
			if rule.MinNotional.IsPositive() && rule.QtyStep.IsPositive() {
				qty = rule.MinNotional.Div(price).Div(rule.QtyStep).Ceil().Mul(rule.QtyStep)
			}
			qty = rule.AdjustQty(price, qty)
		}
	}

	fmt.Println("=========== rest api 公共 ===========")
	serverTime, err := cexObj.FuturesServerTime(typ)
	if err != nil {
		fmt.Println("Error FuturesServerTime:", err)
	} else {
		fmt.Println("OK FuturesServerTime:", serverTime, "diff", time.Now().UnixMilli()-serverTime)
	}
	allTicker, err := cexObj.FuturesGetAll24hTicker(typ)
	if err != nil {
		fmt.Println("Error FuturesGetAll24hTicker:", err)
	} else {
		fmt.Println("OK FuturesGetAll24hTicker:", len(allTicker), allTicker[symbol].LastPrice)
	}
	allRule, err := cexObj.FuturesLoadAllPairRule(typ)
	if err != nil {
		fmt.Println("Error FuturesLoadAllPairRule:", err)
	} else {
		fmt.Println("OK FuturesLoadAllPairRule:", len(allRule), allRule[symbol])
	}
	bbo, err := cexObj.FuturesGetBBO(typ, symbol)
	if err != nil {
		fmt.Println("Error FuturesGetBBO:", err)
	} else {
		fmt.Println("OK FuturesGetBBO:", bbo.BidPrice, bbo.BidQty, bbo.AskPrice, bbo.AskQty)
	}
	obd, err := cexObj.FuturesGetOrderBook(typ, symbol, 5)
	if err != nil {
		fmt.Println("Error FuturesGetOrderBook:", err)
	} else if len(obd.Bids) == 0 || len(obd.Asks) == 0 {
		fmt.Println("Error FuturesGetOrderBook: empty orderbook")
	} else {
		fmt.Println("OK FuturesGetOrderBook:", obd.Bids[0], obd.Asks[0])
	}
	allRate, err := cexObj.FuturesGetAllFundingRate(typ)
	if err != nil {
		fmt.Println("Error FuturesGetAllFundingRate:", err)
	} else {
		fmt.Println("OK FuturesGetAllFundingRate:", len(allRate), allRate[symbol].Val)
	}
	now := time.Now().UnixMilli()
	rateHistory, err := cexObj.FuturesGetFundingRateHistory(typ, symbol, now/1000-86400, now/1000)
	if err != nil {
		fmt.Println("Error FuturesGetFundingRateHistory:", err)
	} else {
		fmt.Println("OK FuturesGetFundingRateHistory:", len(rateHistory))
	}
	markPrice, err := cexObj.FuturesGetFundingRateMarkPrice(typ, symbol)
	if err != nil {
		fmt.Println("Error FuturesGetFundingRateMarkPrice:", err)
	} else {
		fmt.Println("OK FuturesGetFundingRateMarkPrice:", markPrice.MarkPrice, markPrice.FundingRate)
	}
	klineL, err := cexObj.FuturesGetKLine(typ, symbol, "1m", now/1000-3600, 0, 5)
	if err != nil {
		fmt.Println("Error FuturesGetKLine:", err)
	} else if len(klineL) == 0 {
		fmt.Println("Error FuturesGetKLine: empty kline")
	} else {
		fmt.Println("OK FuturesGetKLine:", len(klineL), klineL[len(klineL)-1].ClosePrice)
	}

	fmt.Println("=========== rest api 私有 ===========")
	allAsset, err := cexObj.FuturesGetAllAssets(typ)
	if err != nil {
		fmt.Println("Error FuturesGetAllAssets:", err)
	} else {
		fmt.Println("OK FuturesGetAllAssets:", len(allAsset))
	}
	posM, err := cexObj.FuturesGetAllPositionList(typ)
	if err != nil {
		fmt.Println("Error FuturesGetAllPositionList:", err)
	} else {
		fmt.Println("OK FuturesGetAllPositionList:", len(posM))
	}
	posL, err := cexObj.FuturesGetAllPositions(typ)
	if err != nil {
		fmt.Println("Error FuturesGetAllPositions:", err)
	} else {
		fmt.Println("OK FuturesGetAllPositions:", len(posL))
	}
	lbL, err := cexObj.FuturesMaintMargin(typ, symbol)
	if err != nil {
		fmt.Println("Error FuturesMaintMargin:", err)
	} else if len(lbL) == 0 {
		fmt.Println("Error FuturesMaintMargin: empty leverage bracket")
	} else {
		fmt.Println("OK FuturesMaintMargin:", len(lbL), lbL[0])
	}
	plL, err := cexObj.FuturesGetProfitLossHistory(typ, symbol, "FUNDING_FEE", now-86400000, now)
	if err != nil {
		fmt.Println("Error FuturesGetProfitLossHistory:", err)
	} else {
		fmt.Println("OK FuturesGetProfitLossHistory:", len(plL))
	}
	cltId := gutils.RandomStr(24)
	orderId, err := cexObj.FuturesPlaceOrder(typ, symbol, cltId, price, qty,
		"BUY", "LIMIT", "GTC", "BOTH", 0, 0)
	if err != nil {
		fmt.Println("Error FuturesPlaceOrder:", err)
	} else {
		fmt.Println("OK FuturesPlaceOrder:", orderId, price, qty)
	}
	if orderId != "" {
		order, err := cexObj.FuturesGetOrder(typ, symbol, orderId, "")
		if err != nil {
			fmt.Println("Error FuturesGetOrder:", err)
		} else {
			fmt.Println("OK FuturesGetOrder:", order)
		}
		orderL, err := cexObj.FuturesGetOpenOrders(typ, symbol)
		if err != nil {
			fmt.Println("Error FuturesGetOpenOrders:", err)
		} else {
			fmt.Println("OK FuturesGetOpenOrders:", len(orderL))
		}
		if err = cexObj.FuturesCancelOrder(typ, symbol, orderId, ""); err != nil {
			fmt.Println("Error FuturesCancelOrder:", err)
		} else {
			fmt.Println("OK FuturesCancelOrder:", orderId)
		}
	}
	// 会改账户设置, 需要时打开
	//if err = cexObj.FuturesSwitchPositionMode(typ, 0); err != nil {
	//	fmt.Println("Error FuturesSwitchPositionMode:", err)
	//} else {
	//	fmt.Println("OK FuturesSwitchPositionMode: one-way")
	//}
	//if err = cexObj.FuturesSwitchTradeMode(typ, symbol, 0, 2); err != nil {
	//	fmt.Println("Error FuturesSwitchTradeMode:", err)
	//} else {
	//	fmt.Println("OK FuturesSwitchTradeMode: cross, leverage=2")
	//}
	// 会划转资金, 需要时打开
	//if err = cexObj.Transfer("USDT", "FUTURE", "SPOT", "NORMAL", "", decimal.NewFromInt(1)); err != nil {
	//	fmt.Println("Error Transfer:", err)
	//} else {
	//	fmt.Println("OK Transfer: future->spot")
	//}

	fmt.Println("=========== wss 公共 ===========")
	err = cexObj.FuturesWsPublicOpen(typ)
	if err != nil {
		fmt.Println("Error FuturesWsPublicOpen:", err)
	} else {
		fmt.Println("OK FuturesWsPublicOpen")
		ch := make(chan any, 256)
		go cexObj.FuturesWsPublicLoop(ch)
		cexObj.FuturesWsPublicSubscribe([]string{"bbo@" + symbol, "orderbook5@" + symbol,
			"ticker@" + symbol})
		bboN, obN, tickerN := 0, 0, 0
		timeout := time.After(wsWait)
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					fmt.Println("Error FuturesWsPublicLoop: chan closed")
					goto PUBLIC_END
				}
				switch val := v.(type) {
				case *cex.BestBidAsk:
					bboN++
					if bboN < 3 {
						fmt.Println("OK FuturesWsPublicLoop bbo:", val.Symbol, val.BidPrice, val.AskPrice)
					}
					cexObj.FuturesWsPublicBBOPoolPut(val)
				case *cex.OrderBookDepth:
					obN++
					if obN < 3 {
						fmt.Println("OK FuturesWsPublicLoop orderbook5:", val.Symbol, val.Bids[0], val.Asks[0])
					}
					cexObj.FuturesWsPublicOrderBook5PoolPut(val)
				case *cex.Pub24hTicker:
					tickerN++
					if tickerN < 3 {
						fmt.Println("OK FuturesWsPublicLoop ticker:", val.Symbol, val.LastPrice, val.Volume)
					}
					cexObj.FuturesWsPublicTickerPoolPut(val)
				}
			case <-timeout:
				cexObj.FuturesWsPublicUnsubscribe([]string{"bbo@" + symbol, "ticker@" + symbol})
				goto PUBLIC_END
			}
		}
	PUBLIC_END:
		fmt.Println("OK FuturesWsPublicLoop: bbo", bboN, "orderbook5", obN, "ticker", tickerN)
		cexObj.FuturesWsPublicClose()
	}

	fmt.Println("=========== wss 私有 ===========")
	err = cexObj.FuturesWsPrivateOpen(typ)
	if err != nil {
		fmt.Println("Error FuturesWsPrivateOpen:", err)
	} else {
		fmt.Println("OK FuturesWsPrivateOpen")
		ch := make(chan any, 256)
		go cexObj.FuturesWsPrivateLoop(ch)
		cexObj.FuturesWsPrivateSubscribe([]string{"orders", "positions", "balance"})
		// wss api下单, ack里的orderId用来撤单; 只有binance/gate/okx实现了
		wsOrderId := ""
		wsReqId, err := cexObj.FuturesWsPlaceOrder(symbol, gutils.RandomStr(24), price, qty,
			"BUY", "LIMIT", "GTC", "BOTH", 0, 0)
		if err != nil {
			fmt.Println("Error FuturesWsPlaceOrder:", err)
		} else {
			fmt.Println("OK FuturesWsPlaceOrder:", wsReqId)
		}
		// rest下单, 让私有wss有订单推送
		if orderId, err := cexObj.FuturesPlaceOrder(typ, symbol, gutils.RandomStr(24), price, qty,
			"BUY", "LIMIT", "GTC", "BOTH", 0, 0); err != nil {
			fmt.Println("Error FuturesPlaceOrder(private ws):", err)
		} else {
			fmt.Println("OK FuturesPlaceOrder(private ws):", orderId)
		}
		orderN, posN, assetN := 0, 0, 0
		timeout := time.After(wsWait)
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					fmt.Println("Error FuturesWsPrivateLoop: chan closed")
					goto PRIV_END
				}
				switch val := v.(type) {
				case *cex.FuturesOrder:
					orderN++
					if orderN < 3 {
						fmt.Println("OK FuturesWsPrivateLoop order:", val)
					}
					if val.RequestId != "" && val.OrderId != "" && wsOrderId == "" {
						wsOrderId = val.OrderId
						if _, err = cexObj.FuturesWsCancelOrder(symbol, wsOrderId, ""); err != nil {
							fmt.Println("Error FuturesWsCancelOrder:", err)
						} else {
							fmt.Println("OK FuturesWsCancelOrder:", wsOrderId)
						}
					}
				case *cex.FuturesPosition:
					posN++
					if posN < 3 {
						fmt.Println("OK FuturesWsPrivateLoop position:", val)
					}
				case *cex.FuturesAsset:
					assetN++
					if assetN < 3 {
						fmt.Println("OK FuturesWsPrivateLoop asset:", val)
					}
				}
			case <-timeout:
				goto PRIV_END
			}
		}
	PRIV_END:
		fmt.Println("OK FuturesWsPrivateLoop: orders", orderN, "positions", posN, "balance", assetN)
		// 兜底撤单
		if orderL, err := cexObj.FuturesGetOpenOrders(typ, symbol); err == nil {
			for _, o := range orderL {
				cexObj.FuturesCancelOrder(typ, symbol, o.OrderId, "")
			}
		}
		cexObj.FuturesWsPrivateClose()
	}
}
