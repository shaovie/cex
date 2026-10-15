package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// fromStdTransferType 转from/to: SPOT,FUTURE
func (as *Aster) fromStdTransferType(typ string) string {
	if typ == "SPOT" {
		return "SPOT"
	} else if typ == "FUTURE" || typ == "UM_FUTURE" || typ == "FUTURES" {
		return "FUTURE"
	}
	return ""
}

// Transfer 现货与合约账户互转, aster只支持SPOT<->FUTURE
func (as *Aster) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	fromType := as.fromStdTransferType(from)
	toType := as.fromStdTransferType(to)
	if fromType == "" || toType == "" || fromType == toType {
		return errors.New(as.Name() + " transfer not support! from=" + from + " to=" + to)
	}
	params := "asset=" + symbol + "&amount=" + qty.String() +
		"&kindType=" + fromType + "_" + toType +
		"&clientTranId=WC" + strconv.FormatInt(time.Now().UnixNano(), 10)
	path := "/api/v3/asset/wallet/transfer"
	link := asSpotEndpoint + path + "?" + as.buildQuery(params)
	_, resp, err := as.Post(link, nil, asApiDeadline, nil)
	if err != nil {
		return errors.New(as.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"code,omitempty"`
		Msg    string `json:"msg,omitempty"`
		TranId int64  `json:"tranId,omitempty"`
		Status string `json:"status,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(as.Name() + " transfer unmarshal fail! " + err.Error())
	}
	if ret.TranId == 0 || ret.Status != "SUCCESS" {
		return as.handleExceptionResp("Transfer", resp)
	}
	return nil
}
