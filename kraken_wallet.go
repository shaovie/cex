package cex

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/shaovie/gutils/ihttp"
	"github.com/shopspring/decimal"
)

func (kk *Kraken) Transfer(symbol, from, to, typ, subAccount string, qty decimal.Decimal) error {
	return nil
}
func (kk *Kraken) Withdrawal(symbol, addr, memo, chain string, qty decimal.Decimal) (*WithdrawReturn, error) {
	path := "/0/private/Withdraw"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("asset", symbol)
	if kk.isXStocksSymbol(symbol + "USD") {
		values.Set("aclass", "tokenized_asset")
	}
	values.Set("key", memo)
	values.Set("address", addr)
	values.Set("amount", qty.String())

	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error"`
		Result struct {
			Id string `json:"refid"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return nil, errors.New(kk.Name() + " resp error! " + recv.Error[0])
	}

	wr := &WithdrawReturn{
		WId:    recv.Result.Id,
		Symbol: symbol,
	}
	return wr, nil
}
func (kk *Kraken) GetWithdrawalHistory(symbol string) ([]WithdrawResult, error) {
	path := "/0/private/WithdrawStatus"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("asset", symbol)
	if kk.isXStocksSymbol(symbol + "USD") {
		values.Set("aclass", "tokenized_asset")
	}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}

	ret := struct {
		Error  []string `json:"error"`
		Result []struct {
			Id     string          `json:"refid"`
			Symbol string          `json:"asset"`
			Txid   string          `json:"txid"`
			Status string          `json:"status"`
			Qty    decimal.Decimal `json:"amount"`
			Fee    decimal.Decimal `json:"fee"`
			UTime  int64           `json:"time"`
		}
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	res := make([]WithdrawResult, 0, len(ret.Result))
	for i := range ret.Result {
		a := WithdrawResult{
			WId:      ret.Result[i].Id,
			Symbol:   ret.Result[i].Symbol,
			Txid:     ret.Result[i].Txid,
			Status:   kk.toStdWithdrawStatus(ret.Result[i].Status),
			Qty:      ret.Result[i].Qty,
			Fee:      ret.Result[i].Fee,
			DoneTime: ret.Result[i].UTime,
		}
		res = append(res, a)
	}
	return res, nil
}
func (kk *Kraken) getDepositMethods(symbol string) ([]string, error) {
	path := "/0/private/DepositMethods"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("asset", symbol)
	if kk.isXStocksSymbol(symbol + "USD") {
		values.Set("aclass", "tokenized_asset")
	}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error"`
		Result []struct {
			Method string `json:"method"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return nil, errors.New(kk.Name() + " resp error! " + recv.Error[0])
	}

	if len(recv.Result) == 0 {
		return nil, errors.New(kk.Name() + " get methods resp empty!")
	}
	retL := make([]string, 0, len(recv.Result))
	for i := range recv.Result {
		retL = append(retL, recv.Result[i].Method)
	}

	return retL, nil
}
func (kk *Kraken) GetDepositAddress(symbol, network string) ([]DepositAddress, error) {
	methods, err := kk.getDepositMethods(symbol)
	// {"method":"AAPLx","limit":false,"fee":"0.00000000","gen-address":true,"minimum":"0.00501332"}
	// {"method":"AAPLx - Ethereum","limit":false,"gen-address":true,"minimum":"0.00461225"}
	// 这一步不完全正确，有时候这个SB交易所会返回空的Method
	if err != nil {
		return nil, err
	}
	path := "/0/private/DepositAddresses"
	link := kkSpotEndpoint + path
	values := url.Values{}
	values.Set("asset", symbol)
	for _, m := range methods {
		if strings.Index(m, network) != -1 {
			values.Set("method", m)
			break
		}
	}
	if kk.isXStocksSymbol(symbol + "USD") {
		values.Set("aclass", "tokenized_asset")
	}
	headers, params := kk.buildHeaders(path, values)
	_, resp, err := ihttp.Post(link, []byte(params), kkApiDeadline, headers)
	if err != nil {
		return nil, errors.New(kk.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Error  []string `json:"error"`
		Result []struct {
			Addr string `json:"address"`
			Memo string `json:"tag"`
		}
	}{}
	err = json.Unmarshal(resp, &recv)
	if err != nil {
		return nil, errors.New(kk.Name() + " unmarshal fail! " + err.Error())
	}
	if len(recv.Error) > 0 {
		return nil, errors.New(kk.Name() + " resp error! " + recv.Error[0])
	}

	if len(recv.Result) == 0 {
		return nil, errors.New(kk.Name() + " resp empty!")
	}
	daL := make([]DepositAddress, 0, len(recv.Result))
	for i := range recv.Result {
		daL = append(daL, DepositAddress{
			Network: network,
			Addr:    recv.Result[i].Addr,
			Memo:    recv.Result[i].Memo,
		})
	}

	return daL, nil
}
