package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// fromStdAccountType 转from/to: FUNDING,SPOT,UM_FUTURE,CM_FUTURE,UNIFIED,MARGIN
// 统一账户(UTA)下现货/杠杆共用一个账户, 合约不需要划转
func (kc *Kucoin) fromStdAccountType(typ string) string {
	if typ == "SPOT" || typ == "UNIFIED" || typ == "MARGIN" {
		return "UNIFIED"
	} else if typ == "FUNDING" {
		return "FUNDING"
	}
	return ""
}
func (kc *Kucoin) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	fromType := kc.fromStdAccountType(from)
	toType := kc.fromStdAccountType(to)
	if fromType == "" || toType == "" {
		return errors.New(kc.Name() + " transfer not support! from=" + from + " to=" + to)
	}
	req := struct {
		ClientOid       string `json:"clientOid"`
		Currency        string `json:"currency"`
		Amount          string `json:"amount"`
		Type            string `json:"type"`
		FromAccountType string `json:"fromAccountType"`
		ToAccountType   string `json:"toAccountType"`
	}{
		ClientOid:       "WC" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Currency:        symbol,
		Amount:          qty.String(),
		Type:            "0", // INTERNAL 账户间划转
		FromAccountType: fromType,
		ToAccountType:   toType,
	}
	payload, _ := json.Marshal(req)
	path := "/api/ua/v1/account/transfer"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Post(link, payload, kcApiDeadline,
		kc.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(kc.Name() + " transfer unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return errors.New(kc.Name() + " transfer fail! " + ret.Code + " " + ret.Msg)
	}
	return nil
}

// FundingGetAllAssets 资金账户资产
func (kc *Kucoin) FundingGetAllAssets() (map[string]*FundingAsset, error) {
	assetsMap, err := kc.spotGetAccountAssets("FUNDING")
	if err != nil {
		return nil, err
	}
	fundingMap := make(map[string]*FundingAsset, len(assetsMap))
	for symbol, v := range assetsMap {
		fundingMap[symbol] = &FundingAsset{
			Symbol: v.Symbol,
			Avail:  v.Avail,
			Locked: v.Locked,
			Total:  v.Total,
		}
	}
	return fundingMap, nil
}
func (kc *Kucoin) FundingGetAsset(symbol string) (FundingAsset, error) {
	var fa FundingAsset
	assetsMap, err := kc.spotGetAccountAssets("FUNDING")
	if err != nil {
		return fa, err
	}
	if v := assetsMap[symbol]; v != nil {
		fa.Symbol = v.Symbol
		fa.Avail = v.Avail
		fa.Locked = v.Locked
		fa.Total = v.Total
	}
	return fa, nil
}
func (kc *Kucoin) spotGetAccountAssets(accountType string) (map[string]*SpotAsset, error) {
	path := "/api/ua/v1/account/balance"
	params := "accountType=" + accountType
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Accounts []struct {
				Currencies []KucoinCurrency `json:"currencies"`
			} `json:"accounts"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if recv.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + recv.Code + " " + recv.Msg)
	}
	assetsMap := make(map[string]*SpotAsset)
	for _, acc := range recv.Data.Accounts {
		for _, v := range acc.Currencies {
			if v.Available.IsZero() && v.Balance.IsZero() && v.Hold.IsZero() {
				continue
			}
			assetsMap[v.Currency] = &SpotAsset{
				Symbol: v.Currency,
				Avail:  v.Available,
				Locked: v.Hold,
				Total:  v.Balance,
			}
		}
	}
	return assetsMap, nil
}

func (kc *Kucoin) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	req := struct {
		Currency     string `json:"currency"`
		ToAddress    string `json:"toAddress"`
		Amount       string `json:"amount"`
		Memo         string `json:"memo,omitempty"`
		WithdrawType string `json:"withdrawType"`
		ChainId      string `json:"chainId,omitempty"`
		IsInner      bool   `json:"isInner"`
	}{
		Currency:     symbol,
		ToAddress:    addr,
		Amount:       qty.String(),
		Memo:         memo,
		WithdrawType: "ADDRESS",
		ChainId:      chain,
	}
	payload, _ := json.Marshal(req)
	path := "/api/ua/v1/withdrawal"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Post(link, payload, kcApiDeadline,
		kc.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			WithdrawalId string `json:"withdrawalId,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return nil, errors.New(kc.Name() + " withdrawal fail! " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data.WithdrawalId) == 0 {
		return nil, errors.New(kc.Name() + " withdrawalId is empty!")
	}

	wr := &WithdrawReturn{
		WId:    ret.Data.WithdrawalId,
		Symbol: symbol,
	}
	return wr, nil
}
func (kc *Kucoin) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	now := time.Now().UnixMilli()
	path := "/api/ua/v1/asset/withdrawal/history"
	params := "startAt=" + strconv.FormatInt(now-90*24*3600*1000, 10) +
		"&endAt=" + strconv.FormatInt(now, 10) + "&pageSize=50"
	if symbol != "" {
		params += "&currency=" + symbol
	}
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				Id         string          `json:"id,omitempty"`
				Currency   string          `json:"currency,omitempty"`
				Status     string          `json:"status,omitempty"`
				Amount     decimal.Decimal `json:"amount"`
				Fee        decimal.Decimal `json:"fee"`
				WalletTxId string          `json:"walletTxId,omitempty"`
				CreatedAt  int64           `json:"createdAt,omitempty"`
				UpdatedAt  int64           `json:"updatedAt,omitempty"`
			} `json:"items"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
	}

	res := make([]WithdrawResult, 0, len(ret.Data.Items))
	for _, v := range ret.Data.Items {
		a := WithdrawResult{
			WId:      v.Id,
			Symbol:   v.Currency,
			Status:   kc.toStdWithdrawStatus(v.Status),
			Txid:     v.WalletTxId,
			Qty:      v.Amount,
			Fee:      v.Fee,
			DoneTime: v.UpdatedAt / 1000, // second
		}
		res = append(res, a)
	}
	return res, nil
}
func (kc *Kucoin) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	path := "/api/ua/v1/asset/deposit/address"
	params := "currency=" + symbol
	if network != "" {
		params += "&chain=" + network
	}
	link := kcSpotEndpoint + path + "?" + params
	_, resp, err := kc.Get(link, kcApiDeadline, kc.buildHeaders("GET", path, "?"+params, ""))
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Address   string `json:"address,omitempty"`
			Memo      string `json:"memo,omitempty"`
			ChainId   string `json:"chainId,omitempty"`
			ChainName string `json:"chainName,omitempty"`
			Currency  string `json:"currency,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data) == 0 || len(ret.Data[0].Address) == 0 {
		return nil, errors.New(kc.Name() + " resp empty!")
	}

	daL := make([]DepositAddress, 0, len(ret.Data))
	for _, v := range ret.Data {
		daL = append(daL, DepositAddress{
			Network: v.ChainId,
			Addr:    v.Address,
			Memo:    v.Memo,
		})
	}
	return daL, nil
}

