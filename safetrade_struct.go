package cex

import (
	"github.com/shopspring/decimal"
)

// SafetradeDepth 公有depth推送的body, 如:
// {"asks":[],"bids":[["1.31","12449.2664"]],"sequence":3592202}
type SafetradeDepth struct {
	Asks [][2]decimal.Decimal `json:"asks"`
	Bids [][2]decimal.Decimal `json:"bids"`
}
