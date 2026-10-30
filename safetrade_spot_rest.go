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
	_, resp, err := sa.GetWithCache(link, stApiDeadline, stHttpHeaders,
		spotPairRuleCacheFile(sa.Name()))
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
		Market   string `json:"market"`
		Side     string `json:"side"`
		Type     string `json:"type"`
		Price    string `json:"price,omitempty"`
		Amount   string `json:"amount,omitempty"`
		Total    string `json:"total,omitempty"`
		MaxPrice string `json:"max_price,omitempty"`
	}{
		Market: id,
		Side:   sa.fromStdSide(side),
		Type:   sa.fromStdOrderType(orderType),
	}
	if orderType == "LIMIT" {
		req.Price = price.String()
		req.Amount = qty.String()
	} else if orderType == "MARKET" {
		// 市价单price为保护价: 买入最高可接受价, 卖出最低可接受价
		if !price.IsPositive() {
			return "", errors.New(sa.Name() + " market order needs price(max_price)!")
		}
		req.MaxPrice = price.String()
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
	return nil
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

type safetradeOrder struct {
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
}

func (sa *Safetrade) toStdOrder(v *safetradeOrder) *SpotOrder {
	ordType := v.OrdType
	if ordType == "" {
		ordType = v.Type
	}
	ctime, _ := time.Parse(time.RFC3339, v.CreatedAt)
	utime, _ := time.Parse(time.RFC3339, v.UpdatedAt)
	status := sa.toStdOrderState(v.State)
	if status == "NEW" && v.FilledAmount.IsPositive() {
		status = "PARTIALLY_FILLED"
	}
	return &SpotOrder{
		Symbol:    strings.ToUpper(v.Market),
		OrderId:   strconv.FormatInt(v.Id, 10),
		Price:     v.Price,
		Qty:       v.OriginAmount,
		FilledQty: v.FilledAmount,
		FilledAmt: v.FilledAmount.Mul(v.AvgPrice),
		AvgPrice:  v.AvgPrice,
		Status:    status,
		Type:      sa.toStdOrderType(ordType),
		Side:      sa.toStdSide(v.Side),
		CTime:     ctime.UnixMilli(),
		UTime:     utime.UnixMilli(),
	}
}

func (sa *Safetrade) SpotGetOpenOrders(symbol string) ([]*SpotOrder, error) {
	params := "?state=wait,pending&limit=100&page=1"
	if symbol != "" {
		id := sa.getSpotSymbol(symbol)
		if id == "" {
			return nil, errors.New(sa.Name() + " unknown symbol! " + symbol)
		}
		params += "&market=" + id
	}
	link := stEndpoint + "/trade/market/orders" + params
	_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
	if err != nil {
		return nil, errors.New(sa.Name() + " net error! " + err.Error())
	}
	if len(resp) == 0 || resp[0] != '[' {
		return nil, sa.handleExceptionResp("SpotGetOpenOrders", resp)
	}
	ret := []safetradeOrder{}
	if err = json.Unmarshal(resp, &ret); err != nil {
		return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
	}
	dl := make([]*SpotOrder, 0, len(ret))
	for i := range ret {
		dl = append(dl, sa.toStdOrder(&ret[i]))
	}
	return dl, nil
}

// SpotGetFilledOrders 已成交订单: state=done, 按页取完
func (sa *Safetrade) SpotGetFilledOrders(symbol string) ([]*SpotOrder, error) {
	market := ""
	if symbol != "" {
		market = sa.getSpotSymbol(symbol)
		if market == "" {
			return nil, errors.New(sa.Name() + " unknown symbol! " + symbol)
		}
	}
	dl := make([]*SpotOrder, 0, 128)
	for page := 1; ; page++ {
		params := "?state=done&limit=100&page=" + strconv.Itoa(page)
		if market != "" {
			params += "&market=" + market
		}
		link := stEndpoint + "/trade/market/orders" + params
		_, resp, err := sa.Get(link, stApiDeadline, sa.buildHeaders())
		if err != nil {
			return nil, errors.New(sa.Name() + " net error! " + err.Error())
		}
		if len(resp) == 0 || resp[0] != '[' {
			return nil, sa.handleExceptionResp("SpotGetFilledOrders", resp)
		}
		ret := []safetradeOrder{}
		if err = json.Unmarshal(resp, &ret); err != nil {
			return nil, errors.New(sa.Name() + " unmarshal fail! " + err.Error())
		}
		for i := range ret {
			dl = append(dl, sa.toStdOrder(&ret[i]))
		}
		if len(ret) < 100 { // 不够一页说明取完了
			break
		}
	}
	return dl, nil
}
