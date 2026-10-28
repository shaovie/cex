package cex

import (
	"errors"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/shopspring/decimal"
)

var (
	spotExchangePairRule       map[string]map[string]*SpotExchangePairRule
	spotExchangePairRuleMtx    sync.RWMutex
	futuresExchangePairRule    map[string]map[string]*FuturesExchangePairRule
	futuresExchangePairRuleMtx sync.RWMutex

	ruleCacheFirst atomic.Bool  // 只在ruleInit期间为true, 外部主动更新时仍走实时请求
	ruleCacheTTL   atomic.Int64 // 规则缓存有效期(nanosecond)
)

func init() {
	spotExchangePairRule = make(map[string]map[string]*SpotExchangePairRule)
	futuresExchangePairRule = make(map[string]map[string]*FuturesExchangePairRule)
}
func spotPairRuleCacheFile(cexName string) string {
	return "/tmp/cex_" + cexName + "_spot_pair_rule.json"
}
func ruleInit() {
	if ruleCacheTTL.Load() > 0 {
		ruleCacheFirst.Store(true)
		defer ruleCacheFirst.Store(false)
	}
	var wg sync.WaitGroup
	for cexName := range CexList {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			SpotUpdateExPairRule(name)
		}(cexName)
		for _, typ := range []string{"UM", "CM"} {
			wg.Add(1)
			go func(name, typ string) {
				defer wg.Done()
				FuturesUpdateExPairRule(name, typ)
			}(cexName, typ)
		}
	}
	wg.Wait()
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

// SpotUpdateExPairRule 更新指定交易所的现货交易规则, 由外部按需调用
func SpotUpdateExPairRule(cexName string) error {
	if co, _ := NewPublic(cexName); co != nil {
		if ret, err := co.SpotLoadAllPairRule(); ret != nil {
			spotExchangePairRuleMtx.Lock()
			spotExchangePairRule[cexName] = ret
			spotExchangePairRuleMtx.Unlock()
			return nil
		} else if err != nil {
			return err
		} else {
			return errors.New("unknown error")
		}
	}
	return errors.New("unknown exchange: " + cexName)
}
func FuturesGetAllExPairRule(cex string) map[string]*FuturesExchangePairRule {
	futuresExchangePairRuleMtx.RLock()
	defer futuresExchangePairRuleMtx.RUnlock()
	return futuresExchangePairRule[cex]
}
func FuturesGetExPairRule(cex, symbol string) *FuturesExchangePairRule {
	futuresExchangePairRuleMtx.RLock()
	defer futuresExchangePairRuleMtx.RUnlock()
	if exr := futuresExchangePairRule[cex]; exr != nil {
		return exr[symbol]
	}
	return nil
}
func GetFuturesMultiplier(cex, symbol /*btcusdt*/ string) decimal.Decimal {
	futuresExchangePairRuleMtx.RLock()
	defer futuresExchangePairRuleMtx.RUnlock()
	if exr := futuresExchangePairRule[cex]; exr != nil {
		if v := exr[symbol]; v != nil {
			return v.ContractMultiplier
		}
	}
	return decimal.Zero
}
func FuturesSymbolValid(cex, symbol /*btcusdt*/ string) bool {
	futuresExchangePairRuleMtx.RLock()
	defer futuresExchangePairRuleMtx.RUnlock()
	if exr := futuresExchangePairRule[cex]; exr != nil {
		return exr[symbol] != nil
	}
	return false
}

// FuturesUpdateExPairRule 更新指定交易所指定类型的合约交易规则, typ: UM/CM, 由外部按需调用
func FuturesUpdateExPairRule(cexName, typ string) error {
	co, err := NewPublic(cexName)
	if co == nil {
		return err
	}
	if !co.FuturesSupported(typ) {
		return errors.New(co.Name() + " not support futures type! " + typ)
	}
	ret, err := co.FuturesLoadAllPairRule(typ)
	if err != nil {
		return err
	}
	if ret == nil {
		return errors.New("unknown error")
	}
	futuresExchangePairRuleMtx.Lock()
	old := futuresExchangePairRule[cexName]
	ol := make(map[string]*FuturesExchangePairRule, len(old)+len(ret))
	maps.Copy(ol, old)
	maps.Copy(ol, ret)
	futuresExchangePairRule[cexName] = ol
	futuresExchangePairRuleMtx.Unlock()
	return nil
}

// 计算 0.1 的 n 次方
func PowOneTenth(n int) decimal.Decimal {
	b01 := decimal.NewFromFloat(0.1)
	b1 := decimal.NewFromInt(1)
	for range n {
		b1 = b1.Mul(b01)
	}
	return b1
}
