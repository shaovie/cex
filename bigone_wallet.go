package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/shopspring/decimal"
)

func (bo *Bigone) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	if symbol == "XAUT" {
		symbol = "XAUt"
	}
	if from == "FUNDING" {
		from = "FUND"
	}
	if from == "UM_FUTURE" || from == "CM_FUTURE" {
		from = "CONTRACT"
	}
	if to == "FUNDING" {
		to = "FUND"
	}
	if to == "UM_FUTURE" || to == "CM_FUTURE" {
		to = "CONTRACT"
	}
	url := boSpotEndpoint + "/viewer/transfer"
	jwt := "Bearer " + bo.jwt()
	guid, _ := uuid.NewV4()
	payload := `{"symbol":"` + symbol + `"` +
		`,"amount":"` + qty.String() + `"` +
		`,"guid":"` + guid.String() + `"` +
		`,"from":"` + from + `"` +
		`,"to":"` + to + `"` +
		`,"sub_account":"` + subAccount + `"` +
		`,"type":"` + typ + `"` +
		`}`
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	_, resp, err := bo.Post(url, []byte(payload), boApiDeadline, header)
	if err != nil {
		return errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(bo.Name() + " transfer fail! msg=" + ret.Msg)
	}
	return nil
}
func (bo *Bigone) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	if symbol == "XAUT" {
		symbol = "XAUt"
	}
	url := boSpotEndpoint + "/viewer/withdrawals"
	jwt := "Bearer " + bo.jwt()
	guid, _ := uuid.NewV4()
	payload := `{"symbol":"` + symbol + `"` +
		`,"amount":"` + qty.String() + `"` +
		`,"guid":"` + guid.String() + `"` +
		`,"target_address":"` + addr + `"` +
		`,"memo":"` + memo + `"` +
		`,"gateway_name":"` + bo.fromStdChainName(chain) + `"` +
		`}`
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	_, resp, err := bo.Post(url, []byte(payload), boApiDeadline, header)
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Id   int64  `json:"id"`
			Txid string `json:"txid,omitempty"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " withdrawals fail! msg=" + ret.Msg)
	}
	wr := &WithdrawReturn{
		WId:    strconv.FormatInt(ret.Data.Id, 10),
		Symbol: symbol,
		Txid:   ret.Data.Txid,
	}
	return wr, nil
}
func (bo *Bigone) CancelWithdrawal(wid string) error {
	url := boSpotEndpoint + "/viewer/withdrawals/" + wid + "/cancel"
	jwt := "Bearer " + bo.jwt()
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": jwt,
	}
	_, resp, err := bo.Post(url, nil, boApiDeadline, header)
	if err != nil {
		return errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(bo.Name() + " cancel withdrawals fail! msg=" + ret.Msg)
	}

	return nil
}
func (bo *Bigone) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	url := boSpotEndpoint + "/viewer/withdrawals"
	if symbol != "" {
		url += "?asset_symbol=" + symbol
	}
	jwt := "Bearer " + bo.jwt()
	_, resp, err := bo.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data []struct {
			Id       int64           `json:"id"`
			Symbol   string          `json:"asset_symbol"`
			Qty      decimal.Decimal `json:"amount"`
			Fee      decimal.Decimal `json:"fee"`
			Txid     string          `json:"txid"`
			Status   string          `json:"state"`
			CTime    string          `json:"inserted_at"`
			DoneTime string          `json:"completed_at"`
			UTime    string          `json:"updated_at"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	res := make([]WithdrawResult, 0, len(ret.Data))
	for i := range ret.Data {
		doneTime, _ := time.Parse(time.RFC3339, ret.Data[i].DoneTime)
		a := WithdrawResult{
			WId:      strconv.FormatInt(ret.Data[i].Id, 10),
			Symbol:   ret.Data[i].Symbol,
			Txid:     ret.Data[i].Txid,
			Status:   bo.toStdWithdrawStatus(ret.Data[i].Status),
			Qty:      ret.Data[i].Qty,
			Fee:      ret.Data[i].Fee,
			DoneTime: doneTime.Unix(),
		}
		if ret.Data[i].DoneTime == "" {
			a.DoneTime = 0
		}
		res = append(res, a)
	}
	return res, nil
}
func (bo *Bigone) FundingGetAllAssets() (map[string]*FundingAsset, error) {
	url := boSpotEndpoint + "/viewer/fund/accounts"
	jwt := "Bearer " + bo.jwt()
	_, resp, err := bo.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data []struct {
			Symbol  string          `json:"asset_symbol"`
			Balance decimal.Decimal `json:"balance"`
			Locked  decimal.Decimal `json:"locked_balance"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " resp fail! msg=" + ret.Msg)
	}
	assetsMap := make(map[string]*FundingAsset)
	for _, v := range ret.Data {
		if v.Balance.IsZero() {
			continue
		}
		assetsMap[v.Symbol] = &FundingAsset{
			Symbol: v.Symbol,
			Avail:  v.Balance.Sub(v.Locked),
			Locked: v.Locked,
			Total:  v.Balance,
		}
	}
	return assetsMap, nil
}
func (bo *Bigone) FundingGetAsset(symbol string) (FundingAsset, error) {
	url := boSpotEndpoint + "/viewer/fund/accounts/" + symbol
	jwt := "Bearer " + bo.jwt()
	var fa FundingAsset
	_, resp, err := bo.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return fa, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data struct {
			Symbol  string          `json:"asset_symbol"`
			Balance decimal.Decimal `json:"balance"`
			Locked  decimal.Decimal `json:"locked_balance"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return fa, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return fa, errors.New(bo.Name() + " get funding asset fail! msg=" + ret.Msg)
	}
	return FundingAsset{
		Symbol: ret.Data.Symbol,
		Avail:  ret.Data.Balance,
		Locked: ret.Data.Locked,
		Total:  ret.Data.Balance.Add(ret.Data.Locked),
	}, nil
}
func (bo *Bigone) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	url := boSpotEndpoint + "/viewer/assets/" + symbol + "/address"
	jwt := "Bearer " + bo.jwt()
	_, resp, err := bo.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code int    `json:"code,omitempty"`
		Msg  string `json:"message,omitempty"`
		Data []struct {
			Chain string `json:"chain"`
			Addr  string `json:"value"`
			Memo  string `json:"memo"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " get deposit addr fail! msg=" + ret.Msg)
	}
	daL := make([]DepositAddress, 0, len(ret.Data))
	for i := range ret.Data {
		if network != "" && ret.Data[i].Chain != network {
			continue
		}
		daL = append(daL, DepositAddress{
			Network: ret.Data[i].Chain,
			Addr:    ret.Data[i].Addr,
			Memo:    ret.Data[i].Memo,
		})
	}
	return daL, nil
}
func (bo *Bigone) GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error) {
	url := boSpotEndpoint + "/assets"
	_, resp, err := bo.Get(url, boApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bo.Name() + " net error! " + err.Error())
	}

	recv := struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data []struct {
			Symbol            string `json:"symbol"`
			IsTransferEnabled bool   `json:"is_transfer_enabled"`
			TransferScale     int32  `json:"transfer_scale"`
			BindNetworks      []struct {
				Network             string          `json:"gateway_name"`
				IsDepositEnabled    bool            `json:"is_deposit_enabled"`
				IsWithdrawalEnabled bool            `json:"is_withdrawal_enabled"`
				WithdrawScale       int32           `json:"withdrawal_scale"`
				WithdrawFee         decimal.Decimal `json:"withdrawal_fee"`
				MinWithdrawalAmount decimal.Decimal `json:"min_withdrawal_amount"`
				MinDepositAmount    decimal.Decimal `json:"min_deposit_amount"`
			} `json:"binding_gateways"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != 0 {
		return nil, errors.New(recv.Msg)
	}
	waiMap := make(map[string]*WalletAssetInfo)
	for _, v := range recv.Data {
		if len(v.BindNetworks) == 0 {
			continue
		}
		wai := WalletAssetInfo{
			Symbol:            v.Symbol,
			IsTransferEnabled: v.IsTransferEnabled,
			TransferScale:     v.TransferScale,
			BindNetworks:      make(map[string]*WalletAssetBindNetworkInfo),
		}
		for _, vv := range v.BindNetworks {
			wbni := WalletAssetBindNetworkInfo{
				IsWithdrawalEnabled: vv.IsWithdrawalEnabled,
				IsDepositEnabled:    vv.IsDepositEnabled,
				WithdrawScale:       vv.WithdrawScale,
				WithdrawFee:         vv.WithdrawFee,
				MinWithdrawalAmount: vv.MinWithdrawalAmount,
				MinDepositAmount:    vv.MinDepositAmount,
			}
			wai.BindNetworks[vv.Network] = &wbni
		}
		waiMap[v.Symbol] = &wai
	}
	return waiMap, nil
}
