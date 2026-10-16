package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shaovie/cex"
	"github.com/shaovie/gutils/gutils"
	"github.com/shopspring/decimal"
)

// 现货接口自测: 先rest api, 再wss; 不想测的直接注释掉
//
// CEX=binance go run examples/spot.go                          // 公共接口
// CEX=binance APIKEY=x SECRETKEY=y go run examples/spot.go     // 私有接口(会小额下单, 测试完自动撤单)
//
// 环境变量: CEX 交易所, APIKEY/SECRETKEY/PASSPHRASE 私有接口
// 结果: 成功一行"OK ...", 失败一行"Error ..."

const (
	symbol = "BTCUSDT"
	wsWait = 15 * time.Second // wss的观察时长
)

func main() {
	cexName := os.Getenv("CEX")
	cexObj, err := cex.NewPrivate(cexName, "", os.Getenv("APIKEY"),
		os.Getenv("SECRETKEY"), os.Getenv("PASSPHRASE"))
	if err != nil || cexObj == nil {
		fmt.Println("Error NewPrivate", cexName, err)
		return
	}
	cexObj.Debug(true) // 打印wss原始报文

	// 下单参数(私有接口用): 价格用买1下方1%(不会立即成交), 数量取满足最小下单量的最小值
	price, qty := decimal.NewFromFloat(60000), decimal.NewFromFloat(0.0003)
	if b, err := cexObj.SpotGetBBO(symbol); err == nil && b.BidPrice.IsPositive() {
		price = b.BidPrice.Mul(decimal.NewFromFloat(0.99))
	}
	if all, err := cexObj.SpotLoadAllPairRule(); err == nil {
		if rule := all[symbol]; rule != nil {
			price = rule.AdjustPrice(price)
			if rule.MinNotional.IsPositive() && rule.QtyStep.IsPositive() {
				qty = rule.MinNotional.Div(price).Div(rule.QtyStep).Ceil().Mul(rule.QtyStep)
			}
			qty = rule.AdjustQty(price, qty)
		}
	}

	fmt.Println("=========== rest api 公共 ===========")
	serverTime, err := cexObj.SpotServerTime()
	if err != nil {
		fmt.Println("Error SpotServerTime:", err)
	} else {
		fmt.Println("OK SpotServerTime:", serverTime, "diff", time.Now().UnixMilli()-serverTime)
	}
	allTicker, err := cexObj.SpotGetAll24hTicker()
	if err != nil {
		fmt.Println("Error SpotGetAll24hTicker:", err)
	} else {
		fmt.Println("OK SpotGetAll24hTicker:", len(allTicker), allTicker[symbol].LastPrice)
	}
	allRule, err := cexObj.SpotLoadAllPairRule()
	if err != nil {
		fmt.Println("Error SpotLoadAllPairRule:", err)
	} else {
		fmt.Println("OK SpotLoadAllPairRule:", len(allRule), allRule[symbol])
	}
	bbo, err := cexObj.SpotGetBBO(symbol)
	if err != nil {
		fmt.Println("Error SpotGetBBO:", err)
	} else {
		fmt.Println("OK SpotGetBBO:", bbo.BidPrice, bbo.BidQty, bbo.AskPrice, bbo.AskQty)
	}
	obd, err := cexObj.SpotGetOrderBook(symbol, 5)
	if err != nil {
		fmt.Println("Error SpotGetOrderBook:", err)
	} else if len(obd.Bids) == 0 || len(obd.Asks) == 0 {
		fmt.Println("Error SpotGetOrderBook: empty orderbook")
	} else {
		fmt.Println("OK SpotGetOrderBook:", obd.Bids[0], obd.Asks[0])
	}

	fmt.Println("=========== rest api 私有 ===========")
	allAsset, err := cexObj.SpotGetAllAssets()
	if err != nil {
		fmt.Println("Error SpotGetAllAssets:", err)
	} else {
		fmt.Println("OK SpotGetAllAssets:", len(allAsset))
	}
	fee, err := cexObj.SpotGetTradeFee(symbol)
	if err != nil {
		fmt.Println("Error SpotGetTradeFee:", err)
	} else {
		fmt.Println("OK SpotGetTradeFee:", fee)
	}
	cltId := gutils.RandomStr(18)
	orderId, err := cexObj.SpotPlaceOrder(symbol, cltId, price, decimal.Zero, qty,
		"BUY", "GTC", "LIMIT", false)
	if err != nil {
		fmt.Println("Error SpotPlaceOrder:", err)
	} else {
		fmt.Println("OK SpotPlaceOrder:", orderId, price, qty)
	}
	if orderId != "" {
		order, err := cexObj.SpotGetOrder(symbol, orderId, "")
		if err != nil {
			fmt.Println("Error SpotGetOrder:", err)
		} else {
			fmt.Println("OK SpotGetOrder:", order)
		}
		orderL, err := cexObj.SpotGetOpenOrders(symbol)
		if err != nil {
			fmt.Println("Error SpotGetOpenOrders:", err)
		} else {
			fmt.Println("OK SpotGetOpenOrders:", len(orderL))
		}
		if err = cexObj.SpotCancelOrder(symbol, orderId, ""); err != nil {
			fmt.Println("Error SpotCancelOrder:", err)
		} else {
			fmt.Println("OK SpotCancelOrder:", orderId)
		}
	}
	orderL, err := cexObj.SpotGetFilledOrders(symbol)
	if err != nil {
		fmt.Println("Error SpotGetFilledOrders:", err)
	} else {
		fmt.Println("OK SpotGetFilledOrders:", len(orderL))
	}
	allFunding, err := cexObj.FundingGetAllAssets()
	if err != nil {
		fmt.Println("Error FundingGetAllAssets:", err)
	} else {
		fmt.Println("OK FundingGetAllAssets:", len(allFunding))
	}
	depositL, err := cexObj.GetDepositAddress("USDT", "")
	if err != nil {
		fmt.Println("Error GetDepositAddress:", err)
	} else {
		fmt.Println("OK GetDepositAddress:", depositL)
	}
	withdrawalL, err := cexObj.GetWithdrawalAddress("USDT")
	if err != nil {
		fmt.Println("Error GetWithdrawalAddress:", err)
	} else {
		fmt.Println("OK GetWithdrawalAddress:", withdrawalL)
	}
	walletM, err := cexObj.GetWalletAllAssetInfo()
	if err != nil {
		fmt.Println("Error GetWalletAllAssetInfo:", err)
	} else {
		fmt.Println("OK GetWalletAllAssetInfo:", len(walletM))
	}
	// 下面几个会动资金, 需要时打开
	//if err = cexObj.Transfer("USDT", "SPOT", "FUTURE", "NORMAL", "", decimal.NewFromInt(1)); err != nil {
	//	fmt.Println("Error Transfer:", err)
	//} else {
	//	fmt.Println("OK Transfer: spot->future")
	//}
	//if wh, err := cexObj.GetWithdrawalHistory("USDT"); err != nil {
	//	fmt.Println("Error GetWithdrawalHistory:", err)
	//} else {
	//	fmt.Println("OK GetWithdrawalHistory:", len(wh))
	//}
	//if _, err = cexObj.Withdrawal("USDT", "0x0000000000000000000000000000000000000000", "", "bsc-tokens", decimal.NewFromInt(1)); err != nil {
	//	fmt.Println("Error Withdrawal:", err)
	//} else {
	//	fmt.Println("OK Withdrawal")
	//}

	fmt.Println("=========== wss 公共 ===========")
	err = cexObj.SpotWsPublicOpen()
	if err != nil {
		fmt.Println("Error SpotWsPublicOpen:", err)
	} else {
		fmt.Println("OK SpotWsPublicOpen")
		ch := make(chan any, 256)
		go cexObj.SpotWsPublicLoop(ch)
		cexObj.SpotWsPublicSubscribe([]string{"bbo@" + symbol, "orderbook5@" + symbol,
			"ticker@" + symbol, "trades@" + symbol})
		bboN, obN, tickerN, tradeN := 0, 0, 0, 0
		timeout := time.After(wsWait)
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					fmt.Println("Error SpotWsPublicLoop: chan closed")
					goto PUBLIC_END
				}
				switch val := v.(type) {
				case *cex.BestBidAsk:
					bboN++
					if bboN < 3 {
						fmt.Println("OK SpotWsPublicLoop bbo:", val.Symbol, val.BidPrice, val.AskPrice)
					}
					cexObj.SpotWsPublicBBOPoolPut(val)
				case *cex.OrderBookDepth:
					obN++
					if obN < 3 {
						fmt.Println("OK SpotWsPublicLoop orderbook5:", val.Symbol, val.Bids[0], val.Asks[0])
					}
					cexObj.SpotWsPublicOrderBook5PoolPut(val)
				case *cex.Pub24hTicker:
					tickerN++
					if tickerN < 3 {
						fmt.Println("OK SpotWsPublicLoop ticker:", val.Symbol, val.LastPrice, val.Volume)
					}
					cexObj.SpotWsPublicTickerPoolPut(val)
				case *cex.PublicTrade:
					tradeN++
					if tradeN < 3 {
						fmt.Println("OK SpotWsPublicLoop trade:", val.Symbol, val.Price, val.Qty)
					}
					cexObj.SpotWsPublicTradePoolPut(val)
				}
			case <-timeout:
				cexObj.SpotWsPublicUnsubscribe([]string{"trades@" + symbol, "ticker@" + symbol})
				goto PUBLIC_END
			}
		}
	PUBLIC_END:
		fmt.Println("OK SpotWsPublicLoop: bbo", bboN, "orderbook5", obN, "ticker", tickerN, "trades", tradeN)
		cexObj.SpotWsPublicClose()
	}

	fmt.Println("=========== wss 私有 ===========")
	err = cexObj.SpotWsPrivateOpen()
	if err != nil {
		fmt.Println("Error SpotWsPrivateOpen:", err)
	} else {
		fmt.Println("OK SpotWsPrivateOpen")
		ch := make(chan any, 256)
		go cexObj.SpotWsPrivateLoop(ch)
		cexObj.SpotWsPrivateSubscribe([]string{"orders", "balance"})
		// wss api下单, ack里的orderId用来撤单; 只有binance/gate/okx实现了
		wsOrderId := ""
		wsReqId, err := cexObj.SpotWsPlaceOrder(symbol, gutils.RandomStr(18), price, decimal.Zero, qty,
			"BUY", "GTC", "LIMIT", false)
		if err != nil {
			fmt.Println("Error SpotWsPlaceOrder:", err)
		} else {
			fmt.Println("OK SpotWsPlaceOrder:", wsReqId)
		}
		// rest下单, 让私有wss有订单/资产推送
		if orderId, err := cexObj.SpotPlaceOrder(symbol, gutils.RandomStr(18), price, decimal.Zero, qty,
			"BUY", "GTC", "LIMIT", false); err != nil {
			fmt.Println("Error SpotPlaceOrder(private ws):", err)
		} else {
			fmt.Println("OK SpotPlaceOrder(private ws):", orderId)
		}
		orderN, assetN := 0, 0
		timeout := time.After(wsWait)
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					fmt.Println("Error SpotWsPrivateLoop: chan closed")
					goto PRIV_END
				}
				switch val := v.(type) {
				case *cex.SpotOrder:
					orderN++
					if orderN < 3 {
						fmt.Println("OK SpotWsPrivateLoop order:", val)
					}
					if val.RequestId != "" && val.OrderId != "" && wsOrderId == "" {
						wsOrderId = val.OrderId
						if _, err = cexObj.SpotWsCancelOrder(symbol, wsOrderId, ""); err != nil {
							fmt.Println("Error SpotWsCancelOrder:", err)
						} else {
							fmt.Println("OK SpotWsCancelOrder:", wsOrderId)
						}
					}
				case *cex.SpotAsset:
					assetN++
					if assetN < 3 {
						fmt.Println("OK SpotWsPrivateLoop asset:", val)
					}
				}
			case <-timeout:
				goto PRIV_END
			}
		}
	PRIV_END:
		fmt.Println("OK SpotWsPrivateLoop: orders", orderN, "balance", assetN)
		// 兜底撤单
		if orderL, err := cexObj.SpotGetOpenOrders(symbol); err == nil {
			for _, o := range orderL {
				cexObj.SpotCancelOrder(symbol, o.OrderId, "")
			}
		}
		cexObj.SpotWsPrivateClose()
	}
}
