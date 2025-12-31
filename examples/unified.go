package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shaovie/cex"
	"github.com/shaovie/gutils/ilog"
)

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
	asL, _ := cexObj.UnifiedGetAssets()
	for _, v := range asL {
		ilog.Rinfo("asset %s %s", v.Symbol, v.Avail.String())
	}

	err = cexObj.UnifiedWsOpen()
	if err != nil {
		ilog.Rinfo("unified ws open err %s", err.Error())
		return
	}
	ilog.Rinfo("unified ws open ok")
	cexObj.UnifiedWsSubscribe([]string{"balance"})
	ch := make(chan any, 256)
	go cexObj.UnifiedWsLoop(ch)
	go func() {
		for v := range ch {
			switch val := v.(type) {
			case *cex.UnifiedAsset:
				ilog.Rinfo("recv asset: %v", *val)
			}
		}
		ilog.Rinfo("unified chan read nil, so ws and chan closed")
	}()
	time.Sleep(300 * time.Second)
	ilog.Rinfo("to close unifiedws loop")
	cexObj.UnifiedWsClose()
	time.Sleep(1 * time.Second)
	return
}
