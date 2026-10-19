package cex

type KtxWsPubMsg struct {
	Pong   float64   `json:"pong"`
	Op     string    `json:"op"`
	Stream string    `json:"stream"`
	Data   WsRawJSON `json:"data,omitempty"`
}

func (v *KtxWsPubMsg) reset() {
	v.Pong = 0.0
	v.Op = ""
	v.Stream = ""
	v.Data = nil
}

type KtxTicker struct {
	Symbol   string `json:"product"`
	BidPrice string `json:"bidPrice"`
	BidQty   string `json:"bidQty"`
	AskPrice string `json:"askPrice"`
	AskQty   string `json:"askQty"`
}
