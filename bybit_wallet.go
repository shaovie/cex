package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (bb *Bybit) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	if !qty.IsPositive() {
		return errors.New(bb.Name() + " transfer qty <= 0. =" + qty.String())
	}
	if from == "FUNDING" {
		from = "FUND"
	}
	if to == "FUNDING" {
		to = "FUND"
	}
	guid, _ := uuid.NewV4()
	payload := `{"transferId":"` + guid.String() + `",` +
		`"coin":"` + symbol + `",` +
		`"amount":"` + qty.String() + `",` +
		`"fromAccountType":"` + from + `",` +
		`"toAccountType":"` + to + `"}`
	path := "/v5/asset/transfer/inter-transfer"
	headers := bb.buildHeaders("", payload)
	url := bbUniEndpoint + path
	_, resp, err := ihttp.Get(url, bbApiDeadline, headers)
	if err != nil {
		return errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			TransferId string `json:"transferId"`
			Status     string `json:"status"`
		} `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return errors.New(bb.Name() + " transfer " + qty.String() + " fail! " + ret.Msg)
	}

	return nil
}
func (bb *Bybit) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	path := "/v5/asset/withdraw/create"
	url := bbUniEndpoint + path
	payload := `{"coin":"` + symbol + `"` +
		`,"amount":"` + qty.String() + `"` +
		`,"timestamp":` + strconv.FormatInt(time.Now().UnixMilli(), 10) +
		`,"address":"` + addr + `"` +
		`,"accountType":"FUND"` +
		`,"feeType":1` + // 輸入金額不是實際收到的金額, 系統將會自動計算所需的手續費
		`,"tag":"` + memo + `"` +
		`,"chain":"` + chain + `"` +
		`}`
	headers := bb.buildHeaders("", payload)
	_, resp, err := ihttp.Post(url, []byte(payload), bbApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			Id string `json:"id"`
		} `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bb.Name() + " withdrawal fail! msg=" + ret.Msg)
	}

	wr := &WithdrawReturn{
		Symbol: symbol,
		WId:    ret.Result.Id,
	}
	return wr, nil
}
func (bb *Bybit) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	path := "/v5/asset/withdraw/query-record"
	params := "coin=" + symbol
	headers := bb.buildHeaders(params, "")
	url := bbUniEndpoint + path + "?" + params
	_, resp, err := ihttp.Get(url, bbApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			Rows []struct {
				Id       string          `json:"withdrawId"`
				Symbol   string          `json:"coin"`
				Qty      decimal.Decimal `json:"amount"`
				Fee      decimal.Decimal `json:"withdrawFee"`
				Status   string          `json:"status"`
				Txid     string          `json:"txId"`
				DoneTime string          `json:"updateTime"` // msec
			} `json:"rows"`
		} `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}

	res := make([]WithdrawResult, 0, 4)
	for _, v2 := range ret.Result.Rows {
		dtime, _ := strconv.ParseInt(v2.DoneTime, 10, 64)
		a := WithdrawResult{
			WId:      v2.Id,
			Symbol:   v2.Symbol,
			Status:   bb.toStdWithdrawStatus(v2.Status),
			Qty:      v2.Qty,
			Txid:     v2.Txid,
			Fee:      v2.Fee,
			DoneTime: dtime / 1000,
		}
		res = append(res, a)
	}
	return res, nil
}
func (bb *Bybit) FundingGetAsset(symbol string) (FundingAsset, error) {
	path := "/v5/asset/transfer/query-account-coin-balance"
	params := "accountType=FUND&coin=" + symbol
	headers := bb.buildHeaders(params, "")
	url := bbUniEndpoint + path + "?" + params
	_, resp, err := ihttp.Get(url, bbApiDeadline, headers)
	if err != nil {
		return FundingAsset{}, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			AccountType string `json:"accountType"`
			Balance     struct {
				Symbol string          `json:"coin"`
				Total  decimal.Decimal `json:"walletBalance"`
				Avail  decimal.Decimal `json:"transferBalance"`
			} `json:"balance"`
		} `json:"result"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return FundingAsset{}, errors.New(bb.Name() + " unmarshal fail! " + err.Error())
	}

	return FundingAsset{
		Symbol: ret.Result.Balance.Symbol,
		Avail:  ret.Result.Balance.Avail,
		Total:  ret.Result.Balance.Total,
	}, nil
}
func (bb *Bybit) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	path := "/v5/asset/deposit/query-address"
	params := "coin=" + symbol
	if network != "" {
		params += "&chainType=" + network
	}
	headers := bb.buildHeaders(params, "")
	url := bbUniEndpoint + path + "?" + params
	_, resp, err := ihttp.Get(url, bbApiDeadline, headers)
	if err != nil {
		return nil, errors.New(bb.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Code   int    `json:"retCode,omitempty"`
		Msg    string `json:"retMsg,omitempty"`
		Result struct {
			Symbol       string `json:"coin"`
			BindNetworks []struct {
				Network string `json:"chain"`
				Addr    string `json:"addressDeposit"`
				Memo    string `json:"tagDeposit"`
			} `json:"chains"`
		} `json:"result"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(bb.Name() + " unmarshal error! " + err.Error())
	}
	daL := make([]DepositAddress, 0, 4)
	for _, v := range ret.Result.BindNetworks {
		daL = append(daL, DepositAddress{
			Network: v.Network,
			Addr:    v.Addr,
			Memo:    v.Memo,
		})
	}
	return daL, nil
}
