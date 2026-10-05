package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Transfer 该交易所没有账户间资金划转
func (sa *Safetrade) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	return nil
}
func (sa *Safetrade) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	if memo != "" { // 该交易所把memo拼接在地址上
		addr += "?memo=" + memo
	}
	payload := `{"currency":"` + strings.ToLower(symbol) + `"` +
		`,"blockchain_key":"` + chain + `"` +
		`,"address":"` + addr + `"` +
		`,"amount":` + qty.String() +
		`}`
	link := stEndpoint + "/trade/account/withdraws"
	_, resp, err := sa.Post(link, []byte(payload), stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Errors []string `json:"errors,omitempty"`
		Id     int64    `json:"id,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Errors) > 0 {
		return nil, errors.New(sa.Name() + ": " + ret.Errors[0])
	}
	if ret.Id == 0 {
		return nil, errors.New(sa.Name() + " withdraw id is empty!")
	}

	wr := &WithdrawReturn{
		WId:    strconv.FormatInt(ret.Id, 10),
		Symbol: symbol,
	}
	return wr, nil
}
func (sa *Safetrade) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	link := stEndpoint + "/trade/account/withdraws?limit=100&page=1"
	if symbol != "" {
		link += "&currency=" + strings.ToLower(symbol)
	}
	_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, sa.handleExceptionResp("GetWithdrawalHistory", resp)
	}
	ret := []struct {
		Id       int64           `json:"id"`
		Symbol   string          `json:"currency_id"`
		Qty      decimal.Decimal `json:"amount"`
		Fee      decimal.Decimal `json:"fee"`
		Status   string          `json:"status"`
		Txid     string          `json:"txid"`
		DoneTime string          `json:"completed_at,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]WithdrawResult, 0, len(ret))
	for _, v := range ret {
		var doneTime int64 // 未完成时completed_at为空
		if v.DoneTime != "" {
			dtime, _ := time.Parse(time.RFC3339, v.DoneTime)
			doneTime = dtime.Unix()
		}
		a := WithdrawResult{
			WId:      strconv.FormatInt(v.Id, 10),
			Symbol:   strings.ToUpper(v.Symbol),
			Status:   sa.toStdWithdrawStatus(v.Status),
			Txid:     v.Txid,
			Qty:      v.Qty,
			Fee:      v.Fee,
			DoneTime: doneTime,
		}
		res = append(res, a)
	}
	return res, nil
}
func (sa *Safetrade) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	link := stEndpoint + "/trade/account/deposit_address/" + strings.ToLower(symbol)
	if network != "" {
		link += "?network=" + network
	}
	_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Errors  []string `json:"errors,omitempty"`
		Addr    string   `json:"address"`
		Network string   `json:"network"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Errors) > 0 {
		return nil, errors.New(sa.Name() + ": " + ret.Errors[0])
	}
	if ret.Addr == "" {
		return nil, errors.New(sa.Name() + " resp empty!")
	}

	daL := []DepositAddress{
		{
			Network: ret.Network,
			Addr:    ret.Addr,
		},
	}
	return daL, nil
}
