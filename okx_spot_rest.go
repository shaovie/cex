package cex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mailru/easyjson"
	"github.com/shopspring/decimal"

	"github.com/shaovie/gutils/ihttp"
)

func (ok *Okx) SpotSupported() bool {
	return true
}
func (ok *Okx) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	url := okUniEndpoint + "/api/v5/public/instruments?instType=SPOT"
	retCode, resp, err := ihttp.Get(url, okApiDeadline, nil)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}

	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			InstId      string `json:"instId"`
			Base        string `json:"baseCcy"`
			Quote       string `json:"quoteCcy"`
			State       string `json:"state"`
			PriceTickSz string `json:"tickSz"`
			QtyStep     string `json:"lotSz"`
			MinOrderQty string `json:"minSz"`
			MaxOrderQty string `json:"maxLmtSz"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		return nil, errors.New("resp error: " + ret.Msg)
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tokxSpotSymbolMap := make(map[string]string)
	for _, pair := range ret.Data {
		if pair.State != "live" { // open for trading
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:   pair.Base + pair.Quote,
			Base:     pair.Base,
			Quote:    pair.Quote,
			MaxPrice: decimal.NewFromFloat(999999999.99),
			Time:     now,
		}
		ep.PriceTickSize, _ = decimal.NewFromString(pair.PriceTickSz)
		ep.QtyStep, _ = decimal.NewFromString(pair.QtyStep)
		ep.MinOrderQty, _ = decimal.NewFromString(pair.MinOrderQty)
		ep.MaxOrderQty, _ = decimal.NewFromString(pair.MaxOrderQty)
		ep.MinPrice = ep.PriceTickSize
		all[ep.Symbol] = ep
		tokxSpotSymbolMap[ep.Symbol] = pair.InstId
	}

	okxSpotSymbolMapMtx.Lock()
	okxSpotSymbolMap = tokxSpotSymbolMap
	okxSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (ok *Okx) SpotGetAll24hTicker() (map[string]Spot24hTicker, error) {
	path := "/api/v5/market/tickers"
	url := okUniEndpoint + path + "?instType=SPOT"
	retCode, resp, err := ihttp.Get(url, okApiDeadline, nil)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	tickers := Okx24hTickers{}
	err = easyjson.Unmarshal(resp, &tickers)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal error! " + err.Error())
	}
	if len(tickers.Data) == 0 {
		return nil, errors.New(ok.Name() + " resp empty")
	}
	allTk := make(map[string]Spot24hTicker, len(tickers.Data))
	for _, tk := range tickers.Data {
		firstDash := strings.Index(tk.Symbol, "-")
		if firstDash == -1 {
			continue
		}
		sym := tk.Symbol[:firstDash] + tk.Symbol[firstDash+1:]
		v := Spot24hTicker{
			Symbol:      sym,
			LastPrice:   tk.Last,
			Volume:      tk.Volume,
			QuoteVolume: tk.QuoteVolume,
		}
		allTk[v.Symbol] = v
	}
	return allTk, nil
}
func (ok *Okx) SpotPlaceOrder(symbol, clientId string, /*BTCUSDT*/
	price, qty decimal.Decimal, side, timeInForce, orderType string) (string, error) {

	if timeInForce == "IOC" || timeInForce == "FOK" {
		orderType = timeInForce
	}
	symbolS := ok.getSpotSymbol(symbol)
	payload := `{"instId":"` + symbolS + `"` +
		`,"sz":"` + qty.String() + `"`
	if clientId != "" {
		payload += `,"clOrdId":"` + clientId + `"`
	}
	payload += "" +
		`,"px":"` + price.String() + `"` +
		`,"side":"` + ok.fromStdSide(side) + `"` +
		`,"ordType":"` + ok.fromStdOrderType(orderType) + `"` + // LIMIT/MARKET/LIMIT_MAKER
		`,"tdMode":"cash"` +
		`}`
	path := "/api/v5/trade/order"
	headers := ok.buildHeaders("POST", path, payload)
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, []byte(payload), okApiDeadline, headers)
	if err != nil {
		return "", errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return "", errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode,omitempty"`
			SMsg    string `json:"sMsg,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return "", errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" {
		if len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return "", errors.New(ok.Name() + " fail! code=" + ret.Code + " msg=" + ret.Msg)
	}

	if len(ret.Data) == 0 {
		return "", errors.New(ok.Name() + " fail! response emtpy")
	}

	return ret.Data[0].OrderId, nil
}
func (ok *Okx) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	symbolS := ok.getSpotSymbol(symbol)
	path := "/api/v5/trade/order?instId=" + symbolS + "&ordId=" + orderId
	if orderId == "" && cltId != "" {
		path = "/api/v5/trade/order?instId=" + symbolS + "&clOrdId=" + cltId
	}
	headers := ok.buildHeaders("GET", path, "")
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Get(url, okApiDeadline, headers)
	if err != nil {
		return nil, errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return nil, errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			SCode       string          `json:"sCode,omitempty"`
			SMsg        string          `json:"sMsg,omitempty"`
			Symbol      string          `json:"instId"`
			OrderId     string          `json:"ordId"`
			ClientId    string          `json:"clOrdId"`
			AvgPrice    string          `json:"avgPx,omitempty"`
			Price       string          `json:"px,omitempty"`
			Qty         decimal.Decimal `json:"sz"`
			ExecutedQty string          `json:"accFillSz"`
			Status      string          `json:"state"`
			Type        string          `json:"ordType"`
			Side        string          `json:"side"`
			FeeCoin     string          `json:"feeCcy,omitempty"`
			FeeQty      string          `json:"fee,omitempty"`
			Time        string          `json:"cTime"`
			UTime       string          `json:"uTime,omitempty"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return nil, errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" || len(ret.Data) == 0 {
		if ret.Code != "0" && len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return nil, errors.New(ok.Name() + " resp err! " + ret.Msg)
	}

	dt := ret.Data[0]
	ctime, _ := strconv.ParseInt(dt.Time, 10, 64)
	utime, _ := strconv.ParseInt(dt.UTime, 10, 64)
	avgPrice, _ := decimal.NewFromString(dt.AvgPrice)
	so := &SpotOrder{
		Symbol:   symbol,
		OrderId:  dt.OrderId,
		ClientId: dt.ClientId,
		Qty:      dt.Qty,
		Status:   ok.toStdOrderStatus(dt.Status),
		Type:     ok.toStdOrderType(dt.Type),
		Side:     ok.toStdSide(dt.Side),
		FeeAsset: dt.FeeCoin,
		CTime:    ctime,
		UTime:    utime,
	}
	so.Price, _ = decimal.NewFromString(dt.Price)
	so.FilledQty, _ = decimal.NewFromString(dt.ExecutedQty)
	so.FilledAmt = so.FilledQty.Mul(avgPrice)
	so.FeeQty, _ = decimal.NewFromString(dt.FeeQty)
	return so, nil
}
func (ok *Okx) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	symbolS := ok.getSpotSymbol(symbol)
	path := "/api/v5/trade/cancel-order"
	payload := `{"instId":"` + symbolS + `","ordId":"` + orderId + `"}`
	if orderId == "" && cltId != "" {
		payload = `{"instId":"` + symbolS + `","clOrdId":"` + cltId + `"}`
	}
	headers := ok.buildHeaders("POST", path, payload)
	url := okUniEndpoint + path
	retCode, resp, err := ihttp.Post(url, []byte(payload), okApiDeadline, headers)
	if err != nil {
		return errors.New(ok.Name() + " net error! " + err.Error())
	}
	if retCode != 200 {
		return errors.New(ok.Name() + " http code " + fmt.Sprintf("%d", retCode))
	}
	ret := struct {
		Code string `json:"code,omitempty"`
		Msg  string `json:"msg,omitempty"`
		Data []struct {
			OrderId string `json:"ordId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data,omitempty"`
	}{}
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return errors.New(ok.Name() + " unmarshal fail! " + err.Error())
	}
	if ret.Code != "0" || len(ret.Data) == 0 {
		if ret.Code != "0" && len(ret.Data) > 0 {
			ret.Code = ret.Data[0].SCode
			ret.Msg = ret.Data[0].SMsg
		}
		return errors.New(ok.Name() + " resp error! " + ret.Msg)
	}

	dt := ret.Data[0]
	if dt.SCode != "0" {
		return errors.New(ok.Name() + " resp error! " + dt.SMsg)
	}
	return nil
}
