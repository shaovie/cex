package cex

import (
	"sync"
	"sync/atomic"
	"time"
)

// CexOptions cex对象的可选配置, 每个交易所结构体里内嵌一份
type CexOptions struct {
	spotBboThrottle    WsBboThrottle // 现货公共ws的BBO节流
	futuresBboThrottle WsBboThrottle // 合约公共ws的BBO节流
}

// SpotWsPublicSetBBOInterval 设置某交易对BBO推送的最小间隔(毫秒), 间隔内的推送丢弃(0=不过滤)
// 按标准符号(BestBidAsk.Symbol那种)设置; 支持运行中修改
func (o *CexOptions) SpotWsPublicSetBBOInterval(symbol string, interval int64) {
	o.spotBboThrottle.SetInterval(symbol, interval)
}

// spotWsBboAllow 该symbol的这条BBO是否放行; 没配置节流时连时间都不取
// 只在ws public loop里调用(单goroutine)
func (o *CexOptions) spotWsBboAllow(symbol string) bool {
	return o.spotBboThrottle.Allow(symbol)
}

// FuturesWsPublicSetBBOInterval 设置某合约交易对BBO推送的最小间隔(毫秒), 间隔内的推送丢弃(0=不过滤)
// 按标准符号(BestBidAsk.Symbol那种)设置; 支持运行中修改
func (o *CexOptions) FuturesWsPublicSetBBOInterval(symbol string, interval int64) {
	o.futuresBboThrottle.SetInterval(symbol, interval)
}

// futuresWsBboAllow 该symbol的这条BBO是否放行; 没配置节流时连时间都不取
// 只在ws public loop里调用(单goroutine)
func (o *CexOptions) futuresWsBboAllow(symbol string) bool {
	return o.futuresBboThrottle.Allow(symbol)
}

// resetBboThrottle 清空现货/合约BBO的放行时间记录, 配置的interval保留; 交易所Init里调用
func (o *CexOptions) resetBboThrottle() {
	o.spotBboThrottle.Reset()
	o.futuresBboThrottle.Reset()
}

// WsBboThrottle 按交易对节流BBO推送: n毫秒内同一symbol只保留第一条
// 配置(interval)用写时复制+原子发布, 支持运行中修改, 读侧无锁无分配;
// lastPush只在ws loop这一个goroutine里读写, Reset时loop必须已退出, 所以都不需要锁(现货/合约各用一个实例)
type WsBboThrottle struct {
	mtx      sync.RWMutex
	interval atomic.Pointer[map[string]int64] // symbol(标准符号) -> 最小间隔ms, 发布后只读
	lastPush map[string]int64                 // symbol -> 上次放行的时间ms
}

// SetInterval 设置symbol的BBO最小间隔(毫秒), <=0表示不过滤
func (t *WsBboThrottle) SetInterval(symbol string, interval int64) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	old := t.interval.Load()
	m := make(map[string]int64, 4)
	if old != nil {
		for k, v := range *old {
			m[k] = v
		}
	}
	if interval <= 0 {
		delete(m, symbol)
	} else {
		m[symbol] = interval
	}
	t.interval.Store(&m)
}

// Reset 清空放行时间记录, 配置的interval保留; 调用时ws loop必须已退出(Init里保证)
func (t *WsBboThrottle) Reset() {
	clear(t.lastPush)
}

// Allow 该symbol的这条BBO是否放行: 间隔内丢弃, 首次(没有记录)一定放行
// 没配置节流或symbol没配置时直接放行, 连时间都不取
func (t *WsBboThrottle) Allow(symbol string) bool {
	m := t.interval.Load()
	if m == nil {
		return true
	}
	interval, ok := (*m)[symbol]
	if !ok || interval <= 0 {
		return true
	}
	now := time.Now().UnixMilli()
	if last, ok := t.lastPush[symbol]; ok && now-last < interval {
		return false
	}
	if t.lastPush == nil {
		t.lastPush = make(map[string]int64, 4)
	}
	t.lastPush[symbol] = now
	return true
}
