package cex

import "strconv"

// jsonNumber 兼容JSON number和string, 保留原始文本
type jsonNumber string

func (n *jsonNumber) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			s = v
		} else {
			s = s[1 : len(s)-1]
		}
	}
	*n = jsonNumber(s)
	return nil
}

func (n jsonNumber) String() string {
	return string(n)
}
