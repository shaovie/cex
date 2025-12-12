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
	cexObj.SpotWsPublicSubscribe([]string{"ticker@" + allSymbols, "orderbook5@ETHUSDT,BTCUSDT", "orderbook5@SOLUSDT"})
	go cexObj.SpotWsPublicLoop(ch)
	ticker := time.NewTicker(time.Duration(99) * time.Millisecond)
	defer ticker.Stop()
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
			case *cex.Spot24hTicker:
				if (tickerN % 10) == 0 {
					ilog.Rinfo("#%d, %s ticker:%v", tickerN, val.Symbol, *val)
				}
				tickerN += 1
				cexObj.SpotWsPublicTickerPoolPut(val)
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
				if err = cexObj.SpotWsCancelOrder(val.Symbol, val.OrderId, ""); err != nil {
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
		time.Sleep(10 * time.Second)
		cexObj.SpotWsPublicUnsubscribe([]string{"orderbook5@ETHUSDT,SOLUSDT"})
		ilog.Rinfo("spot pub ws unsubscribe orderbook5@ETHUSDT,SOLUSDT")
		time.Sleep(1 * time.Second)
		cexObj.SpotWsPublicUnsubscribe([]string{"ticker@BTCUSDT"})
		ilog.Rinfo("spot pub ws unsubscribe ticker@BTCUSDT")
	}()
}
func testPrivWs(cexObj cex.Exchanger) {
	price := decimal.NewFromFloat(80990.238)
	qty := decimal.NewFromFloat(0.00032486)
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(qty)
	}
	allAssets, err := cexObj.SpotGetAllAssets()
	if err != nil {
		ilog.Rinfo("get all asset fail: " + err.Error())
	} else {
		for symbol, as := range allAssets {
			ilog.Rinfo(cexObj.Name() + " " + symbol + " avail: " + as.Avail.String())
		}
	}
	go spotPrivWs(cexObj)
	time.Sleep(2 * time.Second)
	cltId := gutils.RandomStr(24)
	ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	reqId, err := cexObj.SpotWsPlaceOrder("BTCUSDT", cltId, price, qty, "BUY", "GTC", "LIMIT")
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
		ilog.Rinfo("local - server diff time: %d", time.Now().UnixMilli()-serverTime)
	}
	allTickers, err := cexObj.SpotGetAll24hTicker()
	if err != nil {
		ilog.Rinfo("SpotGetAll24hTicker fail: %s", err.Error())
	} else {
		ilog.Rinfo("test get public 24hticker: %v", allTickers["BTCUSDT"])
	}
	price := decimal.NewFromFloat(80990.238)
	qty := decimal.NewFromFloat(0.00032486)
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(qty)
		ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	}
	cltId := gutils.RandomStr(24)
	orderId, err := cexObj.SpotPlaceOrder("BTCUSDT", cltId, price, qty, "BUY", "GTC", "LIMIT")
	if err != nil {
		ilog.Rinfo("place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("place order ok, new order:%s", orderId)
		order, err := cexObj.SpotGetOrder("BTCUSDT", orderId, "")
		if err != nil {
			ilog.Rinfo("get order fail: ", err.Error())
		} else {
			ilog.Rinfo("get order: %v", *order)
		}
		orderL, err := cexObj.SpotGetOpenOrders("BTCUSDT")
		if err != nil {
			ilog.Rinfo("get open orders fail: ", err.Error())
		} else {
			for _, o := range orderL {
				ilog.Rinfo("get open orders: %v", *o)
			}
		}
		err = cexObj.SpotCancelOrder("BTCUSDT", orderId, "")
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
	ilog.Rinfo("cex=%s", cexName)
	// ok,gate,bybit,binance
	cexObj, _ := cex.New(cexName, "", apiKey, secretKey, passphrase)
	testRest(cexObj)
	testPrivWs(cexObj)
	testPubWs(cexObj)

	time.Sleep(300 * time.Second)

	ilog.Rinfo("to close spot pub ws loop")
	cexObj.SpotWsPublicClose()

	ilog.Rinfo("to close spot priv ws loop")
	cexObj.SpotWsPrivateClose()

	time.Sleep(1 * time.Second)
	return

	/*
		as, err := cexObj.UnifiedGetAssets()
		if err != nil {
			ilog.Rinfo("get asset err: %s", err.Error())
		} else {
			for k, v := range as {
				ilog.Rinfo("asset %s=%v", k, *v)
			}
		}
		if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
			price = exRule.AdjustPrice(price)
			qty = exRule.AdjustQty(qty)
			ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
		}

		go spotPubWs(cexObj)
		time.Sleep(2 * time.Second)
		cexObj.SpotWsPublicClose()

		if true {
			spotPrivWs(cexObj)
			cltId := gutils.RandomStr(24)
			orderId, err := cexObj.SpotPlaceOrder("BTCUSDT", cltId, price, qty, "BUY", "GTC", "LIMIT")
			if err != nil {
				ilog.Rinfo("api place order=%s", err.Error())
			}
			ord, err := cexObj.SpotGetOrder("BTCUSDT", orderId, "")
			if err != nil {
				ilog.Rinfo("get order fail = " + err.Error())
			} else {
				ilog.Rinfo("order = %v", *ord)
			}
			err = cexObj.SpotCancelOrder("BTCUSDT", orderId, "")
			if err != nil {
				ilog.Rinfo("cancel order %s fail %s", orderId, err.Error())
			}

			cexObj.SpotWsCancelOrder("BTCUSDT", orderId, "")

			orderType := "LIMIT"
			if orderType == "MARKET" {
				qty = decimal.NewFromFloat(100)
			}
			_, err = cexObj.SpotWsPlaceOrder("BTCUSDT", cltId, price, qty, "BUY", "GTC", orderType)
			if err != nil {
				ilog.Rinfo("ws place order=%s", err.Error())
			}
			time.Sleep(60 * time.Second)
			cexObj.SpotWsPrivateClose()
		}

		time.Sleep(1 * time.Second)
		ilog.Rinfo("test end")
	*/
}
