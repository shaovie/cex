package cex

import (
	"encoding/json/v2"
	"errors"
	"strconv"

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
	_, resp, err := gt.Post(url, []byte(payload), gtApiDeadline, headers)
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
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, gt.handleExceptionResp("GetWithdrawalHistory", resp)
	}
	ret := []struct {
		Id       string          `json:"id"`
		Symbol   string          `json:"currency"`
		Qty      decimal.Decimal `json:"amount"`
		Fee      decimal.Decimal `json:"fee"`
		Status   string          `json:"status"`
		Txid     string          `json:"txid"`
		CTime    string          `json:"timestamp"`
		DoneTime string          `json:"timestamp2"` // second
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]WithdrawResult, 0, 4)
	for _, v2 := range ret {
		dtime, _ := strconv.ParseInt(v2.DoneTime, 10, 64)
		ctime, _ := strconv.ParseInt(v2.CTime, 10, 64)
		a := WithdrawResult{
			WId:      v2.Id,
			Symbol:   v2.Symbol,
			Status:   gt.toStdWithdrawStatus(v2.Status),
			Qty:      v2.Qty,
			Txid:     v2.Txid,
			Fee:      v2.Fee,
			CTime:    ctime,
			DoneTime: dtime,
		}
		if a.Status == "COMPLETED" {
			a.Qty = a.Qty.Sub(a.Fee) // 计算到账数量
		}
		res = append(res, a)
	}
	return res, nil
}
func (gt *Gate) GetDepositHistory(symbol string) ([]DepositResult, error) {
	path := "/api/v4/wallet/deposits"
	params := "currency=" + symbol
	headers := gt.buildHeaders("GET", path, params, "")
	url := gtUniEndpoint + path + "?" + params
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, gt.handleExceptionResp("GetDepositHistory", resp)
	}
	ret := []struct {
		Id     string          `json:"id"`
		Symbol string          `json:"currency"`
		Qty    decimal.Decimal `json:"amount"`
		Status string          `json:"status"`
		Txid   string          `json:"txid"`
		CTime  string          `json:"timestamp"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(gt.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]DepositResult, 0, len(ret))
	for _, v2 := range ret {
		ctime, _ := strconv.ParseInt(v2.CTime, 10, 64)
		a := DepositResult{
			DId:    v2.Id,
			Symbol: v2.Symbol,
			Status: gt.toStdDepositStatus(v2.Status),
			Qty:    v2.Qty,
			Txid:   v2.Txid,
			CTime:  ctime,
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
	_, resp, err := gt.Get(url, gtApiDeadline, headers)
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

// GetWalletAllAssetInfo 币种链级充值/提现可用状态, 结构与binance保持一致
func (gt *Gate) GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error) {
	path := "/api/v4/spot/currencies"
	_, resp, err := gt.Get(gtUniEndpoint+path, gtApiDeadline, nil)
	if err != nil {
		return nil, errors.New(gt.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, gt.handleExceptionResp("GetWalletAllAssetInfo", resp)
	}
	recv := []struct {
		Symbol           string `json:"currency"`
		WithdrawDisabled bool   `json:"withdraw_disabled"`
		WithdrawDelayed  bool   `json:"withdraw_delayed"`
		DepositDisabled  bool   `json:"deposit_disabled"`
		Chains           []struct {
			Name             string `json:"name"`
			WithdrawDisabled bool   `json:"withdraw_disabled"`
			WithdrawDelayed  bool   `json:"withdraw_delayed"`
			DepositDisabled  bool   `json:"deposit_disabled"`
		} `json:"chains"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(gt.Name() + " unmarshal error! " + err.Error())
	}
	waiMap := make(map[string]*WalletAssetInfo, len(recv))
	for _, v := range recv {
		if len(v.Chains) == 0 {
			continue
		}
		wai := &WalletAssetInfo{
			Symbol:            v.Symbol,
			IsTransferEnabled: true,
			TransferScale:     8,
			BindNetworks:      make(map[string]*WalletAssetBindNetworkInfo, len(v.Chains)),
		}
		for _, chain := range v.Chains {
			wai.BindNetworks[chain.Name] = &WalletAssetBindNetworkInfo{
				// withdraw_delayed 也算不可正常提现
				IsWithdrawalEnabled: !v.WithdrawDisabled && !v.WithdrawDelayed &&
					!chain.WithdrawDisabled && !chain.WithdrawDelayed,
				IsDepositEnabled: !v.DepositDisabled && !chain.DepositDisabled,
			}
		}
		waiMap[v.Symbol] = wai
	}
	return waiMap, nil
}
