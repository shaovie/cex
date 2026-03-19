package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shaovie/cex"
	"github.com/shaovie/gutils/gutils"
	"github.com/shaovie/gutils/ilog"
	"github.com/shopspring/decimal"
)

func testRest(cexObj cex.Exchanger) {
	ilog.Rinfo("rest api test...")
	accInfo, err := cexObj.MarginGetCrossAccountInfo()
	if err != nil {
		ilog.Rinfo("get accont err: " + err.Error())
		return
	}
	ilog.Rinfo("margin acc: %v", *accInfo)
	marginMaxBorrowable, err := cexObj.MarginGetMaxBorrowable("BTC")
	if err != nil {
		ilog.Rinfo("get max borrowable err: " + err.Error())
		return
	}
	ilog.Rinfo("max borrowable: %v", marginMaxBorrowable)
	bba, err := cexObj.SpotGetBBO("BTCUSDT")
	if err != nil {
		ilog.Rinfo("get bba err: " + err.Error())
		return
	}
	price := bba.AskPrice.Mul(decimal.NewFromFloat(1.05))
	qty := marginMaxBorrowable.Amount.Mul(decimal.NewFromFloat(0.8))
	if exRule := cex.SpotGetExPairRule(cexObj.Name(), "BTCUSDT"); exRule != nil {
		price = exRule.AdjustPrice(price)
		qty = exRule.AdjustQty(price, qty)
		ilog.Rinfo("to palce order: price=%s qty=%s", price.String(), qty.String())
	}
	cltId := gutils.RandomStr(24)
	placeTime := time.Now().UnixMilli()
	orderId, err := cexObj.MarginPlaceOrder("BTCUSDT", cltId, price, decimal.Zero, qty,
		"SELL", "GTC", "LIMIT", "MARGIN_BUY", false)
	if err != nil {
		ilog.Rinfo("place order fail: %s", err.Error())
	} else {
		ilog.Rinfo("place order ok, new order:%s at %d", orderId, placeTime)
		order, err := cexObj.MarginGetOrder("BTCUSDT", orderId, "", false)
		if err != nil {
			ilog.Rinfo("get order fail: ", err.Error())
		} else {
			ilog.Rinfo("get order: %v", *order)
		}
		if order.Status == "NEW" {
			err = cexObj.MarginCancelOrder("BTCUSDT", "", order.ClientId, false)
			if err != nil {
				ilog.Rinfo("cancel order fail: %s", err.Error())
			} else {
				ilog.Rinfo("cancel %s ok", orderId)
			}
		}
		trs, err := cexObj.MarginGetTrades("BTCUSDT", orderId, false)
		if err != nil {
			ilog.Rinfo("get order trades fail: %s", err.Error())
		} else {
			for _, tr := range trs {
				ilog.Rinfo("trades %v", *tr)
			}
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
	testRest(cexObj)

	time.Sleep(1 * time.Second)
	return
}
