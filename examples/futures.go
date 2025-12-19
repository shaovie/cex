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

func testPubWs(cexObj cex.Exchanger, typ string) {
	ilog.Rinfo("public-%s websocket test...", typ)
	err := cexObj.FuturesWsPublicOpen(typ)
	if err != nil {
		ilog.Rinfo("pub ws open err %s", err.Error())
		return
	}
	ilog.Rinfo("pub ws open ok")
	ch := make(chan any, 256)
	allSpotSymbols := cex.FuturesGetAllExPairRule(cexObj.Name())
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
	cexObj.FuturesWsPublicSubscribe([]string{"ticker@" + allSymbols,
		"orderbook5@ETHUSDT,BTCUSDT", "orderbook5@SOLUSDT", "bbo@BTCUSDT"})
	go cexObj.FuturesWsPublicLoop(ch)
	go func() {
		time.Sleep(3 * time.Second)
		cexObj.FuturesWsPublicUnsubscribe([]string{"orderbook5@BTCUSDT", "bbo@BTCUSDT"})
	}()
	ticker := time.NewTicker(time.Duration(99) * time.Millisecond)
	defer ticker.Stop()
	orderBookN := 0
	tickerN := 0
	bboN := 0
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
				cexObj.FuturesWsPublicOrderBook5PoolPut(val)
			case *cex.BestBidAsk:
				if (bboN % 10) == 0 {
					ilog.Rinfo("#%d, %s diff:%d bbo bids:%s,%s ask:%s,%s",
						bboN, val.Symbol, time.Now().UnixMilli()-val.Time, val.BidPrice.String(),
						val.BidQty.String(), val.AskPrice.String(), val.AskQty.String())
				}
				bboN += 1
				cexObj.FuturesWsPublicBBOPoolPut(val)
			case *cex.Pub24hTicker:
				if (tickerN % 10) == 0 {
					ilog.Rinfo("#%d, %s ticker:%v", tickerN, val.Symbol, *val)
				}
				tickerN += 1
				cexObj.FuturesWsPublicTickerPoolPut(val)
			}
		case <-ticker.C:
			if cexObj.FuturesWsPublicIsClosed() {
				ilog.Rinfo("pub ws loop end")
				return
			}
		}
	}
}
func testRest(cexObj cex.Exchanger, typ string) {
	ilog.Rinfo("rest api test...")
	serverTime, err := cexObj.FuturesServerTime(typ)
	if err != nil {
		ilog.Rinfo("ServerTime fail: %s", err.Error())
	} else {
		ilog.Rinfo("local - server diff time: %d", time.Now().UnixMilli()-serverTime)
	}
	allTickers, err := cexObj.FuturesGetAll24hTicker(typ)
	if err != nil {
		ilog.Rinfo("GetAll24hTicker fail: %s", err.Error())
	} else {
		ilog.Rinfo("test get public 24hticker: %v", allTickers["BTCUSDT"])
	}
	price := decimal.NewFromFloat(80990.238)
	qty := decimal.NewFromFloat(0.0022486)
	if exRule := cex.FuturesGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		if typ == "CM" {
			qty = decimal.NewFromInt(1) // exRule.ContractSize
		}
		qty = exRule.AdjustQty(price, qty)
		ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	}
	cltId := gutils.RandomStr(24)
	err = cexObj.FuturesSwitchTradeMode(typ, "BTCUSDT", 0, 2)
	if err != nil {
		ilog.Rinfo("switch trade mode fail: %s", err.Error())
	}
	orderId, err := cexObj.FuturesPlaceOrder(typ, "BTCUSDT", cltId,
		price, qty, "BUY", "LIMIT", "GTC", 0, 0, 0)
	if err != nil {
		ilog.Rinfo("place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("place order ok, new order:%s", orderId)
		order, err := cexObj.FuturesGetOrder(typ, "BTCUSDT", orderId, "")
		if err != nil {
			ilog.Rinfo("get order fail: %s", err.Error())
		} else {
			ilog.Rinfo("get order: %v", *order)
		}
		orderL, err := cexObj.FuturesGetOpenOrders(typ, "BTCUSDT")
		if err != nil {
			ilog.Rinfo("get open orders fail: %s", err.Error())
		} else {
			for _, o := range orderL {
				ilog.Rinfo("get open orders: %v", *o)
			}
		}
		err = cexObj.FuturesCancelOrder(typ, "BTCUSDT", orderId, "")
		if err != nil {
			ilog.Rinfo("cancel order fail: %s", err.Error())
		} else {
			ilog.Rinfo("cancel %s ok", orderId)
		}
	}
}
func futuresPrivWs(cexObj cex.Exchanger, typ string) {
	ilog.Rinfo("private websocket test...")
	err := cexObj.FuturesWsPrivateOpen(typ)
	if err != nil {
		ilog.Rinfo("priv ws open err %s", err.Error())
		return
	}
	ilog.Rinfo("priv ws open ok")
	cexObj.FuturesWsPrivateSubscribe([]string{"orders", "positions"})
	ch := make(chan any, 256)
	go cexObj.FuturesWsPrivateLoop(ch)
	for v := range ch {
		switch val := v.(type) {
		case *cex.FuturesOrder:
			ilog.Rinfo("recv order: %v", *val)
			if val.RequestId != "" {
				break
			}
			if val.Status == "NEW" {
				ilog.Rinfo("to cancel order:%s", val.OrderId)
				if _, err = cexObj.FuturesWsCancelOrder(val.Symbol, val.OrderId, ""); err != nil {
					ilog.Rinfo("cancel err: " + err.Error())
				}
			}
		case *cex.FuturesPosition:
			ilog.Rinfo("recv position: %v", *val)
		}
	}
	ilog.Rinfo("priv chan read nil, so ws and chan closed")
}
func testPrivWs(cexObj cex.Exchanger, typ string) {
	price := decimal.NewFromFloat(80990.238)
	qty := decimal.NewFromFloat(0.00232486)
	if exRule := cex.FuturesGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		if typ == "CM" {
			qty = decimal.NewFromInt(1) // exRule.ContractSize
		}
		qty = exRule.AdjustQty(price, qty)
	}
	go futuresPrivWs(cexObj, typ)
	time.Sleep(2 * time.Second)
	cltId := gutils.RandomStr(24)
	ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	reqId, err := cexObj.FuturesWsPlaceOrder("BTCUSDT", cltId,
		price, qty, "BUY", "LIMIT", "GTC", 0, 0, 0)
	if err != nil {
		ilog.Rinfo("ws place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("ws place order ok, reqId=%s", reqId)
	}
	time.Sleep(1 * time.Second)
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
	typ := os.Getenv("TYP")
	apiKey := os.Getenv("APIKEY")
	secretKey := os.Getenv("SECRETKEY")
	passphrase := os.Getenv("PASSPHRASE")
	ilog.Rinfo("cex=%s typ=%s", cexName, typ)
	// ok,gate,bybit,binance
	cexObj, _ := cex.New(cexName, "", apiKey, secretKey, passphrase)
	testRest(cexObj, typ)
	testPrivWs(cexObj, typ)
	go testPubWs(cexObj, typ)

	time.Sleep(300 * time.Second)

	ilog.Rinfo("to close futures pub ws loop")
	cexObj.FuturesWsPublicClose()

	ilog.Rinfo("to close futures priv ws loop")
	cexObj.FuturesWsPrivateClose()

	time.Sleep(1 * time.Second)
}
