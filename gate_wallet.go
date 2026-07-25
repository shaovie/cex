package cex

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (gt *Gate) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	return nil
}
func (gt *Gate) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	path := "/api/v4/withdrawals"
	url := gtUniEndpoint + path
	payload := `{"currency":"` + symbol + `"` +
		`,"amount":"` + qty.String() + `"` +
		`,"address":"` + addr + `"` +
		`,"memo":"` + memo + `"` +
		`,"chain":"` + chain + `"` +
		`}`
	headers := gt.buildHeaders("POST", path, "", payload)
	_, resp, err := ihttp.Post(url, []byte(payload), gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Label string `json:"label"`
		Msg   string `json:"message"`

		Id string `json:"id"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Label != "" {
		return nil, errors.New(gt.Name() + " withdrawal fail! msg=" + ret.Msg)
	}

	wr := &WithdrawReturn{
		Symbol: symbol,
		WId:    ret.Id,
	}
	return wr, nil
}
func (gt *Gate) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	path := "/api/v4/wallet/withdrawals"
	params := "currency=" + symbol
	headers := gt.buildHeaders("GET", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	_, resp, err := ihttp.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if resp[0] != '[' {
		return nil, gt.handleExceptionResp("GetWithdrawalHistory", resp)
	}
	ret := []struct {
		Id       string          `json:"id"`
		Symbol   string          `json:"currency"`
		Qty      decimal.Decimal `json:"amount"`
		Fee      decimal.Decimal `json:"fee"`
		Status   string          `json:"status"`
		Txid     string          `json:"txid"`
		DoneTime string          `json:"timestamp2"` // second
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]WithdrawResult, 0, 4)
	for _, v2 := range ret {
		dtime, _ := strconv.ParseInt(v2.DoneTime, 10, 64)
		a := WithdrawResult{
			WId:      v2.Id,
			Symbol:   v2.Symbol,
			Status:   gt.toStdWithdrawStatus(v2.Status),
			Qty:      v2.Qty,
			Txid:     v2.Txid,
			Fee:      v2.Fee,
			DoneTime: dtime,
		}
		if a.Status == "COMPLETED" {
			a.Qty = a.Qty.Sub(a.Fee) // 计算到账数量
		}
		res = append(res, a)
	}
	return res, nil
}
func (gt *Gate) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	path := "/api/v4/wallet/deposit_address"
	params := "currency=" + symbol
	headers := gt.buildHeaders("GET", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	_, resp, err := ihttp.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Label string `json:"label"`
		Msg   string `json:"message"`

		Symbol       string `json:"currency"`
		BindNetworks []struct {
			Network string `json:"chain"`
			Addr    string `json:"address"`
			Memo    string `json:"payment_name"`
		} `json:"multichain_addresses"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	if ret.Label != "" {
		return nil, errors.New(gt.Name() + " request fail! err=" + ret.Msg)
	}
	daL := make([]DepositAddress, 0, 4)
	for _, v := range ret.BindNetworks {
		if network != "" && v.Network != network {
			continue
		}
		daL = append(daL, DepositAddress{
			Network: v.Network,
			Addr:    v.Addr,
			Memo:    v.Memo,
		})
	}
	return daL, nil
}