// GetWalletAllAssetInfo 币种与网络的充提配置, 公开接口
func (kc *Kucoin) GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error) {
	path := "/api/ua/v1/asset/currencies"
	link := kcSpotEndpoint + path
	_, resp, err := kc.Get(link, kcApiDeadline, nil)
	if err != nil {
		return nil, errors.New(kc.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Currency  string `json:"currency,omitempty"`
			Precision int32  `json:"precision,omitempty"`
			List      []struct {
				ChainId           string          `json:"chainId,omitempty"`
				ChainName         string          `json:"chainName,omitempty"`
				IsDepositEnabled  bool            `json:"isDepositEnabled,omitempty"`
				IsWithdrawEnabled bool            `json:"isWithdrawEnabled,omitempty"`
				WithdrawFeeRate   decimal.Decimal `json:"withdrawFeeRate"`
				MinWithdrawFee    decimal.Decimal `json:"minWithdrawFee"`
				MinWithdrawSize   decimal.Decimal `json:"minWithdrawSize"`
				MinDepositSize    decimal.Decimal `json:"minDepositSize"`
				WithdrawPrecision int32           `json:"withdrawPrecision,omitempty"`
				NeedTag           bool            `json:"needTag,omitempty"`
			} `json:"items,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(kc.Name() + " unmarshal error! " + err.Error())
	}
	if ret.Code != "200000" {
		return nil, errors.New(kc.Name() + " " + ret.Code + " " + ret.Msg)
	}

	waiMap := make(map[string]*WalletAssetInfo, len(ret.Data))
	for _, v := range ret.Data {
		if len(v.List) == 0 {
			continue
		}
		wai := WalletAssetInfo{
			Symbol:            v.Currency,
			IsTransferEnabled: true,
			TransferScale:     v.Precision,
			BindNetworks:      make(map[string]*WalletAssetBindNetworkInfo, len(v.List)),
		}
		for _, vv := range v.List {
			wbni := WalletAssetBindNetworkInfo{
				IsWithdrawalEnabled:     vv.IsWithdrawEnabled,
				IsDepositEnabled:        vv.IsDepositEnabled,
				WithdrawScale:           vv.WithdrawPrecision,
				WithdrawIntegerMultiple: PowOneTenth(int(vv.WithdrawPrecision)),
				WithdrawFee:             vv.MinWithdrawFee,
				MinWithdrawalAmount:     vv.MinWithdrawSize,
				MinDepositAmount:        vv.MinDepositSize,
			}
			wai.BindNetworks[vv.ChainId] = &wbni
		}
		waiMap[v.Currency] = &wai
	}
	return waiMap, nil
}
