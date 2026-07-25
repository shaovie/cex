package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shaovie/cex"
	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func spotPubWs(cexObj cex.Exchanger) {
	ilog.Rinfo("public websocket test...")
	err := cexObj.SpotWsPublicOpen()
	if err != nil {
		ilog.Rinfo("pub ws open err %s", err.Error())
		return
	}
	ilog.Rinfo("pub ws open ok")
	ch := make(chan any, 256)
	allSpotSymbols := cex.SpotGetAllExPairRule(cexObj.Name())
	arr := make([]string, 0, len(allSpotSymbols))
	arr = append(arr, "BTCUSDT")
	for k, _ := range allSpotSymbols {
		arr = append(arr, k)
		if len(arr) > 4 {
			break
		}
	}
	ilog.Rinfo("test load exchange rule: %v", len(arr) > 0)
	allSymbols := strings.Join(arr, ",")
	//cexObj.SpotWsPublicSubscribe([]string{"ticker@" + allSymbols,
	//"orderbook5@ETHUSDT,BTCUSDT", "orderbook5@SOLUSDT", "bbo@SPCXXUSDT", "trades@BTCUSDT"})
	_ = allSymbols
	cexObj.SpotWsPublicSubscribe([]string{"bbo@PRLUSDT"})
	go cexObj.SpotWsPublicLoop(ch)
	go func() {
		time.Sleep(30 * time.Second)
		cexObj.SpotWsPublicUnsubscribe([]string{"orderbook5@BTCUSDT", "bbo@BTCUSDT"})
		ilog.Rinfo("spot pub ws unsubscribe orderbook5@BTCUSDT bbo@BTCUSDT")
	}()
	ticker := time.NewTicker(time.Duration(99) * time.Millisecond)
	defer ticker.Stop()
	bboN := 0
	orderBookN := 0
	tickerN := 0
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				ilog.Rinfo("pubic chan read nil, so ws and chan closed")
				return
			}
			switch val := v.(type) {
			case *cex.OrderBookDepth:
				if (orderBookN % 10) == 0 {
					ilog.Rinfo("#%d, %s orderbook5 bids-1:%v ask-1:%v",
						orderBookN, val.Symbol, val.Bids[0], val.Asks[0])
				}
				orderBookN += 1
				cexObj.SpotWsPublicOrderBook5PoolPut(val)
			case *cex.BestBidAsk:
				if (bboN % 10) == 0 {
					ilog.Rinfo("#%d, %s bbo bids:%s,%s ask:%s,%s",
						bboN, val.Symbol, val.BidPrice.String(),
						val.BidQty.String(), val.AskPrice.String(), val.AskQty.String())
				}
				bboN += 1
				cexObj.SpotWsPublicBBOPoolPut(val)
			case *cex.Pub24hTicker:
				if (tickerN % 10) == 0 {
					ilog.Rinfo("#%d, %s ticker:%v", tickerN, val.Symbol, *val)
				}
				tickerN += 1
				cexObj.SpotWsPublicTickerPoolPut(val)
			case *cex.PublicTrade:
				ilog.Rinfo("%s trade:%v", val.Symbol, *val)
				cexObj.SpotWsPublicTradePoolPut(val)
			}
		case <-ticker.C:
			if cexObj.SpotWsPublicIsClosed() {
				ilog.Rinfo("pub ws loop end")
				return
			}
		}
	}
}
func spotPrivWs(cexObj cex.Exchanger) {
	ilog.Rinfo("private websocket test...")
	err := cexObj.SpotWsPrivateOpen()
	if err != nil {
		ilog.Rinfo("priv ws open err %s", err.Error())
		return
	}
	ilog.Rinfo("priv ws open ok")
	cexObj.SpotWsPrivateSubscribe([]string{"orders", "balance"})
	ch := make(chan any, 256)
	go cexObj.SpotWsPrivateLoop(ch)
	for v := range ch {
		switch val := v.(type) {
		case *cex.SpotOrder:
			ilog.Rinfo("recv order: %v", *val)
			if val.Status == "NEW" {
				ilog.Rinfo("to cancel order:%s", val.OrderId)
				if _, err = cexObj.SpotWsCancelOrder(val.Symbol, val.OrderId, ""); err != nil {
					ilog.Rinfo("cancel err: " + err.Error())
				}
			}
		case *cex.SpotAsset:
			ilog.Rinfo("recv asset: %v", *val)
		}
	}
	ilog.Rinfo("priv chan read nil, so ws and chan closed")
}
func testPubRest(cexObj cex.Exchanger) {
}
func testPubWs(cexObj cex.Exchanger) {
	go spotPubWs(cexObj)
	go func() {
		time.Sleep(50 * time.Second)
		cexObj.SpotWsPublicUnsubscribe([]string{"orderbook5@ETHUSDT,SOLUSDT"})
		ilog.Rinfo("spot pub ws unsubscribe orderbook5@ETHUSDT,SOLUSDT")
		time.Sleep(1 * time.Second)
		cexObj.SpotWsPublicUnsubscribe([]string{"ticker@BTCUSDT"})
		ilog.Rinfo("spot pub ws unsubscribe ticker@BTCUSDT")
	}()
}
func testPrivWs(cexObj cex.Exchanger) {
	price := decimal.NewFromFloat(60990.238)
	qty := decimal.NewFromFloat(0.000324863)
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(price, qty)
	}
	go spotPrivWs(cexObj)
	time.Sleep(2 * time.Second)
	cltId := gutils.RandomStr(18)
	placeTime := time.Now().UnixMilli()
	ilog.Rinfo("to palce order: price=%s qty=%s at %d", price.String(), qty.String(), placeTime)
	reqId, err := cexObj.SpotWsPlaceOrder("BTCUSDT", cltId, price, decimal.Zero, qty, "BUY", "GTC", "LIMIT", false)
	if err != nil {
		ilog.Rinfo("ws place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("ws place order ok, reqId=%s", reqId)
	}
	time.Sleep(1 * time.Second)
}
func testRest(cexObj cex.Exchanger) {
	ilog.Rinfo("rest api test...")
	serverTime, err := cexObj.SpotServerTime()
	if err != nil {
		ilog.Rinfo("SpotServerTime fail: %s", err.Error())
	} else {
		ilog.Rinfo("local - server diff time: %dms", time.Now().UnixMilli()-serverTime)
	}
	allAssets, err := cexObj.SpotGetAllAssets()
	if err != nil {
		ilog.Rinfo("get all asset fail: " + err.Error())
	} else {
		for symbol, as := range allAssets {
			ilog.Rinfo(cexObj.Name() + " " + symbol + " avail: " + as.Avail.String())
		}
	}
	return
	allTickers, err := cexObj.SpotGetAll24hTicker()
	if err != nil {
		ilog.Rinfo("SpotGetAll24hTicker fail: %s", err.Error())
	} else {
		ilog.Rinfo("test get public 24hticker: %v", allTickers["BTCUSDT"])
	}
	transferQty := decimal.NewFromFloat(10.233444)
	err = cexObj.Transfer("USDT", "SPOT", "UNIFIED", "NORMAL", "", transferQty)
	if err == nil {
		ilog.Rinfo("transfer ok")
		time.Sleep(time.Second)
		err = cexObj.Transfer("USDT", "UNIFIED", "SPOT", "NORMAL", "", transferQty)
		if err == nil {
			ilog.Rinfo("transfer back ok")
		} else {
			ilog.Rinfo("transfer back fail: " + err.Error())
		}
	} else {
		ilog.Rinfo("transfer fail: " + err.Error())
	}

	symbol := "AAPLxUSD"
	price := decimal.NewFromFloat(202.34)
	qty := decimal.NewFromFloat(0.02465)
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), symbol); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(price, qty)
		ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	}
	cltId := gutils.RandomStr(18)
	placeTime := time.Now().UnixMilli()
	orderId, err := cexObj.SpotPlaceOrder(symbol, cltId, price, decimal.Zero, qty, "BUY", "GTC", "LIMIT", false)
	if err != nil {
		ilog.Rinfo("place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("place order ok, new order:%s at %d", orderId, placeTime)
		order, err := cexObj.SpotGetOrder(symbol, orderId, "")
		if err != nil {
			ilog.Rinfo("get order fail: ", err.Error())
		} else {
			ilog.Rinfo("get order: %v", *order)
		}
		orderL, err := cexObj.SpotGetOpenOrders(symbol)
		if err != nil {
			ilog.Rinfo("get open orders fail: ", err.Error())
		} else {
			for _, o := range orderL {
				ilog.Rinfo("get open orders: %v", *o)
			}
		}
		err = cexObj.SpotCancelOrder(symbol, orderId, "")
		if err != nil {
			ilog.Rinfo("cancel order fail: %s", err.Error())
		} else {
			ilog.Rinfo("cancel %s ok", orderId)
		}
	}
}
func main() {
	var err error
	if err = ilog.Init("./logs"); err != nil {
		fmt.Println("open log file failed! " + err.Error())
		os.Exit(1)
	}
	if err = cex.Init(); err != nil {
		fmt.Println("cex init failed! " + err.Error())
		os.Exit(1)
	}
	cexName := os.Getenv("CEX")
	apiKey := os.Getenv("APIKEY")
	secretKey := os.Getenv("SECRETKEY")
	passphrase := os.Getenv("PASSPHRASE")
	ilog.Rinfo("spot api:ws test. cex = %s", cexName)
	// ok,gate,bybit,binance
	cexObj, _ := cex.New(cexName, "", apiKey, secretKey, passphrase)
	cexObj.Debug(true)
	orders, _ := cexObj.SpotGetFilledOrders("ICNTUSDT")
	for _, order := range orders {
		ilog.Rinfo("%s, %v", order.Symbol, *order)
		order1, err := cexObj.SpotGetOrder(order.Symbol, order.OrderId, "")
		if err != nil {
			ilog.Rinfo("get order %s err:%s", order.OrderId, err.Error())
			break
		}
		if order1 != nil {
			ilog.Rinfo("%v", *order1)
		}
		break
	}
	//order, err := cexObj.SpotGetOrder("AMDxUSD", "OG7HA3-I656I-VH7MX7", "")
	//ilog.Rinfo("%v", *order)
	//testRest(cexObj)
	return
	/*
		_, err = cexObj.Withdrawal("USDT", "asfdsafdsdf", "232323", "", decimal.NewFromFloat(1000))
		if err != nil {
			ilog.Rinfo("Withdrawal%s", err.Error())
		}*/
	wh, err := cexObj.GetWithdrawalHistory("SPCXx")
	if err != nil {
		ilog.Rinfo("GetWithdrawalHistory %s", err.Error())
	}
	for _, v := range wh {
		ilog.Rinfo("%v", v)
	}
	wh2, err := cexObj.GetDepositAddress("USDT", "")
	if err != nil {
		ilog.Rinfo("GetDepositAddress %s", err.Error())
	}
	for _, v := range wh2 {
		ilog.Rinfo("%v", v)
	}
	testPubWs(cexObj)
	//testPrivWs(cexObj)
	//go spotPrivWs(cexObj)
	//time.Sleep(2 * time.Second)
	//testRest(cexObj)

	time.Sleep(300 * time.Second)

	ilog.Rinfo("to close spot pub ws loop")
	cexObj.SpotWsPublicClose()

	ilog.Rinfo("to close spot priv ws loop")
	cexObj.SpotWsPrivateClose()

	time.Sleep(1 * time.Second)
	return
}
