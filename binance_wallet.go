package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bn *Binance) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	var t string
	if from == "SPOT" && to == "UM_FUTURE" {
		t = "MAIN_UMFUTURE"
	} else if from == "SPOT" && to == "CM_FUTURE" {
		t = "MAIN_CMFUTURE"
	} else if from == "SPOT" && to == "FUNDING" {
		t = "MAIN_FUNDING"
	} else if from == "SPOT" && to == "UNIFIED" {
		t = "MAIN_PORTFOLIO_MARGIN"
	} else if from == "SPOT" && to == "MARGIN" {
		t = "MAIN_MARGIN"
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
	} else if from == "FUNDING" && to == "MARGIN" {
		t = "FUNDING_MARGIN"
	} else if from == "UNIFIED" && to == "SPOT" {
		t = "PORTFOLIO_MARGIN_MAIN"
	} else if from == "MARGIN" && to == "FUNDING" {
		t = "MARGIN_FUNDING"
	} else if from == "MARGIN" && to == "SPOT" {
		t = "MARGIN_SPOT"
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
func (bn *Binance) FundingGetAsset(symbol string) (FundingAsset, error) {
	query := fmt.Sprintf("&asset=%s", symbol)
	query = ""
	url := bnWalletEndpoint + "/sapi/v1/asset/get-funding-asset?" + bn.httpQuerySign(query)
	var fa FundingAsset
	_, resp, err := ihttp.Post(url, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return fa, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return fa, bn.handleExceptionResp("FundingGetAsset", resp)
	}
	recv := []struct {
		Symbol      string          `json:"asset"`
		Free        decimal.Decimal `json:"free"`
		Locked      decimal.Decimal `json:"locked"`
		Freeze      decimal.Decimal `json:"freeze"`
		Withdrawing decimal.Decimal `json:"withdrawing"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return fa, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}

	for _, v := range recv {
		if v.Symbol == symbol {
			return FundingAsset{
				Symbol: v.Symbol,
				Avail:  v.Free,
				Locked: v.Locked,
				Total:  v.Free.Add(v.Locked).Add(v.Freeze).Add(v.Withdrawing),
			}, nil
		}
	}
	return fa, nil
}

func (bn *Binance) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	query := fmt.Sprintf("&coin=%s&network=%s&address=%s&addressTag=%s&amount=%s&walletType=%d",
		symbol, chain, addr, memo, qty.String(), 1)
	url := bnWalletEndpoint + "/sapi/v1/capital/withdraw/apply?" + bn.httpQuerySign(query)
	_, resp, err := ihttp.Post(url, nil, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Id   string `json:"id"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bn.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bn.Name() + " fail! msg=" + ret.Msg)
	}
	wr := &WithdrawReturn{
		WId:    ret.Id,
		Symbol: symbol,
	}
	return wr, nil
}
func (bn *Binance) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	query := fmt.Sprintf("&coin=%s", symbol)
	url := bnWalletEndpoint + "/sapi/v1/capital/withdraw/history?" + bn.httpQuerySign(query)
	_, resp, err := ihttp.Get(url, bnApiDeadline, map[string]string{"X-MBX-APIKEY": bn.apikey})
	if err != nil {
		return nil, errors.New(bn.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, bn.handleExceptionResp("GetWithdrawalHistory", resp)
	}
	ret := []struct {
		Id       string          `json:"id"`
		Symbol   string          `json:"coin"`
		Qty      decimal.Decimal `json:"amount"`
		Fee      decimal.Decimal `json:"transactionFee"`
		Status   int             `json:"status"`
		Txid     string          `json:"txId"`
		CTime    string          `json:"applyTime"`
		DoneTime string          `json:"completeTime"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bn.Name() + " unmarshal error! " + err.Error())
	}

	res := make([]WithdrawResult, 0, len(ret))
	for i := range ret {
		doneTime, _ := time.Parse(time.DateTime, ret[i].DoneTime)
		a := WithdrawResult{
			WId:      ret[i].Id,
			Symbol:   ret[i].Symbol,
			Status:   bn.toStdWithdrawStatus(ret[i].Status),
			Qty:      ret[i].Qty,
			Txid:     ret[i].Txid,
			Fee:      ret[i].Fee,
			DoneTime: doneTime.Unix(),
		}
		res = append(res, a)
	}
	return res, nil
}
