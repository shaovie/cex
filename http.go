package cex

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 连接池按(交易所, 出口IP)隔离并复用
// 同一出口IP打同一交易所的多个cex对象共用一份池, 临时创建对象不再新建池,
// 避免fd, TLS握手与TIME_WAIT随对象数线性增长
var clientCache sync.Map // key: cexName + "|" + localIP

type Http struct {
	client *http.Client
}

// HttpConf 连接池参数, 字段为 0 表示沿用库内默认值
type HttpConf struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	DialTimeout         time.Duration
	KeepAlive           time.Duration
}

var (
	// 库内默认参数: 未绑出口IP / 已绑出口IP
	httpConfDefault = HttpConf{32, 24, 0, 90 * time.Second, 5 * time.Second, 5 * time.Second, 30 * time.Second}
	httpConfLocalIP = HttpConf{32, 8, 0, 30 * time.Second, 3 * time.Second, 3 * time.Second, 15 * time.Second}

	// 各交易所的定制参数, 未设置的项取上面的默认值
	httpConfMtx   sync.RWMutex
	httpConfByCex = map[string]HttpConf{
		// "binance": {MaxIdleConnsPerHost: 32},
		// "bybit":   {MaxIdleConnsPerHost: 24},
	}
	httpConfByCexIP = map[string]HttpConf{
		// "binance": {MaxIdleConnsPerHost: 16},
	}
)

// SetHttpConf 定制某交易所未绑定出口IP时的连接池参数
// 必须在该交易所的client创建之前调用, 否则返回错误
func SetHttpConf(cexName string, conf HttpConf) error {
	return setHttpConf(cexName, conf, httpConfByCex)
}

// SetHttpConfLocalIP 定制某交易所已绑定出口IP时的连接池参数
func SetHttpConfLocalIP(cexName string, conf HttpConf) error {
	return setHttpConf(cexName, conf, httpConfByCexIP)
}
func setHttpConf(cexName string, conf HttpConf, m map[string]HttpConf) error {
	httpConfMtx.Lock()
	defer httpConfMtx.Unlock()
	used := false
	clientCache.Range(func(k, _ any) bool {
		if strings.HasPrefix(k.(string), cexName+"|") {
			used = true
			return false
		}
		return true
	})
	if used {
		return errors.New("cex " + cexName + " client is already in use, SetHttpConf must be called before")
	}
	m[cexName] = conf
	return nil
}

// GetHttpConf 取某交易所当前生效的连接池参数
func GetHttpConf(cexName, localIP string) HttpConf {
	httpConfMtx.RLock()
	defer httpConfMtx.RUnlock()
	if localIP != "" {
		if conf, ok := httpConfByCexIP[cexName]; ok {
			return mergeHttpConf(conf, httpConfLocalIP)
		}
		return httpConfLocalIP
	}
	if conf, ok := httpConfByCex[cexName]; ok {
		return mergeHttpConf(conf, httpConfDefault)
	}
	return httpConfDefault
}
func mergeHttpConf(conf, def HttpConf) HttpConf {
	if conf.MaxIdleConns == 0 {
		conf.MaxIdleConns = def.MaxIdleConns
	}
	if conf.MaxIdleConnsPerHost == 0 {
		conf.MaxIdleConnsPerHost = def.MaxIdleConnsPerHost
	}
	if conf.MaxConnsPerHost == 0 {
		conf.MaxConnsPerHost = def.MaxConnsPerHost
	}
	if conf.IdleConnTimeout == 0 {
		conf.IdleConnTimeout = def.IdleConnTimeout
	}
	if conf.TLSHandshakeTimeout == 0 {
		conf.TLSHandshakeTimeout = def.TLSHandshakeTimeout
	}
	if conf.DialTimeout == 0 {
		conf.DialTimeout = def.DialTimeout
	}
	if conf.KeepAlive == 0 {
		conf.KeepAlive = def.KeepAlive
	}
	return conf
}

func NewClient(cexName, localIP string) (*http.Client, error) {
	key := cexName + "|" + localIP
	if v, ok := clientCache.Load(key); ok {
		return v.(*http.Client), nil
	}
	client, err := newClient(cexName, localIP)
	if err != nil {
		return nil, err
	}
	// 并发下可能同时构建, 只保留先入缓存的; 未使用的Transport不会建立连接
	actual, _ := clientCache.LoadOrStore(key, client)
	return actual.(*http.Client), nil
}
func newClient(cexName, localIP string) (*http.Client, error) {
	conf := GetHttpConf(cexName, localIP)
	dialer := &net.Dialer{
		Timeout:   conf.DialTimeout,
		KeepAlive: conf.KeepAlive,
	}
	if localIP != "" {
		ipAddr := net.ParseIP(localIP)
		if ipAddr == nil {
			return nil, errors.New("invalid local ip address")
		}
		dialer.LocalAddr = &net.TCPAddr{IP: ipAddr, Port: 0} // 绑定出口IP
	}
	tr := &http.Transport{
		MaxIdleConns:        conf.MaxIdleConns,
		MaxIdleConnsPerHost: conf.MaxIdleConnsPerHost,
		MaxConnsPerHost:     conf.MaxConnsPerHost,
		IdleConnTimeout:     conf.IdleConnTimeout,
		TLSHandshakeTimeout: conf.TLSHandshakeTimeout,
		DisableCompression:  false, // 启用gzip压缩（节省带宽）
		DialContext:         dialer.DialContext,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 { // 限制重定向次数（默认10次，避免无限重定向）
				return errors.New("too many redirects (max 5)")
			}
			return nil
		},
	}, nil
}
func (h *Http) Get(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodGet, link, nil, timeout, headers)
}
func (h *Http) Post(link string, pl []byte, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodPost, link, pl, timeout, headers)
}
func (h *Http) Delete(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodDelete, link, nil, timeout, headers)
}
func (h *Http) Put(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodPut, link, nil, timeout, headers)
}
func (h *Http) doRequest(method, link string, pl []byte, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	buffer := bytes.NewBuffer(pl)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, link, buffer)
	if err != nil {
		return 0, nil, errors.New("create request failed: " +
			link + ", err: " + err.Error())
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close() // 忽略关闭错误（核心是确保关闭）
		}
	}()
	if err != nil {
		var timeoutErr error
		if uErr, ok := err.(*url.Error); ok {
			if netErr, ok := uErr.Err.(net.Error); ok {
				if netErr.Timeout() {
					timeoutErr = errors.New("request timeout: " + link +
						", timeout: " + timeout.String())
				} else if netErr.Temporary() {
					timeoutErr = errors.New("temporary error (network issue): " +
						link + ", err: " + netErr.Error())
				}
			}
		}
		if timeoutErr != nil {
			return 0, nil, timeoutErr
		}
		return 0, nil, errors.New("request failed: " + link + ", err: " + err.Error())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, errors.New("read response body failed: " +
			link + ", err: " + err.Error())
	}

	return resp.StatusCode, body, nil
}
