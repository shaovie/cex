package cex

import (
	"math/rand"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ilog"
)

var (
	spotExchangePairRule        map[string]map[string]*SpotExchangePairRule
	spotExchangePairRuleMtx     sync.RWMutex
	contractExchangePairRule    map[string]map[string]*ContractExchangePairRule
	contractExchangePairRuleMtx sync.RWMutex
)

func init() {
	spotExchangePairRule = make(map[string]map[string]*SpotExchangePairRule)
	contractExchangePairRule = make(map[string]map[string]*ContractExchangePairRule)
}
func ruleInit() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		spotUpdateExPairRule()
	}()
	go func() {
		defer wg.Done()
		contractUpdateExPairRule()
	}()
	wg.Wait()

	go func() {
		rSleep := 120 + rand.Int63()%10
		for range time.Tick(time.Duration(rSleep) * time.Second) { // go 1.23+
			spotUpdateExPairRule()
		}
	}()
	go func() {
		rSleep := 122 + rand.Int63()%10
		for range time.Tick(time.Duration(rSleep) * time.Second) { // go 1.23+
			contractUpdateExPairRule()
		}
	}()
}
func SpotGetAllExPairRule(cex string) map[string]*SpotExchangePairRule {
	spotExchangePairRuleMtx.RLock()
	defer spotExchangePairRuleMtx.RUnlock()
	return spotExchangePairRule[cex]
}
func SpotGetExPairRule(cex, symbol string) *SpotExchangePairRule {
	spotExchangePairRuleMtx.RLock()
	defer spotExchangePairRuleMtx.RUnlock()
	if exr := spotExchangePairRule[cex]; exr != nil {
		return exr[symbol]
	}
	return nil
}
func SpotSymbolValid(cex, symbol /*btcusdt*/ string) bool {
	spotExchangePairRuleMtx.RLock()
	defer spotExchangePairRuleMtx.RUnlock()
	if exr := spotExchangePairRule[cex]; exr != nil {
		return exr[symbol] != nil
	}
	return false
}
func SpotSymbolQuote(cex, symbol /*btcusdt*/ string) string {
	spotExchangePairRuleMtx.RLock()
	defer spotExchangePairRuleMtx.RUnlock()
	if exr := spotExchangePairRule[cex]; exr != nil {
		if s := exr[symbol]; s != nil {
			return s.Quote
		}
	}
	return ""
}
func spotUpdateExPairRule() {
	var wg sync.WaitGroup
	for k, _ := range CexList {
		wg.Add(1)
		go func(cexName string) {
			defer wg.Done()
			if co, _ := New(cexName, "", "", "", ""); co != nil {
				if ret, err := co.SpotLoadAllPairRule(); ret != nil {
					spotExchangePairRuleMtx.Lock()
					spotExchangePairRule[cexName] = ret
					spotExchangePairRuleMtx.Unlock()
				} else if err != nil {
					ilog.Warning("cex.rule.spotUpdateExPairRule: " + err.Error())
				}
			}
		}(k)
	}
	wg.Wait()
}
func ContractGetAllExPairRule(cex string) map[string]*ContractExchangePairRule {
	contractExchangePairRuleMtx.RLock()
	defer contractExchangePairRuleMtx.RUnlock()
	return contractExchangePairRule[cex]
}
func ContractGetExPairRule(cex, symbol string) *ContractExchangePairRule {
	contractExchangePairRuleMtx.RLock()
	defer contractExchangePairRuleMtx.RUnlock()
	if exr := contractExchangePairRule[cex]; exr != nil {
		return exr[symbol]
	}
	return nil
}
func GetContractMultiplier(cex, symbol /*btcusdt*/ string) decimal.Decimal {
	contractExchangePairRuleMtx.RLock()
	defer contractExchangePairRuleMtx.RUnlock()
	if exr := contractExchangePairRule[cex]; exr != nil {
		if v := exr[symbol]; v != nil {
			return v.ContractMultiplier
		}
	}
	return decimal.Zero
}
func ContractSymbolValid(cex, symbol /*btcusdt*/ string) bool {
	contractExchangePairRuleMtx.RLock()
	defer contractExchangePairRuleMtx.RUnlock()
	if exr := contractExchangePairRule[cex]; exr != nil {
		return exr[symbol] != nil
	}
	return false
}
func contractUpdateExPairRule() {
	var wg sync.WaitGroup
	for k, _ := range CexList {
		wg.Add(1)
		go func(cexName string) {
			defer wg.Done()
			if co, _ := New(cexName, "", "", "", ""); co != nil {
				if co.ContractSupported() == false {
					return
				}
				if ret, err := co.ContractLoadAllPairRule("linear"); ret != nil {
					contractExchangePairRuleMtx.Lock()
					contractExchangePairRule[cexName] = ret
					contractExchangePairRuleMtx.Unlock()
				} else if err != nil {
					ilog.Warning("cex.rule.contractUpdateExPairRule: " + err.Error())
				}
			}
		}(k)
	}
	wg.Wait()
}

// 计算 0.1 的 n 次方
func PowOneTenth(n int) decimal.Decimal {
	b01 := decimal.NewFromFloat(0.1)
	b1 := decimal.NewFromInt(1)
	for i := 0; i < n; i++ {
		b1 = b1.Mul(b01)
	}
	return b1
}
