package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"cex"
	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

var (
	spotTicker24hPool *sync.Pool
	spotOrderBookPool *sync.Pool
)

func init() {
	spotTicker24hPool = &sync.Pool{
		New: func() any { return &cex.Spot24hTicker{} },
	}
	spotOrderBookPool = &sync.Pool{
		New: func() any {
			return &cex.OrderBookDepth{
				Bids: make([]cex.Ticker, 0, 5),
				Asks: make([]cex.Ticker, 0, 5),
			}
		},
	}
}
func spotPubWs(cexObj cex.Exchanger) {
	ilog.Rinfo("to open pub ws")
	err := cexObj.SpotWsPublicOpen()
	if err != nil {
		ilog.Rinfo("ws err %s", err.Error())
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
	ilog.Rinfo("allsymbols len=%d", len(arr))
	allSymbols := strings.Join(arr, ",")
	cexObj.SpotWsPublicSubscribe([]string{"ticker@" + allSymbols, "orderbook5@ETHUSDT,BTCUSDT", "orderbook5@SOLUSDT"})
	cexObj.SpotWsPublicSetTickerPool(spotTicker24hPool)
	cexObj.SpotWsPublicSetOrderBookPool(spotOrderBookPool)
	go cexObj.SpotWsPublicLoop(ch)
	ticker := time.NewTicker(time.Duration(99) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				continue
			}
			switch val := v.(type) {
			case *cex.OrderBookDepth:
				ilog.Rinfo("%s orderbook5 bids-1:%v ask-1:%v", val.Symbol, val.Bids[0], val.Asks[0])
				spotOrderBookPool.Put(val)
			case *cex.Spot24hTicker:
				ilog.Rinfo("%s ticker:%v", val.Symbol, *val)
				spotTicker24hPool.Put(val)
			}
		case <-ticker.C:
			if cexObj.SpotWsPublicIsClosed() {
				ilog.Rinfo("pub ch close")
				return
			}
		}
	}
}
func spotPrivWs(cexObj cex.Exchanger) {
	ilog.Rinfo("to open priv ws")
	err := cexObj.SpotWsPrivateOpen()
	if err != nil {
		ilog.Rinfo("priv ws err %s", err.Error())
		return
	}
	ilog.Rinfo("priv ws open ok")
	cexObj.SpotWsPrivateSubscribe([]string{"orders"})
	ch := make(chan any, 256)
	go cexObj.SpotWsPrivateLoop(ch)
	ticker := time.NewTicker(time.Duration(99) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				continue
			}
			if orderP := v.(*cex.SpotOrder); orderP != nil {
				ilog.Rinfo("order:%v", *orderP)
				if orderP.Status == "NEW" {
					ilog.Rinfo("to cancel order:%s", orderP.OrderId)
					if err = cexObj.SpotWsCancelOrder(orderP.Symbol, orderP.OrderId, ""); err != nil {
						ilog.Rinfo("cancel err: " + err.Error())
					}
				}
			}
		case <-ticker.C:
			if cexObj.SpotWsPrivateIsClosed() {
				ilog.Rinfo("priv ch close")
				return
			}
		}
	}
}
func testPubRest(cexObj cex.Exchanger) {
}
func testPubWs(cexObj cex.Exchanger) {
	go spotPubWs(cexObj)
	time.Sleep(10 * time.Second)
	cexObj.SpotWsPublicUnsubscribe([]string{"orderbook5@ETHUSDT,SOLUSDT"})
	ilog.Rinfo("spot ws public unsubscribe ethusdt,solusdt")
	time.Sleep(600 * time.Second)
	cexObj.SpotWsPublicClose()
	time.Sleep(1 * time.Second)
}
func testPrivWs(cexObj cex.Exchanger) {
	price := decimal.NewFromFloat(80990.238)
	qty := decimal.NewFromFloat(0.00032486)
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(qty)
		ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	}
	go spotPrivWs(cexObj)
	time.Sleep(2 * time.Second)
	cltId := gutils.RandomStr(24)
	_, err := cexObj.SpotWsPlaceOrder("BTCUSDT", cltId, price, qty, "BUY", "GTC", "LIMIT")
	if err != nil {
		ilog.Rinfo("ws place order fail: ", err.Error())
	}

	time.Sleep(120 * time.Second)
	cexObj.SpotWsPrivateClose()
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
	apiKey := os.Getenv("APIKEY")
	secretKey := os.Getenv("SECRETKEY")
	passphrase := os.Getenv("PASSPHRASE")
	// ok,gate,bybit,binance
	cexObj, _ := cex.New("okx", "", apiKey, secretKey, passphrase)
	testPubWs(cexObj)
	//testPrivWs(cexObj)
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
