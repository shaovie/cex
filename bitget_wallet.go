package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// fromStdTransferType 转from/to: FUNDING,SPOT,UM_FUTURE,CM_FUTURE,MARGIN
func (bg *Bitget) fromStdTransferType(typ string) string {
	if typ == "SPOT" {
		return "spot"
	} else if typ == "FUNDING" {
		return "p2p"
	} else if typ == "UM_FUTURE" {
		return "usdt_futures"
	} else if typ == "CM_FUTURE" {
		return "coin_futures"
	} else if typ == "MARGIN" {
		return "margin"
	}
	return ""
}
func (bg *Bitget) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	fromType := bg.fromStdTransferType(from)
	toType := bg.fromStdTransferType(to)
	if fromType == "" || toType == "" {
		return errors.New(bg.Name() + " transfer not support! from=" + from + " to=" + to)
	}
	req := struct {
		FromType  string `json:"fromType"`
		ToType    string `json:"toType"`
		Amount    string `json:"amount"`
		Coin      string `json:"coin"`
		ClientOid string `json:"clientOid,omitempty"`
	}{
		FromType: fromType,
		ToType:   toType,
		Amount:   qty.String(),
		Coin:     symbol,
	}
	payload, _ := json.Marshal(req)
	path := "/api/v3/account/transfer"
	link := bgEndpoint + path
	_, resp, err := bg.Post(link, payload, bgApiDeadline,
		bg.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			TransferId string `json:"transferId,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(bg.Name() + " transfer unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return errors.New(bg.Name() + " transfer fail! " + ret.Code + " " + ret.Msg)
	}
	return nil
}
func (bg *Bitget) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	req := struct {
		Coin         string `json:"coin"`
		TransferType string `json:"transferType"`
		Address      string `json:"address"`
		Chain        string `json:"chain,omitempty"`
		Tag          string `json:"tag,omitempty"`
		Size         string `json:"size"`
	}{
		Coin:         symbol,
		TransferType: "on_chain",
		Address:      addr,
		Chain:        chain,
		Tag:          memo,
		Size:         qty.String(),
	}
	payload, _ := json.Marshal(req)
	path := "/api/v3/account/withdrawal"
	link := bgEndpoint + path
	_, resp, err := bg.Post(link, payload, bgApiDeadline,
		bg.buildHeaders("POST", path, "", string(payload)))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			OrderId string `json:"orderId,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return nil, errors.New(bg.Name() + " withdrawal fail! " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data.OrderId) == 0 {
		return nil, errors.New(bg.Name() + " withdrawal orderId is empty!")
	}

	wr := &WithdrawReturn{
		WId:    ret.Data.OrderId,
		Symbol: symbol,
	}
	return wr, nil
}
func (bg *Bitget) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	path := "/api/v3/account/withdrawal-records"
	now := time.Now().UnixMilli()
	params := "startTime=" + strconv.FormatInt(now-90*24*3600*1000, 10) +
		"&endTime=" + strconv.FormatInt(now, 10) + "&limit=100"
	if symbol != "" {
		params += "&coin=" + symbol
	}
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId     string          `json:"orderId,omitempty"`
			RecordId    string          `json:"recordId,omitempty"` // 链上哈希
			Coin        string          `json:"coin,omitempty"`
			Chain       string          `json:"chain,omitempty"`
			Size        decimal.Decimal `json:"size"`
			Fee         decimal.Decimal `json:"fee"`
			Status      string          `json:"status,omitempty"`
			CreatedTime string          `json:"createdTime,omitempty"`
			UpdatedTime string          `json:"updatedTime,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}

	res := make([]WithdrawResult, 0, len(ret.Data))
	for _, v := range ret.Data {
		doneTime, _ := strconv.ParseInt(v.UpdatedTime, 10, 64)
		a := WithdrawResult{
			WId:      v.OrderId,
			Symbol:   v.Coin,
			Status:   bg.toStdWithdrawStatus(v.Status),
			Txid:     v.RecordId,
			Qty:      v.Size,
			Fee:      v.Fee,
			DoneTime: doneTime / 1000, // second
		}
		res = append(res, a)
	}
	return res, nil
}
func (bg *Bitget) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	path := "/api/v3/account/deposit-address"
	params := "coin=" + strings.ToUpper(symbol)
	if network != "" {
		params += "&chain=" + network
	}
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			Coin    string `json:"coin,omitempty"`
			Chain   string `json:"chain,omitempty"`
			Address string `json:"address,omitempty"`
			Tag     string `json:"tag,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}
	if len(ret.Data.Address) == 0 {
		return nil, errors.New(bg.Name() + " resp empty!")
	}

	daL := []DepositAddress{
		{
			Network: ret.Data.Chain,
			Addr:    ret.Data.Address,
			Memo:    ret.Data.Tag,
		},
	}
	return daL, nil
}
func (bg *Bitget) GetWithdrawalAddress(symbol string) ([]WithdrawalAddress, error) {
	path := "/api/v3/account/withdraw-address"
	params := "limit=10"
	if symbol != "" {
		params += "&coin=" + strings.ToUpper(symbol)
	}
	link := bgEndpoint + path + "?" + params
	_, resp, err := bg.Get(link, bgApiDeadline, bg.buildHeaders("GET", path, params, ""))
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data struct {
			AddressList []struct {
				Coin    string `json:"coin,omitempty"`
				Chain   string `json:"chain,omitempty"`
				Address string `json:"address,omitempty"`
				Memo    string `json:"memo,omitempty"`
				Label   string `json:"label,omitempty"`
			} `json:"addressList"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}

	daL := make([]WithdrawalAddress, 0, len(ret.Data.AddressList))
	for _, v := range ret.Data.AddressList {
		daL = append(daL, WithdrawalAddress{
			Symbol:  v.Coin,
			Network: v.Chain,
			Addr:    v.Address,
			Memo:    v.Memo,
		})
	}
	return daL, nil
}

// GetWalletAllAssetInfo 币种与网络的充提配置, 公共接口(与账户类型无关)
func (bg *Bitget) GetWalletAllAssetInfo() (map[string]*WalletAssetInfo, error) {
	path := "/api/v2/spot/public/coins"
	link := bgEndpoint + path
	_, resp, err := bg.Get(link, bgApiDeadline, nil)
	if err != nil {
		return nil, errors.New(bg.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			Coin     string `json:"coin,omitempty"`
			Transfer string `json:"transfer,omitempty"`
			Chains   []struct {
				Chain         string          `json:"chain,omitempty"`
				Rechargeable  string          `json:"rechargeable,omitempty"`
				Withdrawable  string          `json:"withdrawable,omitempty"`
				WithdrawFee   decimal.Decimal `json:"withdrawFee"`
				MinWithdraw   decimal.Decimal `json:"minWithdrawAmount"`
				MinDeposit    decimal.Decimal `json:"minDepositAmount"`
				WithdrawScale string          `json:"withdrawMinScale,omitempty"`
			} `json:"chains,omitempty"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bg.Name() + " unmarshal error! " + err.Error())
	}
	if ret.Code != "00000" {
		return nil, errors.New(bg.Name() + " " + ret.Code + " " + ret.Msg)
	}

	waiMap := make(map[string]*WalletAssetInfo, len(ret.Data))
	for _, v := range ret.Data {
		if len(v.Chains) == 0 {
			continue
		}
		wai := WalletAssetInfo{
			Symbol:            v.Coin,
			IsTransferEnabled: v.Transfer == "true",
			BindNetworks:      make(map[string]*WalletAssetBindNetworkInfo, len(v.Chains)),
		}
		for _, vv := range v.Chains {
			scale, _ := strconv.ParseInt(vv.WithdrawScale, 10, 32)
			wbni := WalletAssetBindNetworkInfo{
				IsWithdrawalEnabled:     vv.Withdrawable == "true",
				IsDepositEnabled:        vv.Rechargeable == "true",
				WithdrawScale:           int32(scale),
				WithdrawIntegerMultiple: PowOneTenth(int(scale)),
				WithdrawFee:             vv.WithdrawFee,
				MinWithdrawalAmount:     vv.MinWithdraw,
				MinDepositAmount:        vv.MinDeposit,
			}
			wai.BindNetworks[vv.Chain] = &wbni
		}
		waiMap[v.Coin] = &wai
	}
	return waiMap, nil
}
