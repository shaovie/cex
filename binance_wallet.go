package cex

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bn *Binance) Transfer(symbol, from, to string, qty decimal.Decimal) error {
	var t string
	if from == "SPOT" && to == "UM_FUTURE" {
		t = "MAIN_UMFUTURE"
	} else if from == "SPOT" && to == "CM_FUTURE" {
		t = "MAIN_CMFUTURE"
	} else if from == "SPOT" && to == "FUNDING" {
		t = "MAIN_FUNDING"
	} else if from == "SPOT" && to == "UNIFIED" {
		t = "MAIN_PORTFOLIO_MARGIN"
	} else if from == "UM_FUTURE" && to == "SPOT" {
		t = "UMFUTURE_MAIN"
	} else if from == "UM_FUTURE" && to == "FUNDING" {
		t = "UMFUTURE_FUNDING"
	} else if from == "CM_FUTURE" && to == "SPOT" {
		t = "CMFUTURE_MAIN"
	} else if from == "CM_FUTURE" && to == "FUNDING" {
		t = "CMFUTURE_FUNDING"
	} else if from == "FUNDING" && to == "SPOT" {
		t = "FUNDING_MAIN"
	} else if from == "FUNDING" && to == "UM_FUTURE" {
		t = "FUNDING_UMFUTURE"
	} else if from == "FUNDING" && to == "CM_FUTURE" {
		t = "FUNDING_CMFUTURE"
	} else if from == "UNIFIED" && to == "SPOT" {
		t = "PORTFOLIO_MARGIN_MAIN"
	} else {
		return errors.New("not support")
	}

	if !qty.IsPositive() {
		return errors.New(bn.Name() + " transfer qty <= 0. =" + qty.String())
	}
	query := fmt.Sprintf("&type=%s&asset=%s&amount=%s", t, symbol, qty.String())
	url := bnWalletEndpoint + "/sapi/v1/asset/transfer?" + bn.httpQuerySign(query)
	_, resp, err := ihttp.Post(url, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return errors.New(bn.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code   int    `json:"code,omitempty"`
		Msg    string `json:"msg,omitempty"`
		TranId int64  `json:"tranId,omitempty"`
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return errors.New(bn.Name() + " transfer unmarshal fail! " + err.Error())
	}
	if recv.Code != 0 || len(recv.Msg) != 0 {
		return errors.New(bn.Name() + " transfer api err! " + recv.Msg)
	}

	if recv.TranId == 0 {
		return errors.New(bn.Name() + " transfer fail!")
	}
	return nil
}
