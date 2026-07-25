package cex

import (
	"encoding/json"
)

type KrakenWsMsg struct {
	Method  string          `json:"method"`
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

func (v *KrakenWsMsg) reset() {
	v.Data = nil
	v.Channel = ""
	v.Type = ""
	v.Method = ""
}
