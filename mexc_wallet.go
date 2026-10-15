package cex

import (
	"encoding/json"
	"errors"

	"github.com/shopspring/decimal"
)

// fromStdAccountType 转from/to: SPOT,FUTURES
// MEXC 现货和合约各有账户, 其它类型不支持
func (mc *Mexc) fromStdAccountType(typ string) string {
	if typ == "SPOT" {
		return "SPOT"
	} else if typ == "UM_FUTURE" || typ == "CM_FUTURE" || typ == "FUTURES" {
		return "FUTURES"
	}
	return ""
}
func (mc *Mexc) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	fromType := mc.fromStdAccountType(from)
	toType := mc.fromStdAccountType(to)
	if fromType == "" || toType == "" {
		return errors.New(mc.Name() + " transfer not support! from=" + from + " to=" + to)
	}
	params := "fromAccountType=" + fromType + "&toAccountType=" + toType +
		"&asset=" + symbol + "&amount=" + qty.String()
	path := "/api/v3/capital/transfer"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Post(link, nil, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '{' {
		return mc.handleExceptionResp("Transfer", resp)
	}
	ret := struct {
		Code   int         `json:"code,omitempty"`
		Msg    string      `json:"msg,omitempty"`
		TranId json.Number `json:"tranId,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(mc.Name() + " transfer unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(mc.Name() + " transfer fail! " + ret.Msg)
	}
	return nil
}
func (mc *Mexc) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	params := "coin=" + symbol + "&address=" + addr + "&amount=" + qty.String()
	if chain != "" {
		params += "&netWork=" + chain
	}
	if memo != "" {
		params += "&memo=" + memo
	}
	path := "/api/v3/capital/withdraw"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Post(link, nil, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int         `json:"code,omitempty"`
		Msg  string      `json:"msg,omitempty"`
		Id   json.Number `json:"id,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 || len(ret.Id) == 0 {
		return nil, mc.handleExceptionResp("Withdrawal", resp)
	}

	wr := &WithdrawReturn{
		WId:    ret.Id.String(),
		Symbol: symbol,
	}
	return wr, nil
}
func (mc *Mexc) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	params := "limit=100"
	if symbol != "" {
		params += "&coin=" + symbol
	}
	path := "/api/v3/capital/withdraw/history"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, mc.handleExceptionResp("GetWithdrawalHistory", resp)
	}
	ret := []struct {
		Id             string          `json:"id,omitempty"`
		Coin           string          `json:"coin,omitempty"`
		Network        string          `json:"network,omitempty"`
		Address        string          `json:"address,omitempty"`
		Amount         decimal.Decimal `json:"amount"`
		Status         int             `json:"status,omitempty"`
		TransactionFee decimal.Decimal `json:"transactionFee"`
		ApplyTime      int64           `json:"applyTime,omitempty"`
		UpdateTime     int64           `json:"updateTime,omitempty"`
		TxId           string          `json:"txId,omitempty"`
		TransHash      string          `json:"transHash,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]WithdrawResult, 0, len(ret))
	for _, v := range ret {
		doneTime := v.ApplyTime
		if v.UpdateTime > 0 {
			doneTime = v.UpdateTime
		}
		a := WithdrawResult{
			WId:      v.Id,
			Symbol:   v.Coin,
			Status:   mc.toStdWithdrawStatus(v.Status),
			Txid:     v.TransHash,
			Qty:      v.Amount,
			Fee:      v.TransactionFee,
			DoneTime: doneTime / 1000, // second
		}
		if a.Txid == "" {
			a.Txid = v.TxId
		}
		res = append(res, a)
	}
	return res, nil
}
func (mc *Mexc) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	params := "coin=" + symbol
	if network != "" {
		params += "&network=" + network
	}
	path := "/api/v3/capital/deposit/address"
	link := mcUniEndpoint + path + "?" + mc.buildQuery(params)
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, mc.handleExceptionResp("GetDepositAddress", resp)
	}
	ret := []struct {
		Coin    string `json:"coin,omitempty"`
		Network string `json:"network,omitempty"`
		Addr    string `json:"address,omitempty"`
		Memo    string `json:"memo,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret) == 0 {
		return nil, errors.New(mc.Name() + " resp empty!")
	}

	daL := make([]DepositAddress, 0, len(ret))
	for _, v := range ret {
		daL = append(daL, DepositAddress{
			Network: v.Network,
			Addr:    v.Addr,
			Memo:    v.Memo,
		})
	}
	return daL, nil
}

// GetWalletAllAssetInfo 币种与网络的充提配置
func (mc *Mexc) GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error) {
	path := "/api/v3/capital/config/getall"
	link := mcUniEndpoint + path + "?" + mc.buildQuery("")
	_, resp, err := mc.Get(link, mcApiDeadline, mc.buildHeaders())
	if err != nil {
		return nil, errors.New(mc.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, mc.handleExceptionResp("GetWalletAllAssetInfo", resp)
	}
	ret := []struct {
		Coin   string `json:"coin,omitempty"`
		Name   string `json:"name,omitempty"`
		Chains []struct {
			Network             string          `json:"network,omitempty"`
			NetWork             string          `json:"netWork,omitempty"`
			IsDepositEnabled    bool            `json:"depositEnable,omitempty"`
			IsWithdrawalEnabled bool            `json:"withdrawEnable,omitempty"`
			WithdrawFee         decimal.Decimal `json:"withdrawFee"`
			MinWithdrawalAmount decimal.Decimal `json:"withdrawMin"`
			MinDepositAmount    decimal.Decimal `json:"depositMin"`
			WithdrawMultiple    decimal.Decimal `json:"withdrawIntegerMultiple"`
		} `json:"networkList,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(mc.Name() + " unmarshal error! " + err.Error())
	}

	waiMap := make(map[string]*WalletAssetInfo, len(ret))
	for _, v := range ret {
		if len(v.Chains) == 0 {
			continue
		}
		wai := WalletAssetInfo{
			Symbol:            v.Coin,
			IsTransferEnabled: true,
			BindNetworks:      make(map[string]*WalletAssetBindNetworkInfo, len(v.Chains)),
		}
		for _, vv := range v.Chains {
			network := vv.Network
			if vv.NetWork != "" {
				network = vv.NetWork
			}
			wbni := WalletAssetBindNetworkInfo{
				IsWithdrawalEnabled:     vv.IsWithdrawalEnabled,
				IsDepositEnabled:        vv.IsDepositEnabled,
				WithdrawIntegerMultiple: vv.WithdrawMultiple,
				WithdrawFee:             vv.WithdrawFee,
				MinWithdrawalAmount:     vv.MinWithdrawalAmount,
				MinDepositAmount:        vv.MinDepositAmount,
			}
			wai.BindNetworks[network] = &wbni
		}
		waiMap[v.Coin] = &wai
	}
	return waiMap, nil
}
