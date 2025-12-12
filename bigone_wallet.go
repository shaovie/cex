package cex

import (
	"encoding/json"
	"errors"

	"github.com/gofrs/uuid"
	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

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
			Status string          `json:"state,omitempty"`
			Txid   string          `json:"txid,omitempty"`
			Qty    decimal.Decimal `json:"amount,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(bo.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != 0 {
		return nil, errors.New(bo.Name() + " withdrawals fail! msg=" + ret.Msg)
	}

	wr := &WithdrawReturn{
		WId:    guid.String(),
		Symbol: symbol,
		Qty:    qty,
		Status: bo.toStdWithdrawStatus(ret.Data.Status),
		Txid:   ret.Data.Txid,
		Addr:   addr,
		Chain:  chain,
		Memo:   memo,
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
