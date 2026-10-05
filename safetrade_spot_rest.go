package cex

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (sa *Safetrade) SpotSupported() bool {
	return true
}
func (sa *Safetrade) SpotLoadAllPairRule() (map[string]*SpotExchangePairRule, error) {
	link := stEndpoint + "/trade/public/markets"
	_, resp, err := sa.Get(link, stApiDeadline, stHttpHeaders)
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}

	ret := []struct {
		Id              string          `json:"id"` // 交易所内部symbol, 如 btcusdt
		Base            string          `json:"base_unit"`
		Quote           string          `json:"quote_unit"`
		State           string          `json:"state"`
		AmountPrecision int32           `json:"amount_precision"`
		PricePrecision  int32           `json:"price_precision"`
		MinPrice        decimal.Decimal `json:"min_price"`
		MaxPrice        decimal.Decimal `json:"max_price"`
		MinAmount       decimal.Decimal `json:"min_amount"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	all := make(map[string]*SpotExchangePairRule)
	now := time.Now().Unix()
	tSymbolMap := make(map[string]string)
	for _, m := range ret {
		if m.State != "enabled" { // open for trading
			continue
		}
		ep := &SpotExchangePairRule{
			Symbol:        strings.ToUpper(m.Id),
			Base:          strings.ToUpper(m.Base),
			Quote:         strings.ToUpper(m.Quote),
			Status:        "online",
			PriceTickSize: PowOneTenth(int(m.PricePrecision)),
			QtyStep:       PowOneTenth(int(m.AmountPrecision)),
			MinPrice:      m.MinPrice,
			MaxPrice:      m.MaxPrice,
			MinOrderQty:   m.MinAmount,
			MaxOrderQty:   decimal.NewFromFloat(999999999999.99),
			Time:          now,
		}
		all[ep.Symbol] = ep
		tSymbolMap[ep.Symbol] = m.Id
	}

	stSpotSymbolMapMtx.Lock()
	stSpotSymbolMap = tSymbolMap
	stSpotSymbolMapMtx.Unlock()
	return all, nil
}
func (sa *Safetrade) SpotGetBBO(symbol string) (BestBidAsk, error) {
	id := sa.getSpotSymbol(symbol)
	if id == "" {
		return BestBidAsk{}, errors.New(sa.Name() + " unknown symbol! " + symbol)
	}
	link := stEndpoint + "/trade/public/markets/" + id + "/depth?limit=1"
	_, resp, err := sa.Get(link, stApiDeadline, stHttpHeaders)
	if err != nil {
		return BestBidAsk{}, errors.New(sa.Name() + " net error! " + err.Error())
	}
	recv := struct {
		Asks [][2]decimal.Decimal `json:"asks"`
		Bids [][2]decimal.Decimal `json:"bids"`
	}{}
	if err = json.Unmarshal(resp, &recv); err != nil {
		return BestBidAsk{}, errors.New(sa.Name() + " unmarshal error! " + err.Error())
	}
	if len(recv.Bids) > 0 && len(recv.Asks) > 0 {
		return BestBidAsk{
			Symbol:   symbol,
			BidPrice: recv.Bids[0][0],
			BidQty:   recv.Bids[0][1],
			AskPrice: recv.Asks[0][0],
			AskQty:   recv.Asks[0][1],
		}, nil
	}
	return BestBidAsk{}, errors.New(sa.Name() + " resp empty!")
}
func (sa *Safetrade) SpotGetAllAssets() (map[string]*SpotAsset, error) {
	link := stEndpoint + "/trade/account/balances/spot"
	_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, sa.handleExceptionResp("SpotGetAllAssets", resp)
	}
	ret := []struct {
		Symbol string          `json:"currency"`
		Avail  decimal.Decimal `json:"balance"`
		Locked decimal.Decimal `json:"locked"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal error! " + err.Error())
	}
	assetsMap := make(map[string]*SpotAsset, len(ret))
	for _, v := range ret {
		if v.Avail.IsZero() && v.Locked.IsZero() {
			continue
		}
		symbol := strings.ToUpper(v.Symbol) // 该接口返回小写币种
		assetsMap[symbol] = &SpotAsset{
			Symbol: symbol,
			Avail:  v.Avail,
			Locked: v.Locked,
			Total:  v.Avail.Add(v.Locked),
		}
	}
	return assetsMap, nil
}
func (sa *Safetrade) SpotPlaceOrder(symbol, cltId string, /*BTCUSDT*/
	price, amt, qty decimal.Decimal,
	side, timeInForce, orderType string, postOnly bool) (string, error) {
	id := sa.getSpotSymbol(symbol)
	if id == "" {
		return "", errors.New(sa.Name() + " unknown symbol! " + symbol)
	}
	// 该交易所不支持client order id, cltId 忽略
	req := struct {
		Market string `json:"market"`
		Side   string `json:"side"`
		Type   string `json:"type"`
		Price  string `json:"price,omitempty"`
		Amount string `json:"amount,omitempty"`
		Total  string `json:"total,omitempty"`
	}{
		Market: id,
		Side:   sa.fromStdSide(side),
		Type:   sa.fromStdOrderType(orderType),
	}
	if orderType == "LIMIT" {
		req.Price = price.String()
		req.Amount = qty.String()
	} else if orderType == "MARKET" {
		if amt.IsPositive() { // amt为计价币金额
			req.Total = amt.String()
		} else {
			req.Amount = qty.String()
		}
	} else {
		return "", errors.New(sa.Name() + " not support order type! " + orderType)
	}
	payload, _ := json.Marshal(req)
	link := stEndpoint + "/trade/market/orders"
	_, resp, err := sa.Post(link, payload, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return "", errors.New(sa.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Errors []string `json:"errors,omitempty"`
		Id     int64    `json:"id,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return "", errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Errors) > 0 {
		return "", errors.New(sa.Name() + ": " + ret.Errors[0])
	}
	if ret.Id == 0 {
		return "", errors.New(sa.Name() + " order id is empty!")
	}
	return strconv.FormatInt(ret.Id, 10), nil
}
func (sa *Safetrade) SpotCancelOrder(symbol string /*BTCUSDT*/, orderId, cltId string) error {
	if orderId == "" {
		if cltId != "" {
			return errors.New(sa.Name() + " not support cltId!")
		}
		return errors.New(sa.Name() + " orderId or cltId empty!")
	}
	link := stEndpoint + "/trade/market/orders/" + orderId + "/cancel"
	_, resp, err := sa.Post(link, nil, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return errors.New(sa.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Errors []string `json:"errors,omitempty"`
		State  string   `json:"state,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Errors) > 0 {
		return errors.New(sa.Name() + ": " + ret.Errors[0])
	}
	if ret.State != "cancel" {
		return errors.New(sa.Name() + " cancel failed! state now: " + ret.State)
	}
	return nil
}
func (sa *Safetrade) SpotGetOrder(symbol, orderId, cltId string) (*SpotOrder, error) {
	if orderId == "" {
		if cltId != "" {
			return nil, errors.New(sa.Name() + " not support cltId!")
		}
		return nil, errors.New(sa.Name() + " orderId or cltId empty!")
	}
	link := stEndpoint + "/trade/market/orders/" + orderId
	_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	ret := struct {
		Errors []string `json:"errors,omitempty"`

		Id           int64           `json:"id"`
		Market       string          `json:"market"`
		Side         string          `json:"side"`
		Type         string          `json:"type"`
		OrdType      string          `json:"ord_type"` // 文档未列出, 实际返回ord_type
		State        string          `json:"state"`
		Price        decimal.Decimal `json:"price"`
		AvgPrice     decimal.Decimal `json:"avg_price"` // 文档未列出
		OriginAmount decimal.Decimal `json:"origin_amount"`
		FilledAmount decimal.Decimal `json:"filled_amount"`
		CreatedAt    string          `json:"created_at,omitempty"`
		UpdatedAt    string          `json:"updated_at,omitempty"`
	}{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	if len(ret.Errors) > 0 {
		return nil, errors.New(sa.Name() + ": " + ret.Errors[0])
	}
	ordType := ret.OrdType
	if ordType == "" {
		ordType = ret.Type
	}
	ctime, _ := time.Parse(time.RFC3339, ret.CreatedAt)
	utime, _ := time.Parse(time.RFC3339, ret.UpdatedAt)
	status := sa.toStdOrderState(ret.State)
	if status == "NEW" && ret.FilledAmount.IsPositive() {
		status = "PARTIALLY_FILLED"
	}
	return &SpotOrder{
		Symbol:    strings.ToUpper(ret.Market),
		OrderId:   strconv.FormatInt(ret.Id, 10),
		ClientId:  cltId, // 该交易所不支持client order id, 回显调用方要查的cltId
		Price:     ret.Price,
		Qty:       ret.OriginAmount,
		FilledQty: ret.FilledAmount,
		FilledAmt: ret.FilledAmount.Mul(ret.AvgPrice),
		AvgPrice:  ret.AvgPrice,
		Status:    status,
		Type:      sa.toStdOrderType(ordType),
		Side:      sa.toStdSide(ret.Side),
		CTime:     ctime.UnixMilli(),
		UTime:     utime.UnixMilli(),
	}, nil
}
