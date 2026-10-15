package cex

import (
	"encoding/binary"
)

// MEXC 的行情推送只支持.pb频道, 报文是protobuf, 这里按官方proto实现最小的wire解析, 不引入protobuf依赖
// proto: https://github.com/mexcdevelop/mexc-api-demo/tree/main/go/clients/spot/websocket/proto

// mcPbEach 遍历protobuf消息的字段: num 字段编号, wire 类型, val 字段内容
// wire=0(变长整数) 时 val 为变长整数的原始字节, 用 mcPbU64 取值
func mcPbEach(b []byte, fn func(num, wire int, val []byte)) {
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return
		}
		b = b[n:]
		num, wire := int(tag>>3), int(tag&7)
		if wire == 0 { // 变长整数
			n = 0
			for n < len(b) && b[n]&0x80 != 0 {
				n++
			}
			if n >= len(b) {
				return
			}
			fn(num, wire, b[:n+1])
			b = b[n+1:]
		} else if wire == 2 { // 长度
			ln, m := binary.Uvarint(b)
			if m <= 0 || ln > uint64(len(b)-m) {
				return
			}
			b = b[m:]
			fn(num, wire, b[:ln])
			b = b[ln:]
		} else if wire == 1 { // 定长8字节
			if len(b) < 8 {
				return
			}
			fn(num, wire, b[:8])
			b = b[8:]
		} else if wire == 5 { // 定长4字节
			if len(b) < 4 {
				return
			}
			fn(num, wire, b[:4])
			b = b[4:]
		} else {
			return
		}
	}
}

// mcPbU64 取变长整数的值
func mcPbU64(b []byte) uint64 {
	v, _ := binary.Uvarint(b)
	return v
}

// mcPbPriceQty 解析 {"price":"x","quantity":"y"} 这类2字段结构
func mcPbPriceQty(b []byte) (price, qty string) {
	mcPbEach(b, func(num, wire int, val []byte) {
		if num == 1 {
			price = string(val)
		} else if num == 2 {
			qty = string(val)
		}
	})
	return
}

// mcPbDeal 解析 PublicAggreDealsV3ApiItem: 1 price 2 quantity 4 time
func mcPbDeal(b []byte) (price, qty string, tm int64) {
	mcPbEach(b, func(num, wire int, val []byte) {
		if num == 1 {
			price = string(val)
		} else if num == 2 {
			qty = string(val)
		} else if num == 4 {
			tm = int64(mcPbU64(val))
		}
	})
	return
}

// mcWsDecode 解析推送数据的包装 PushDataV3ApiWrapper
// 返回: body字段编号, 交易对, 推送时间(msec), body报文
func mcWsDecode(recv []byte) (bodyField int, symbol string, sendTime int64, body []byte) {
	mcPbEach(recv, func(num, wire int, val []byte) {
		if num == 3 { // symbol
			symbol = string(val)
		} else if num == 6 { // sendTime
			sendTime = int64(mcPbU64(val))
		} else if num >= 301 && num <= 315 { // oneof body
			bodyField, body = num, val
		}
	})
	return
}
