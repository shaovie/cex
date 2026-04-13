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
	_, resp, err := ihttp.Post(url, []byte(payload), boApiDeadline, header)
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
	_, resp, err := ihttp.Post(url, []byte(payload), boApiDeadline, header)
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
	_, resp, err := ihttp.Post(url, nil, boApiDeadline, header)
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
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
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
		res = append(res, a)
	}
	return res, nil
}
func (bo *Bigone) FundingGetAsset(symbol string) (FundingAsset, error) {
	url := boSpotEndpoint + "/viewer/fund/accounts/" + symbol
	jwt := "Bearer " + bo.jwt()
	var fa FundingAsset
	_, resp, err := ihttp.Get(url, boApiDeadline, map[string]string{"Authorization": jwt})
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
